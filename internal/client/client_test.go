package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test Helpers
// ============================================================================

// newTestServer creates a test HTTP server with the given handler.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

// newTestClient creates a client configured for the test server.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	c, err := New(Config{
		URL:      serverURL,
		Token:    "test-token",
		Insecure: true,
	})
	require.NoError(t, err)
	return c
}

// assertAuthHeader validates the Authorization header.
func assertAuthHeader(t *testing.T, r *http.Request) {
	t.Helper()
	assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
}

// jsonResponse writes a JSON response with the given status code.
func jsonResponse(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// deleteTestCase represents a test case for delete operations.
type deleteTestCase struct {
	name         string
	expectedPath string
	statusCode   int
	wantErr      bool
}

// runDeleteTest runs a delete operation test with the given test case and delete function.
func runDeleteTest(t *testing.T, tc deleteTestCase, deleteFn func(*Client) error) {
	t.Helper()
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assertAuthHeader(t, r)
		assert.Equal(t, "DELETE", r.Method)
		assert.Equal(t, tc.expectedPath, r.URL.Path)
		jsonResponse(w, tc.statusCode, nil)
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := deleteFn(client)

	if tc.wantErr {
		require.Error(t, err)
		return
	}
	require.NoError(t, err)
}

// Helper functions for env var manipulation in tests
func getEnvOrEmpty(key string) string {
	return os.Getenv(key)
}

func setEnv(key, value string) {
	_ = os.Setenv(key, value)
}

func clearEnv(key string) {
	_ = os.Unsetenv(key)
}

func setEnvOrClear(key, value string) {
	if value == "" {
		clearEnv(key)
	} else {
		setEnv(key, value)
	}
}

// ============================================================================
// New() Constructor Tests
// ============================================================================

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantErr   bool
		errMsg    string
		checkFunc func(t *testing.T, c *Client)
	}{
		{
			name: "valid config",
			config: Config{
				URL:      "https://example.com",
				Token:    "valid-token",
				Insecure: false,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, c *Client) {
				assert.Equal(t, "https://example.com", c.baseURL)
				assert.Equal(t, "valid-token", c.token)
				assert.False(t, c.insecure)
			},
		},
		{
			name: "missing token allowed for OAuth operations",
			config: Config{
				URL:   "https://example.com",
				Token: "",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, c *Client) {
				// Token is now optional in New() for OAuth operations
				// Token validation happens in request() for authenticated endpoints
				assert.NotNil(t, c)
			},
		},
		{
			name: "trailing slash trimmed",
			config: Config{
				URL:   "https://example.com/",
				Token: "valid-token",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, c *Client) {
				assert.Equal(t, "https://example.com", c.baseURL)
			},
		},
		{
			name: "multiple trailing slashes trimmed",
			config: Config{
				URL:   "https://example.com///",
				Token: "valid-token",
			},
			wantErr: false,
			checkFunc: func(t *testing.T, c *Client) {
				// Only one trailing slash is trimmed by TrimSuffix
				assert.Equal(t, "https://example.com//", c.baseURL)
			},
		},
		{
			name: "insecure flag set",
			config: Config{
				URL:      "https://example.com",
				Token:    "valid-token",
				Insecure: true,
			},
			wantErr: false,
			checkFunc: func(t *testing.T, c *Client) {
				assert.True(t, c.insecure)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, c)
			}
		})
	}
}

// ============================================================================
// Organization Operations Tests
// ============================================================================

func TestListOrgs(t *testing.T) {
	tests := []struct {
		name       string
		response   interface{}
		statusCode int
		wantErr    bool
		wantOrgs   []Org
	}{
		{
			name: "success with multiple orgs",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{"id": "org1", "name": "Org One", "state": "ORG_STATE_ACTIVE"},
					{"id": "org2", "name": "Org Two", "state": "ORG_STATE_INACTIVE"},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantOrgs: []Org{
				{ID: "org1", Name: "Org One", State: "ORG_STATE_ACTIVE"},
				{ID: "org2", Name: "Org Two", State: "ORG_STATE_INACTIVE"},
			},
		},
		{
			name: "success with empty result",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantOrgs:   []Org{},
		},
		{
			name:       "API error 500",
			response:   map[string]string{"error": "internal server error"},
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			name:       "API error 401",
			response:   map[string]string{"error": "unauthorized"},
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/admin/v1/orgs/_search", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			orgs, err := client.ListOrgs(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOrgs, orgs)
		})
	}
}

