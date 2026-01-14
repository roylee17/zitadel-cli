// Package client provides a REST client for the Zitadel API.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// API paths - centralized for easy versioning.
const (
	// apiVersion is reserved for future API versioning.
	_ = "v1"

	// Admin API
	pathOrgsSearch = "/admin/v1/orgs/_search"
	pathOrgsCreate = "/admin/v1/orgs"

	// Management API
	pathProjectsSearch     = "/management/v1/projects/_search"
	pathProjects           = "/management/v1/projects"
	pathProjectByID        = "/management/v1/projects/%s"
	pathAppsSearch         = "/management/v1/projects/%s/apps/_search"
	pathAppsOIDC           = "/management/v1/projects/%s/apps/oidc"
	pathAppByID            = "/management/v1/projects/%s/apps/%s"
	pathAppSecret          = "/management/v1/projects/%s/apps/%s/secret"
	pathUsersSearch        = "/management/v1/users/_search"
	pathUsersMachine       = "/management/v1/users/machine"
	pathUsersHumanImport   = "/management/v1/users/human/_import"
	pathUserByID           = "/management/v1/users/%s"
	pathUserPassword       = "/management/v1/users/%s/password"
	pathUserGrants         = "/management/v1/users/%s/grants"
	pathUserPATsSearch     = "/management/v1/users/%s/pats/_search"
	pathUserPATs           = "/management/v1/users/%s/pats"
	pathUserPATByID        = "/management/v1/users/%s/pats/%s"
	pathProjectRolesSearch = "/management/v1/projects/%s/roles/_search"
	pathProjectRoles       = "/management/v1/projects/%s/roles"

	// Auth API
	pathMyOrg = "/auth/v1/users/me/org"

	// Health
	pathHealthz = "/healthz"
)

// Client wraps Zitadel API access.
// Uses REST API for Management operations (most comprehensive).
// The official SDK is better for auth flows, but Management API via REST
// is more complete for provisioning use cases.
type Client struct {
	baseURL    string
	token      string
	insecure   bool
	httpClient *http.Client
}

// Config holds client configuration.
type Config struct {
	// URL is the Zitadel instance URL (e.g., https://zitadel.k8s.orb.local)
	URL string
	// Token is the Personal Access Token (PAT) for authentication
	Token string
	// Insecure skips TLS verification (for local dev with self-signed certs)
	Insecure bool
}

// DefaultConfig returns configuration from environment variables.
func DefaultConfig() Config {
	return Config{
		URL:      getEnv("ZITADEL_URL", "https://zitadel.infra.test"),
		Token:    os.Getenv("ZITADEL_PAT"),
		Insecure: getEnv("ZITADEL_INSECURE", "true") == "true",
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// New creates a new Zitadel client.
func New(cfg Config) (*Client, error) {
	// Token is optional for OAuth operations (e.g., JWT bearer grant)
	// but required for most other operations

	// Ensure URL doesn't have trailing slash
	cfg.URL = strings.TrimSuffix(cfg.URL, "/")

	tr := &http.Transport{}
	if cfg.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // Intentional for local dev with self-signed certs
	}

	return &Client{
		baseURL:  cfg.URL,
		token:    cfg.Token,
		insecure: cfg.Insecure,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: tr,
		},
	}, nil
}