func TestGetOrgByName(t *testing.T) {
	tests := []struct {
		name       string
		orgName    string
		response   interface{}
		statusCode int
		wantErr    bool
		wantOrg    *Org
	}{
		{
			name:    "found org",
			orgName: "TestOrg",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{"id": "org123", "name": "TestOrg", "state": "ORG_STATE_ACTIVE"},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantOrg:    &Org{ID: "org123", Name: "TestOrg", State: "ORG_STATE_ACTIVE"},
		},
		{
			name:    "org not found",
			orgName: "NonExistent",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantOrg:    nil,
		},
		{
			name:       "API error 404",
			orgName:    "TestOrg",
			response:   map[string]string{"error": "not found"},
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/admin/v1/orgs/_search", r.URL.Path)

				// Verify query body contains name query
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if queries, ok := body["queries"].([]interface{}); ok && len(queries) > 0 {
					query := queries[0].(map[string]interface{})
					nameQuery := query["nameQuery"].(map[string]interface{})
					assert.Equal(t, tt.orgName, nameQuery["name"])
				}

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			org, err := client.GetOrgByName(context.Background(), tt.orgName)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOrg, org)
		})
	}
}

func TestCreateOrg(t *testing.T) {
	tests := []struct {
		name       string
		orgName    string
		response   interface{}
		statusCode int
		wantErr    bool
		wantOrg    *Org
	}{
		{
			name:       "success",
			orgName:    "NewOrg",
			response:   map[string]string{"id": "new-org-123"},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantOrg:    &Org{ID: "new-org-123", Name: "NewOrg", State: "ORG_STATE_ACTIVE"},
		},
		{
			name:       "API error 400",
			orgName:    "Invalid",
			response:   map[string]string{"error": "bad request"},
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "API error 403",
			orgName:    "Forbidden",
			response:   map[string]string{"error": "forbidden"},
			statusCode: http.StatusForbidden,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/admin/v1/orgs", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				assert.Equal(t, tt.orgName, body["name"])

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			org, err := client.CreateOrg(context.Background(), tt.orgName)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOrg, org)
		})
	}
}

// ============================================================================
// Project Operations Tests
// ============================================================================

func TestListProjects(t *testing.T) {
	tests := []struct {
		name         string
		response     interface{}
		statusCode   int
		wantErr      bool
		wantProjects []Project
	}{
		{
			name: "success with multiple projects",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{"id": "proj1", "name": "Project One", "state": "PROJECT_STATE_ACTIVE"},
					{"id": "proj2", "name": "Project Two", "state": "PROJECT_STATE_INACTIVE"},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantProjects: []Project{
				{ID: "proj1", Name: "Project One", State: "PROJECT_STATE_ACTIVE"},
				{ID: "proj2", Name: "Project Two", State: "PROJECT_STATE_INACTIVE"},
			},
		},
		{
			name: "empty result",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode:   http.StatusOK,
			wantErr:      false,
			wantProjects: []Project{},
		},
		{
			name:       "API error 500",
			response:   map[string]string{"error": "internal error"},
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/projects/_search", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			projects, err := client.ListProjects(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantProjects, projects)
		})
	}
}

func TestGetProjectByName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		response    interface{}
		statusCode  int
		wantErr     bool
		wantProject *Project
	}{
		{
			name:        "found project",
			projectName: "TestProject",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{"id": "proj123", "name": "TestProject", "state": "PROJECT_STATE_ACTIVE"},
				},
			},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantProject: &Project{ID: "proj123", Name: "TestProject", State: "PROJECT_STATE_ACTIVE"},
		},
		{
			name:        "project not found",
			projectName: "NonExistent",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantProject: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/projects/_search", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			project, err := client.GetProjectByName(context.Background(), tt.projectName)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantProject, project)
		})
	}
}

func TestCreateProject(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		response    interface{}
		statusCode  int
		wantErr     bool
		wantProject *Project
	}{
		{
			name:        "success",
			projectName: "NewProject",
			response:    map[string]string{"id": "new-proj-123"},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantProject: &Project{ID: "new-proj-123", Name: "NewProject", State: "PROJECT_STATE_ACTIVE"},
		},
		{
			name:        "API error 400",
			projectName: "Invalid",
			response:    map[string]string{"error": "bad request"},
			statusCode:  http.StatusBadRequest,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/projects", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				assert.Equal(t, tt.projectName, body["name"])
				assert.Equal(t, true, body["projectRoleAssertion"])

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			project, err := client.CreateProject(context.Background(), tt.projectName)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantProject, project)
		})
	}
}

func TestDeleteProject(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "success",
			projectID:  "proj-to-delete",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "not found",
			projectID:  "non-existent",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:       "forbidden",
			projectID:  "forbidden-proj",
			statusCode: http.StatusForbidden,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "DELETE", r.Method)
				assert.Equal(t, "/management/v1/projects/"+tt.projectID, r.URL.Path)
				jsonResponse(w, tt.statusCode, nil)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			err := client.DeleteProject(context.Background(), tt.projectID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// ============================================================================
// Application Operations Tests
// ============================================================================

func TestListApps(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		response   interface{}
		statusCode int
		wantErr    bool
		wantApps   []App
	}{
		{
			name:      "success with OIDC app",
			projectID: "proj123",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":   "app1",
						"name": "OIDC App",
						"oidcConfig": map[string]interface{}{
							"clientId":     "client-id-123",
							"redirectUris": []string{"http://localhost:3000/callback"},
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantApps: []App{
				{
					ID:           "app1",
					Name:         "OIDC App",
					Type:         "oidc",
					ClientID:     "client-id-123",
					RedirectURIs: []string{"http://localhost:3000/callback"},
				},
			},
		},
		{
			name:      "success with API app",
			projectID: "proj123",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":   "app2",
						"name": "API App",
						"apiConfig": map[string]interface{}{
							"clientId": "api-client-123",
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantApps: []App{
				{
					ID:       "app2",
					Name:     "API App",
					Type:     "api",
					ClientID: "api-client-123",
				},
			},
		},
		{
			name:      "empty result",
			projectID: "proj123",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantApps:   []App{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/projects/"+tt.projectID+"/apps/_search", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			apps, err := client.ListApps(context.Background(), tt.projectID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantApps, apps)
		})
	}
}

func TestCreateOIDCApp(t *testing.T) {
	tests := []struct {
		name          string
		projectID     string
		config        OIDCAppConfig
		response      interface{}
		statusCode    int
		wantErr       bool
		wantApp       *App
		checkAppType  string
		checkAuthType string
	}{
		{
			name:      "success web app",
			projectID: "proj123",
			config: OIDCAppConfig{
				Name:         "Web App",
				RedirectURIs: []string{"http://localhost:3000/callback"},
				AppType:      "web",
				DevMode:      true,
			},
			response: map[string]interface{}{
				"appId":        "app-123",
				"clientId":     "client-123",
				"clientSecret": "secret-123",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantApp: &App{
				ID:           "app-123",
				Name:         "Web App",
				Type:         "oidc",
				ClientID:     "client-123",
				ClientSecret: "secret-123",
				RedirectURIs: []string{"http://localhost:3000/callback"},
			},
			checkAppType:  "OIDC_APP_TYPE_WEB",
			checkAuthType: "OIDC_AUTH_METHOD_TYPE_BASIC",
		},
		{
			name:      "success SPA app",
			projectID: "proj123",
			config: OIDCAppConfig{
				Name:         "SPA App",
				RedirectURIs: []string{"http://localhost:3000/callback"},
				AppType:      "spa",
			},
			response: map[string]interface{}{
				"appId":    "app-spa-123",
				"clientId": "client-spa-123",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantApp: &App{
				ID:           "app-spa-123",
				Name:         "SPA App",
				Type:         "oidc",
				ClientID:     "client-spa-123",
				RedirectURIs: []string{"http://localhost:3000/callback"},
			},
			checkAppType:  "OIDC_APP_TYPE_USER_AGENT",
			checkAuthType: "OIDC_AUTH_METHOD_TYPE_NONE",
		},
		{
			name:      "success native app",
			projectID: "proj123",
			config: OIDCAppConfig{
				Name:         "Native App",
				RedirectURIs: []string{"myapp://callback"},
				AppType:      "native",
			},
			response: map[string]interface{}{
				"appId":    "app-native-123",
				"clientId": "client-native-123",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantApp: &App{
				ID:           "app-native-123",
				Name:         "Native App",
				Type:         "oidc",
				ClientID:     "client-native-123",
				RedirectURIs: []string{"myapp://callback"},
			},
			checkAppType:  "OIDC_APP_TYPE_NATIVE",
			checkAuthType: "OIDC_AUTH_METHOD_TYPE_NONE",
		},
		{
			name:      "API error",
			projectID: "proj123",
			config: OIDCAppConfig{
				Name:         "Error App",
				RedirectURIs: []string{"http://localhost/callback"},
				AppType:      "web",
			},
			response:   map[string]string{"error": "bad request"},
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/projects/"+tt.projectID+"/apps/oidc", r.URL.Path)

				if tt.checkAppType != "" {
					var body map[string]interface{}
					_ = json.NewDecoder(r.Body).Decode(&body)
					assert.Equal(t, tt.checkAppType, body["appType"])
					assert.Equal(t, tt.checkAuthType, body["authMethodType"])
				}

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			app, err := client.CreateOIDCApp(context.Background(), tt.projectID, tt.config)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantApp, app)
		})
	}
}

func TestDeleteApp(t *testing.T) {
	tests := []deleteTestCase{
		{
			name:         "success",
			expectedPath: "/management/v1/projects/proj123/apps/app123",
			statusCode:   http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "not found",
			expectedPath: "/management/v1/projects/proj123/apps/non-existent",
			statusCode:   http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectID := strings.Split(tt.expectedPath, "/")[4]
			appID := strings.Split(tt.expectedPath, "/")[6]
			runDeleteTest(t, tt, func(c *Client) error {
				return c.DeleteApp(context.Background(), projectID, appID)
			})
		})
	}
}

// ============================================================================
// User Operations Tests
// ============================================================================

func TestListUsers(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		response   interface{}
		statusCode int
		wantErr    bool
		wantUsers  []User
	}{
		{
			name:  "success with human users",
			query: "",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":       "user1",
						"userName": "john.doe",
						"state":    "USER_STATE_ACTIVE",
						"human": map[string]interface{}{
							"profile": map[string]interface{}{
								"firstName":   "John",
								"lastName":    "Doe",
								"displayName": "John Doe",
							},
							"email": map[string]interface{}{
								"email": "john@example.com",
							},
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUsers: []User{
				{
					ID:          "user1",
					Username:    "john.doe",
					Email:       "john@example.com",
					FirstName:   "John",
					LastName:    "Doe",
					DisplayName: "John Doe",
					State:       "USER_STATE_ACTIVE",
				},
			},
		},
		{
			name:  "success with query filter",
			query: "john",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUsers:  []User{},
		},
		{
			name:  "user without human profile",
			query: "",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":       "machine1",
						"userName": "service-account",
						"state":    "USER_STATE_ACTIVE",
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUsers: []User{
				{
					ID:       "machine1",
					Username: "service-account",
					State:    "USER_STATE_ACTIVE",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/_search", r.URL.Path)

				if tt.query != "" {
					var body map[string]interface{}
					_ = json.NewDecoder(r.Body).Decode(&body)
					queries := body["queries"].([]interface{})
					assert.Len(t, queries, 1)
				}

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			users, err := client.ListUsers(context.Background(), tt.query)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUsers, users)
		})
	}
}

func TestGetUserByUsername(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		response   interface{}
		statusCode int
		wantErr    bool
		wantUser   *User
	}{
		{
			name:     "found user",
			username: "john.doe",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":       "user123",
						"userName": "john.doe",
						"state":    "USER_STATE_ACTIVE",
						"human": map[string]interface{}{
							"profile": map[string]interface{}{
								"firstName":   "John",
								"lastName":    "Doe",
								"displayName": "John Doe",
							},
							"email": map[string]interface{}{
								"email": "john@example.com",
							},
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUser: &User{
				ID:          "user123",
				Username:    "john.doe",
				Email:       "john@example.com",
				FirstName:   "John",
				LastName:    "Doe",
				DisplayName: "John Doe",
				State:       "USER_STATE_ACTIVE",
			},
		},
		{
			name:     "user not found",
			username: "non-existent",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUser:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/_search", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			user, err := client.GetUserByUsername(context.Background(), tt.username)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUser, user)
		})
	}
}

func TestCreateUser(t *testing.T) {
	tests := []struct {
		name       string
		config     CreateUserConfig
		response   interface{}
		statusCode int
		wantErr    bool
		wantUser   *User
	}{
		{
			name: "success",
			config: CreateUserConfig{
				Username:  "new.user",
				Email:     "new@example.com",
				FirstName: "New",
				LastName:  "User",
				Password:  "SecurePass123!",
			},
			response:   map[string]string{"userId": "new-user-123"},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUser: &User{
				ID:          "new-user-123",
				Username:    "new.user",
				Email:       "new@example.com",
				FirstName:   "New",
				LastName:    "User",
				DisplayName: "New User",
				State:       "USER_STATE_ACTIVE",
			},
		},
		{
			name: "API error",
			config: CreateUserConfig{
				Username:  "invalid",
				Email:     "invalid",
				FirstName: "",
				LastName:  "",
				Password:  "",
			},
			response:   map[string]string{"error": "validation error"},
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/human/_import", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				assert.Equal(t, tt.config.Username, body["userName"])

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			user, err := client.CreateUser(context.Background(), tt.config)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUser, user)
		})
	}
}