// request performs an HTTP request to the Zitadel API.
func (c *Client) request(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	// Validate token is present for authenticated requests
	if c.token == "" {
		return nil, fmt.Errorf("authentication token required (PAT must be set)")
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// requestWithToken performs an HTTP request to the Zitadel API with a custom token.
func (c *Client) requestWithToken(ctx context.Context, method, path, token string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// requestOAuth performs an unauthenticated OAuth request (form-encoded).
func (c *Client) requestOAuth(ctx context.Context, path string, data map[string]string) ([]byte, error) {
	values := url.Values{}
	for k, v := range data {
		values.Set(k, v)
	}
	bodyStr := values.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, strings.NewReader(bodyStr))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// ============================================================================
// Organization Operations
// ============================================================================

// Org represents a Zitadel organization.
type Org struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// GetID returns the organization ID.
func (o *Org) GetID() string { return o.ID }

// GetName returns the organization name.
func (o *Org) GetName() string { return o.Name }

// ListOrgs returns all organizations.
func (c *Client) ListOrgs(ctx context.Context) ([]Org, error) {
	resp, err := c.request(ctx, "POST", pathOrgsSearch, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	orgs := make([]Org, len(result.Result))
	for i, o := range result.Result {
		orgs[i] = Org{ID: o.ID, Name: o.Name, State: o.State}
	}
	return orgs, nil
}

// nameSearchResult represents a common structure for name-based search results.
type nameSearchResult struct {
	Result []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"result"`
}

// searchByName performs a name-based search and returns the first result.
func (c *Client) searchByName(ctx context.Context, path, name string) (*nameSearchResult, error) {
	resp, err := c.request(ctx, "POST", path, map[string]interface{}{
		"queries": []map[string]interface{}{
			{
				"nameQuery": map[string]interface{}{
					"name":   name,
					"method": "TEXT_QUERY_METHOD_EQUALS",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	var result nameSearchResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

// GetOrgByName finds an organization by name.
func (c *Client) GetOrgByName(ctx context.Context, name string) (*Org, error) {
	result, err := c.searchByName(ctx, pathOrgsSearch, name)
	if err != nil {
		return nil, err
	}
	if len(result.Result) == 0 {
		return nil, nil
	}
	o := result.Result[0]
	return &Org{ID: o.ID, Name: o.Name, State: o.State}, nil
}

// CreateOrg creates a new organization.
func (c *Client) CreateOrg(ctx context.Context, name string) (*Org, error) {
	resp, err := c.request(ctx, "POST", pathOrgsCreate, map[string]interface{}{
		"name": name,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &Org{ID: result.ID, Name: name, State: "ORG_STATE_ACTIVE"}, nil
}

// SetOrg sets the organization context for subsequent operations.
func (c *Client) SetOrg(_ context.Context, _ string) error {
	// Add x-zitadel-orgid header for org-scoped operations
	// This is handled per-request in management endpoints
	return nil
}

// ============================================================================
// Project Operations
// ============================================================================

// Project represents a Zitadel project.
type Project struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// GetID returns the project ID.
func (p *Project) GetID() string { return p.ID }

// GetName returns the project name.
func (p *Project) GetName() string { return p.Name }

// ListProjects returns all projects in the default organization.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	resp, err := c.request(ctx, "POST", pathProjectsSearch, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	projects := make([]Project, len(result.Result))
	for i, p := range result.Result {
		projects[i] = Project{ID: p.ID, Name: p.Name, State: p.State}
	}
	return projects, nil
}

// GetProjectByName finds a project by name.
func (c *Client) GetProjectByName(ctx context.Context, name string) (*Project, error) {
	result, err := c.searchByName(ctx, pathProjectsSearch, name)
	if err != nil {
		return nil, err
	}
	if len(result.Result) == 0 {
		return nil, nil
	}
	p := result.Result[0]
	return &Project{ID: p.ID, Name: p.Name, State: p.State}, nil
}

// CreateProject creates a new project.
func (c *Client) CreateProject(ctx context.Context, name string) (*Project, error) {
	resp, err := c.request(ctx, "POST", pathProjects, map[string]interface{}{
		"name":                 name,
		"projectRoleAssertion": true,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &Project{ID: result.ID, Name: name, State: "PROJECT_STATE_ACTIVE"}, nil
}

// DeleteProject deletes a project.
func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	path := fmt.Sprintf(pathProjectByID, projectID)
	_, err := c.request(ctx, "DELETE", path, nil)
	return err
}

// ============================================================================
// Application Operations
// ============================================================================

// App represents a Zitadel application.
type App struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"` // "oidc", "api", "saml"
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"clientSecret,omitempty"`
	RedirectURIs []string `json:"redirectUris,omitempty"`
}

// OIDCAppConfig holds OIDC app configuration.
type OIDCAppConfig struct {
	Name                   string   `json:"name"`
	RedirectURIs           []string `json:"redirectUris"`
	PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris,omitempty"`
	AppType                string   `json:"appType"` // "web", "spa", "native"
	DevMode                bool     `json:"devMode"`
	// GrantTypes allows overriding default grant types. If empty, defaults are used.
	// Valid values: "OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN",
	// "OIDC_GRANT_TYPE_DEVICE_CODE"
	GrantTypes []string `json:"grantTypes,omitempty"`
}

// ListApps returns all apps in a project.
func (c *Client) ListApps(ctx context.Context, projectID string) ([]App, error) {
	path := fmt.Sprintf(pathAppsSearch, projectID)
	resp, err := c.request(ctx, "POST", path, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			OIDCConfig *struct {
				ClientID     string   `json:"clientId"`
				RedirectURIs []string `json:"redirectUris"`
			} `json:"oidcConfig"`
			APIConfig *struct {
				ClientID string `json:"clientId"`
			} `json:"apiConfig"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	apps := make([]App, 0, len(result.Result))
	for _, a := range result.Result {
		app := App{ID: a.ID, Name: a.Name}
		if a.OIDCConfig != nil {
			app.Type = "oidc"
			app.ClientID = a.OIDCConfig.ClientID
			app.RedirectURIs = a.OIDCConfig.RedirectURIs
		} else if a.APIConfig != nil {
			app.Type = "api"
			app.ClientID = a.APIConfig.ClientID
		}
		apps = append(apps, app)
	}
	return apps, nil
}

// CreateOIDCApp creates an OIDC application.
func (c *Client) CreateOIDCApp(ctx context.Context, projectID string, cfg OIDCAppConfig) (*App, error) {
	// Map user-friendly app type to Zitadel enum
	var oidcAppType, authMethod string
	switch cfg.AppType {
	case "spa":
		oidcAppType = "OIDC_APP_TYPE_USER_AGENT"
		authMethod = "OIDC_AUTH_METHOD_TYPE_NONE" // PKCE
	case "native":
		oidcAppType = "OIDC_APP_TYPE_NATIVE"
		authMethod = "OIDC_AUTH_METHOD_TYPE_NONE" // PKCE
	default: // "web"
		oidcAppType = "OIDC_APP_TYPE_WEB"
		authMethod = "OIDC_AUTH_METHOD_TYPE_BASIC" // Client secret
	}

	// Use custom grant types if provided, otherwise use defaults
	grantTypes := cfg.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"}
	}

	payload := map[string]interface{}{
		"name":                     cfg.Name,
		"redirectUris":             cfg.RedirectURIs,
		"postLogoutRedirectUris":   cfg.PostLogoutRedirectURIs,
		"responseTypes":            []string{"OIDC_RESPONSE_TYPE_CODE"},
		"grantTypes":               grantTypes,
		"appType":                  oidcAppType,
		"authMethodType":           authMethod,
		"accessTokenType":          "OIDC_TOKEN_TYPE_BEARER",
		"idTokenRoleAssertion":     true,
		"idTokenUserinfoAssertion": true,
		"clockSkew":                "0s",
		"devMode":                  cfg.DevMode,
	}

	path := fmt.Sprintf(pathAppsOIDC, projectID)
	resp, err := c.request(ctx, "POST", path, payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		AppID        string `json:"appId"`
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &App{
		ID:           result.AppID,
		Name:         cfg.Name,
		Type:         "oidc",
		ClientID:     result.ClientID,
		ClientSecret: result.ClientSecret,
		RedirectURIs: cfg.RedirectURIs,
	}, nil
}

// RegenerateClientSecret generates a new client secret for an app.
func (c *Client) RegenerateClientSecret(ctx context.Context, projectID, appID string) (string, error) {
	path := fmt.Sprintf(pathAppSecret, projectID, appID)
	resp, err := c.request(ctx, "POST", path, map[string]interface{}{})
	if err != nil {
		return "", err
	}

	var result struct {
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return result.ClientSecret, nil
}

// DeleteApp deletes an application.
func (c *Client) DeleteApp(ctx context.Context, projectID, appID string) error {
	path := fmt.Sprintf(pathAppByID, projectID, appID)
	_, err := c.request(ctx, "DELETE", path, nil)
	return err
}

// ============================================================================
// User Operations
// ============================================================================

// User represents a Zitadel user.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	DisplayName string `json:"displayName"`
	State       string `json:"state"`
}

// CreateUserConfig holds user creation configuration.
type CreateUserConfig struct {
	Username  string
	Email     string
	FirstName string
	LastName  string
	Password  string
}

// ListUsers returns users matching a query.
func (c *Client) ListUsers(ctx context.Context, query string) ([]User, error) {
	payload := map[string]interface{}{}
	if query != "" {
		payload["queries"] = []map[string]interface{}{
			{
				"userNameQuery": map[string]interface{}{
					"userName": query,
					"method":   "TEXT_QUERY_METHOD_CONTAINS",
				},
			},
		}
	}

	resp, err := c.request(ctx, "POST", pathUsersSearch, payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID       string `json:"id"`
			UserName string `json:"userName"`
			State    string `json:"state"`
			Human    *struct {
				Profile struct {
					FirstName   string `json:"firstName"`
					LastName    string `json:"lastName"`
					DisplayName string `json:"displayName"`
				} `json:"profile"`
				Email struct {
					Email string `json:"email"`
				} `json:"email"`
			} `json:"human"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	users := make([]User, 0, len(result.Result))
	for _, u := range result.Result {
		user := User{ID: u.ID, Username: u.UserName, State: u.State}
		if u.Human != nil {
			user.Email = u.Human.Email.Email
			user.FirstName = u.Human.Profile.FirstName
			user.LastName = u.Human.Profile.LastName
			user.DisplayName = u.Human.Profile.DisplayName
		}
		users = append(users, user)
	}
	return users, nil
}

// GetUserByUsername finds a user by exact username.
func (c *Client) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	resp, err := c.request(ctx, "POST", pathUsersSearch, map[string]interface{}{
		"queries": []map[string]interface{}{
			{
				"userNameQuery": map[string]interface{}{
					"userName": username,
					"method":   "TEXT_QUERY_METHOD_EQUALS",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID       string `json:"id"`
			UserName string `json:"userName"`
			State    string `json:"state"`
			Human    *struct {
				Profile struct {
					FirstName   string `json:"firstName"`
					LastName    string `json:"lastName"`
					DisplayName string `json:"displayName"`
				} `json:"profile"`
				Email struct {
					Email string `json:"email"`
				} `json:"email"`
			} `json:"human"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Result) == 0 {
		return nil, nil
	}

	u := result.Result[0]
	user := &User{ID: u.ID, Username: u.UserName, State: u.State}
	if u.Human != nil {
		user.Email = u.Human.Email.Email
		user.FirstName = u.Human.Profile.FirstName
		user.LastName = u.Human.Profile.LastName
		user.DisplayName = u.Human.Profile.DisplayName
	}
	return user, nil
}

// CreateUser creates a new human user.
func (c *Client) CreateUser(ctx context.Context, cfg CreateUserConfig) (*User, error) {
	payload := map[string]interface{}{
		"userName": cfg.Username,
		"profile": map[string]interface{}{
			"firstName":   cfg.FirstName,
			"lastName":    cfg.LastName,
			"displayName": cfg.FirstName + " " + cfg.LastName,
		},
		"email": map[string]interface{}{
			"email":           cfg.Email,
			"isEmailVerified": true,
		},
		"password": map[string]interface{}{
			"password":       cfg.Password,
			"changeRequired": false,
		},
	}

	resp, err := c.request(ctx, "POST", pathUsersHumanImport, payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &User{
		ID:          result.UserID,
		Username:    cfg.Username,
		Email:       cfg.Email,
		FirstName:   cfg.FirstName,
		LastName:    cfg.LastName,
		DisplayName: cfg.FirstName + " " + cfg.LastName,
		State:       "USER_STATE_ACTIVE",
	}, nil
}

// SetUserPassword sets a user's password.
func (c *Client) SetUserPassword(ctx context.Context, userID, password string) error {
	path := fmt.Sprintf(pathUserPassword, userID)
	_, err := c.request(ctx, "POST", path, map[string]interface{}{
		"password":       password,
		"changeRequired": false,
	})
	return err
}

// DeleteUser deletes a user.
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	path := fmt.Sprintf(pathUserByID, userID)
	_, err := c.request(ctx, "DELETE", path, nil)
	return err
}

// ============================================================================
// Project Role & Grant Operations
// ============================================================================

// Role represents a project role.
type Role struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	Group       string `json:"group"`
}

// AddProjectRole adds a role to a project.
func (c *Client) AddProjectRole(ctx context.Context, projectID string, role Role) error {
	path := fmt.Sprintf(pathProjectRoles, projectID)
	_, err := c.request(ctx, "POST", path, map[string]interface{}{
		"roleKey":     role.Key,
		"displayName": role.DisplayName,
		"group":       role.Group,
	})
	return err
}

// ListProjectRoles lists roles in a project.
func (c *Client) ListProjectRoles(ctx context.Context, projectID string) ([]Role, error) {
	path := fmt.Sprintf(pathProjectRolesSearch, projectID)
	resp, err := c.request(ctx, "POST", path, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			Key         string `json:"key"`
			DisplayName string `json:"displayName"`
			Group       string `json:"group"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	roles := make([]Role, len(result.Result))
	for i, r := range result.Result {
		roles[i] = Role{Key: r.Key, DisplayName: r.DisplayName, Group: r.Group}
	}
	return roles, nil
}

// GrantUserProjectRoles grants project roles to a user.
func (c *Client) GrantUserProjectRoles(ctx context.Context, projectID, userID string, roleKeys []string) error {
	path := fmt.Sprintf(pathUserGrants, userID)
	_, err := c.request(ctx, "POST", path, map[string]interface{}{
		"projectId": projectID,
		"roleKeys":  roleKeys,
	})
	return err
}

// ============================================================================
// Machine User (Service Account) Operations
// ============================================================================

// MachineUser represents a machine user (service account).
type MachineUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	Description string `json:"description"`
	State       string `json:"state"`
}

// ListMachineUsers returns machine users.
func (c *Client) ListMachineUsers(ctx context.Context) ([]MachineUser, error) {
	resp, err := c.request(ctx, "POST", pathUsersSearch, map[string]interface{}{
		"queries": []map[string]interface{}{
			{
				"typeQuery": map[string]interface{}{
					"type": "TYPE_MACHINE",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID       string `json:"id"`
			UserName string `json:"userName"`
			State    string `json:"state"`
			Machine  *struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"machine"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	users := make([]MachineUser, 0, len(result.Result))
	for _, u := range result.Result {
		user := MachineUser{ID: u.ID, Username: u.UserName, State: u.State}
		if u.Machine != nil {
			user.Name = u.Machine.Name
			user.Description = u.Machine.Description
		}
		users = append(users, user)
	}
	return users, nil
}

// GetMachineUserByUsername finds a machine user by username.
func (c *Client) GetMachineUserByUsername(ctx context.Context, username string) (*MachineUser, error) {
	resp, err := c.request(ctx, "POST", pathUsersSearch, map[string]interface{}{
		"queries": []map[string]interface{}{
			{
				"userNameQuery": map[string]interface{}{
					"userName": username,
					"method":   "TEXT_QUERY_METHOD_EQUALS",
				},
			},
			{
				"typeQuery": map[string]interface{}{
					"type": "TYPE_MACHINE",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID       string `json:"id"`
			UserName string `json:"userName"`
			State    string `json:"state"`
			Machine  *struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"machine"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Result) == 0 {
		return nil, nil
	}

	u := result.Result[0]
	user := &MachineUser{ID: u.ID, Username: u.UserName, State: u.State}
	if u.Machine != nil {
		user.Name = u.Machine.Name
		user.Description = u.Machine.Description
	}
	return user, nil
}

// CreateMachineUser creates a machine user (service account).
func (c *Client) CreateMachineUser(ctx context.Context, username, name, description string) (*MachineUser, error) {
	payload := map[string]interface{}{
		"userName":        username,
		"name":            name,
		"description":     description,
		"accessTokenType": "ACCESS_TOKEN_TYPE_BEARER",
	}

	resp, err := c.request(ctx, "POST", pathUsersMachine, payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &MachineUser{
		ID:          result.UserID,
		Username:    username,
		Name:        name,
		Description: description,
		State:       "USER_STATE_ACTIVE",
	}, nil
}

// PAT represents a Personal Access Token.
type PAT struct {
	ID         string `json:"id"`
	Token      string `json:"token,omitempty"`
	Expiration string `json:"expirationDate"`
}

// ListPATs lists PATs for a user.
func (c *Client) ListPATs(ctx context.Context, userID string) ([]PAT, error) {
	path := fmt.Sprintf(pathUserPATsSearch, userID)
	resp, err := c.request(ctx, "POST", path, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Result []struct {
			ID             string `json:"id"`
			ExpirationDate string `json:"expirationDate"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	pats := make([]PAT, len(result.Result))
	for i, p := range result.Result {
		pats[i] = PAT{ID: p.ID, Expiration: p.ExpirationDate}
	}
	return pats, nil
}

// CreatePAT creates a Personal Access Token for a user.
func (c *Client) CreatePAT(ctx context.Context, userID string, expirationDays int) (*PAT, error) {
	expiration := time.Now().AddDate(0, 0, expirationDays).Format(time.RFC3339)

	path := fmt.Sprintf(pathUserPATs, userID)
	resp, err := c.request(ctx, "POST", path, map[string]interface{}{
		"expirationDate": expiration,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		TokenID string `json:"tokenId"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &PAT{
		ID:         result.TokenID,
		Token:      result.Token,
		Expiration: expiration,
	}, nil
}

// DeletePAT deletes a PAT.
func (c *Client) DeletePAT(ctx context.Context, userID, patID string) error {
	path := fmt.Sprintf(pathUserPATByID, userID, patID)
	_, err := c.request(ctx, "DELETE", path, nil)
	return err
}

// CreateMachineUserPAT is a convenience method that creates a PAT for a machine user.
//
// Deprecated: Use CreatePAT instead.
func (c *Client) CreateMachineUserPAT(ctx context.Context, userID string, expirationDays int) (string, error) {
	pat, err := c.CreatePAT(ctx, userID, expirationDays)
	if err != nil {
		return "", err
	}
	return pat.Token, nil
}

// ============================================================================
// OAuth Operations
// ============================================================================

// ExchangeJWTForAccessToken exchanges a JWT assertion for an OAuth access token.
func (c *Client) ExchangeJWTForAccessToken(ctx context.Context, jwtAssertion string) (string, error) {
	resp, err := c.requestOAuth(ctx, "/oauth/v2/token", map[string]string{
		"grant_type": "urn:ietf:params:oauth:grant-type:jwt-bearer",
		"scope":       "openid urn:zitadel:iam:org:project:id:zitadel:aud",
		"assertion":   jwtAssertion,
	})
	if err != nil {
		return "", err
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if result.AccessToken == "" {
		return "", fmt.Errorf("no access token in response: %s", string(resp))
	}

	return result.AccessToken, nil
}

// CreatePATWithAccessToken creates a PAT using an OAuth access token (not a PAT).
func (c *Client) CreatePATWithAccessToken(ctx context.Context, accessToken, userID string, expirationDate string) (*PAT, error) {
	path := fmt.Sprintf(pathUserPATs, userID)
	resp, err := c.requestWithToken(ctx, "POST", path, accessToken, map[string]interface{}{
		"expirationDate": expirationDate,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		TokenID string `json:"tokenId"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &PAT{
		ID:         result.TokenID,
		Token:      result.Token,
		Expiration: expirationDate,
	}, nil
}

// ============================================================================
// Instance / Admin Operations
// ============================================================================

// GetMyOrg returns the current user's organization.
func (c *Client) GetMyOrg(ctx context.Context) (*Org, error) {
	resp, err := c.request(ctx, "GET", pathMyOrg, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &Org{ID: result.ID, Name: result.Name, State: result.State}, nil
}

// Healthz checks if Zitadel is healthy.
func (c *Client) Healthz(ctx context.Context) error {
	_, err := c.request(ctx, "GET", pathHealthz, nil)
	return err
}