// ============================================================================
// Machine User Operations Tests
// ============================================================================

func TestListMachineUsers(t *testing.T) {
	tests := []struct {
		name       string
		response   interface{}
		statusCode int
		wantErr    bool
		wantUsers  []MachineUser
	}{
		{
			name: "success",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":       "machine1",
						"userName": "service-account-1",
						"state":    "USER_STATE_ACTIVE",
						"machine": map[string]interface{}{
							"name":        "Service Account 1",
							"description": "Test service account",
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUsers: []MachineUser{
				{
					ID:          "machine1",
					Username:    "service-account-1",
					Name:        "Service Account 1",
					Description: "Test service account",
					State:       "USER_STATE_ACTIVE",
				},
			},
		},
		{
			name: "machine user without machine details",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":       "machine2",
						"userName": "service-account-2",
						"state":    "USER_STATE_ACTIVE",
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUsers: []MachineUser{
				{
					ID:       "machine2",
					Username: "service-account-2",
					State:    "USER_STATE_ACTIVE",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/_search", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				queries := body["queries"].([]interface{})
				query := queries[0].(map[string]interface{})
				typeQuery := query["typeQuery"].(map[string]interface{})
				assert.Equal(t, "TYPE_MACHINE", typeQuery["type"])

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			users, err := client.ListMachineUsers(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUsers, users)
		})
	}
}

func TestCreateMachineUser(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		displayName string
		description string
		response    interface{}
		statusCode  int
		wantErr     bool
		wantUser    *MachineUser
	}{
		{
			name:        "success",
			username:    "new-service-account",
			displayName: "Service Account",
			description: "A new service account",
			response:    map[string]string{"userId": "machine-123"},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantUser: &MachineUser{
				ID:          "machine-123",
				Username:    "new-service-account",
				Name:        "Service Account",
				Description: "A new service account",
				State:       "USER_STATE_ACTIVE",
			},
		},
		{
			name:        "API error",
			username:    "invalid",
			displayName: "",
			description: "",
			response:    map[string]string{"error": "bad request"},
			statusCode:  http.StatusBadRequest,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/machine", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				assert.Equal(t, tt.username, body["userName"])
				assert.Equal(t, tt.displayName, body["name"])
				assert.Equal(t, tt.description, body["description"])

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			user, err := client.CreateMachineUser(context.Background(), tt.username, tt.displayName, tt.description)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUser, user)
		})
	}
}

// ============================================================================
// PAT Operations Tests
// ============================================================================

func TestListPATs(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		response   interface{}
		statusCode int
		wantErr    bool
		wantPATs   []PAT
	}{
		{
			name:   "success",
			userID: "user123",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{"id": "pat1", "expirationDate": "2025-12-31T00:00:00Z"},
					{"id": "pat2", "expirationDate": "2026-06-30T00:00:00Z"},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantPATs: []PAT{
				{ID: "pat1", Expiration: "2025-12-31T00:00:00Z"},
				{ID: "pat2", Expiration: "2026-06-30T00:00:00Z"},
			},
		},
		{
			name:   "empty result",
			userID: "user123",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantPATs:   []PAT{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/"+tt.userID+"/pats/_search", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			pats, err := client.ListPATs(context.Background(), tt.userID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPATs, pats)
		})
	}
}

func TestCreatePAT(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		expirationDays int
		response       interface{}
		statusCode     int
		wantErr        bool
	}{
		{
			name:           "success",
			userID:         "user123",
			expirationDays: 30,
			response: map[string]interface{}{
				"tokenId": "pat-123",
				"token":   "secret-token-value",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:           "API error",
			userID:         "user123",
			expirationDays: 30,
			response:       map[string]string{"error": "forbidden"},
			statusCode:     http.StatusForbidden,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/"+tt.userID+"/pats", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				assert.NotEmpty(t, body["expirationDate"])

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			pat, err := client.CreatePAT(context.Background(), tt.userID, tt.expirationDays)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "pat-123", pat.ID)
			assert.Equal(t, "secret-token-value", pat.Token)
			assert.NotEmpty(t, pat.Expiration)
		})
	}
}

// ============================================================================
// Healthz Tests
// ============================================================================

func TestHealthz(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "healthy",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "unhealthy 503",
			statusCode: http.StatusServiceUnavailable,
			wantErr:    true,
		},
		{
			name:       "internal error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/healthz", r.URL.Path)
				jsonResponse(w, tt.statusCode, nil)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			err := client.Healthz(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// ============================================================================
// Error Scenarios Tests
// ============================================================================

func TestAPIErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		errContain string
	}{
		{
			name:       "400 Bad Request",
			statusCode: http.StatusBadRequest,
			response:   `{"error": "invalid request body"}`,
			errContain: "API error (400)",
		},
		{
			name:       "401 Unauthorized",
			statusCode: http.StatusUnauthorized,
			response:   `{"error": "invalid token"}`,
			errContain: "API error (401)",
		},
		{
			name:       "403 Forbidden",
			statusCode: http.StatusForbidden,
			response:   `{"error": "access denied"}`,
			errContain: "API error (403)",
		},
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			response:   `{"error": "resource not found"}`,
			errContain: "API error (404)",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			response:   `{"error": "internal server error"}`,
			errContain: "API error (500)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			_, err := client.ListOrgs(context.Background())

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContain)
		})
	}
}

func TestInvalidJSONResponse(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL)

	t.Run("ListOrgs invalid JSON", func(t *testing.T) {
		_, err := client.ListOrgs(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("GetOrgByName invalid JSON", func(t *testing.T) {
		_, err := client.GetOrgByName(context.Background(), "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("CreateOrg invalid JSON", func(t *testing.T) {
		_, err := client.CreateOrg(context.Background(), "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("ListProjects invalid JSON", func(t *testing.T) {
		_, err := client.ListProjects(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("CreateProject invalid JSON", func(t *testing.T) {
		_, err := client.CreateProject(context.Background(), "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("ListApps invalid JSON", func(t *testing.T) {
		_, err := client.ListApps(context.Background(), "proj123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("CreateOIDCApp invalid JSON", func(t *testing.T) {
		_, err := client.CreateOIDCApp(context.Background(), "proj123", OIDCAppConfig{
			Name:         "test",
			RedirectURIs: []string{"http://localhost"},
			AppType:      "web",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("ListUsers invalid JSON", func(t *testing.T) {
		_, err := client.ListUsers(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("CreateUser invalid JSON", func(t *testing.T) {
		_, err := client.CreateUser(context.Background(), CreateUserConfig{
			Username:  "test",
			Email:     "test@test.com",
			FirstName: "Test",
			LastName:  "User",
			Password:  "password",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("ListMachineUsers invalid JSON", func(t *testing.T) {
		_, err := client.ListMachineUsers(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("CreateMachineUser invalid JSON", func(t *testing.T) {
		_, err := client.CreateMachineUser(context.Background(), "test", "Test", "desc")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("ListPATs invalid JSON", func(t *testing.T) {
		_, err := client.ListPATs(context.Background(), "user123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})

	t.Run("CreatePAT invalid JSON", func(t *testing.T) {
		_, err := client.CreatePAT(context.Background(), "user123", 30)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse response")
	})
}

func TestNetworkTimeout(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		// Sleep longer than context timeout
		time.Sleep(200 * time.Millisecond)
		jsonResponse(w, http.StatusOK, map[string]interface{}{"result": []interface{}{}})
	})
	defer server.Close()

	client := newTestClient(t, server.URL)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.ListOrgs(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

// ============================================================================
// Additional Operations Tests
// ============================================================================

func TestRegenerateClientSecret(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		appID      string
		response   interface{}
		statusCode int
		wantErr    bool
		wantSecret string
	}{
		{
			name:       "success",
			projectID:  "proj123",
			appID:      "app123",
			response:   map[string]string{"clientSecret": "new-secret-123"},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantSecret: "new-secret-123",
		},
		{
			name:       "not found",
			projectID:  "proj123",
			appID:      "non-existent",
			response:   map[string]string{"error": "not found"},
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/projects/"+tt.projectID+"/apps/"+tt.appID+"/secret", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			secret, err := client.RegenerateClientSecret(context.Background(), tt.projectID, tt.appID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSecret, secret)
		})
	}
}

func TestSetUserPassword(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		password   string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "success",
			userID:     "user123",
			password:   "NewPassword123!",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "user not found",
			userID:     "non-existent",
			password:   "password",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/"+tt.userID+"/password", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				assert.Equal(t, tt.password, body["password"])
				assert.Equal(t, false, body["changeRequired"])

				jsonResponse(w, tt.statusCode, nil)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			err := client.SetUserPassword(context.Background(), tt.userID, tt.password)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDeleteUser(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "success",
			userID:     "user-to-delete",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "not found",
			userID:     "non-existent",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "DELETE", r.Method)
				assert.Equal(t, "/management/v1/users/"+tt.userID, r.URL.Path)
				jsonResponse(w, tt.statusCode, nil)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			err := client.DeleteUser(context.Background(), tt.userID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDeletePAT(t *testing.T) {
	tests := []deleteTestCase{
		{
			name:         "success",
			expectedPath: "/management/v1/users/user123/pats/pat123",
			statusCode:   http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "not found",
			expectedPath: "/management/v1/users/user123/pats/non-existent",
			statusCode:   http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Split(tt.expectedPath, "/")
			userID, patID := parts[4], parts[6]
			runDeleteTest(t, tt, func(c *Client) error {
				return c.DeletePAT(context.Background(), userID, patID)
			})
		})
	}
}

func TestAddProjectRole(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		role       Role
		statusCode int
		wantErr    bool
	}{
		{
			name:      "success",
			projectID: "proj123",
			role: Role{
				Key:         "admin",
				DisplayName: "Administrator",
				Group:       "management",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:      "API error",
			projectID: "proj123",
			role: Role{
				Key: "invalid",
			},
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/projects/"+tt.projectID+"/roles", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				assert.Equal(t, tt.role.Key, body["roleKey"])
				assert.Equal(t, tt.role.DisplayName, body["displayName"])
				assert.Equal(t, tt.role.Group, body["group"])

				jsonResponse(w, tt.statusCode, nil)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			err := client.AddProjectRole(context.Background(), tt.projectID, tt.role)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestListProjectRoles(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		response   interface{}
		statusCode int
		wantErr    bool
		wantRoles  []Role
	}{
		{
			name:      "success",
			projectID: "proj123",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{"key": "admin", "displayName": "Administrator", "group": "management"},
					{"key": "user", "displayName": "User", "group": "users"},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantRoles: []Role{
				{Key: "admin", DisplayName: "Administrator", Group: "management"},
				{Key: "user", DisplayName: "User", Group: "users"},
			},
		},
		{
			name:      "empty result",
			projectID: "proj123",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantRoles:  []Role{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/projects/"+tt.projectID+"/roles/_search", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			roles, err := client.ListProjectRoles(context.Background(), tt.projectID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRoles, roles)
		})
	}
}

func TestGrantUserProjectRoles(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		userID     string
		roleKeys   []string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "success",
			projectID:  "proj123",
			userID:     "user123",
			roleKeys:   []string{"admin", "user"},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "API error",
			projectID:  "proj123",
			userID:     "user123",
			roleKeys:   []string{"invalid"},
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/"+tt.userID+"/grants", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				assert.Equal(t, tt.projectID, body["projectId"])

				jsonResponse(w, tt.statusCode, nil)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			err := client.GrantUserProjectRoles(context.Background(), tt.projectID, tt.userID, tt.roleKeys)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGetMyOrg(t *testing.T) {
	tests := []struct {
		name       string
		response   interface{}
		statusCode int
		wantErr    bool
		wantOrg    *Org
	}{
		{
			name: "success",
			response: map[string]interface{}{
				"id":    "my-org-123",
				"name":  "My Organization",
				"state": "ORG_STATE_ACTIVE",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantOrg:    &Org{ID: "my-org-123", Name: "My Organization", State: "ORG_STATE_ACTIVE"},
		},
		{
			name:       "unauthorized",
			response:   map[string]string{"error": "unauthorized"},
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/auth/v1/users/me/org", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			org, err := client.GetMyOrg(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOrg, org)
		})
	}
}

func TestGetMachineUserByUsername(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		response   interface{}
		statusCode int
		wantErr    bool
		wantUser   *MachineUser
	}{
		{
			name:     "found machine user",
			username: "service-account",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":       "machine123",
						"userName": "service-account",
						"state":    "USER_STATE_ACTIVE",
						"machine": map[string]interface{}{
							"name":        "Service Account",
							"description": "A service account",
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUser: &MachineUser{
				ID:          "machine123",
				Username:    "service-account",
				Name:        "Service Account",
				Description: "A service account",
				State:       "USER_STATE_ACTIVE",
			},
		},
		{
			name:     "machine user not found",
			username: "non-existent",
			response: map[string]interface{}{
				"result": []map[string]interface{}{},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUser:   nil,
		},
		{
			name:     "machine user without machine details",
			username: "minimal-sa",
			response: map[string]interface{}{
				"result": []map[string]interface{}{
					{
						"id":       "machine456",
						"userName": "minimal-sa",
						"state":    "USER_STATE_ACTIVE",
					},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantUser: &MachineUser{
				ID:       "machine456",
				Username: "minimal-sa",
				State:    "USER_STATE_ACTIVE",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/_search", r.URL.Path)

				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				queries := body["queries"].([]interface{})
				assert.Len(t, queries, 2) // userNameQuery and typeQuery

				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			user, err := client.GetMachineUserByUsername(context.Background(), tt.username)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUser, user)
		})
	}
}

func TestCreateMachineUserPAT(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		expirationDays int
		response       interface{}
		statusCode     int
		wantErr        bool
		wantToken      string
	}{
		{
			name:           "success",
			userID:         "machine123",
			expirationDays: 90,
			response: map[string]interface{}{
				"tokenId": "pat-machine-123",
				"token":   "machine-token-value",
			},
			statusCode: http.StatusOK,
			wantErr:    false,
			wantToken:  "machine-token-value",
		},
		{
			name:           "API error",
			userID:         "machine123",
			expirationDays: 90,
			response:       map[string]string{"error": "forbidden"},
			statusCode:     http.StatusForbidden,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				assertAuthHeader(t, r)
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/management/v1/users/"+tt.userID+"/pats", r.URL.Path)
				jsonResponse(w, tt.statusCode, tt.response)
			})
			defer server.Close()

			client := newTestClient(t, server.URL)
			token, err := client.CreateMachineUserPAT(context.Background(), tt.userID, tt.expirationDays)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

func TestSetOrg(t *testing.T) {
	// SetOrg is a no-op in the current implementation
	c, err := New(Config{
		URL:   "https://example.com",
		Token: "test-token",
	})
	require.NoError(t, err)

	err = c.SetOrg(context.Background(), "org123")
	require.NoError(t, err)
}

func TestGetProjectByNameInvalidJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetProjectByName(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

func TestGetUserByUsernameInvalidJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetUserByUsername(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

func TestRegenerateClientSecretInvalidJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.RegenerateClientSecret(context.Background(), "proj", "app")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

func TestListProjectRolesInvalidJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListProjectRoles(context.Background(), "proj")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

func TestGetMachineUserByUsernameInvalidJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetMachineUserByUsername(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

func TestGetMyOrgInvalidJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{invalid json}`))
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetMyOrg(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

// ============================================================================
// Request Header Tests
// ============================================================================

func TestRequestHeaders(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify all required headers
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		jsonResponse(w, http.StatusOK, map[string]interface{}{"result": []interface{}{}})
	})
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, _ = client.ListOrgs(context.Background())
}

// ============================================================================
// DefaultConfig Tests
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	// Save original env vars
	originalURL := strings.Clone(getEnvOrEmpty("ZITADEL_URL"))
	originalPAT := strings.Clone(getEnvOrEmpty("ZITADEL_PAT"))
	originalInsecure := strings.Clone(getEnvOrEmpty("ZITADEL_INSECURE"))

	// Restore after test
	defer func() {
		setEnvOrClear("ZITADEL_URL", originalURL)
		setEnvOrClear("ZITADEL_PAT", originalPAT)
		setEnvOrClear("ZITADEL_INSECURE", originalInsecure)
	}()

	t.Run("default values", func(t *testing.T) {
		clearEnv("ZITADEL_URL")
		clearEnv("ZITADEL_PAT")
		clearEnv("ZITADEL_INSECURE")

		cfg := DefaultConfig()
		assert.Equal(t, "https://zitadel.infra.test", cfg.URL)
		assert.Empty(t, cfg.Token)
		assert.True(t, cfg.Insecure) // Default is "true"
	})

	t.Run("custom values from env", func(t *testing.T) {
		setEnv("ZITADEL_URL", "https://custom.zitadel.io")
		setEnv("ZITADEL_PAT", "custom-token")
		setEnv("ZITADEL_INSECURE", "false")

		cfg := DefaultConfig()
		assert.Equal(t, "https://custom.zitadel.io", cfg.URL)
		assert.Equal(t, "custom-token", cfg.Token)
		assert.False(t, cfg.Insecure)
	})
}
