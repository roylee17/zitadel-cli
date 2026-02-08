package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var oidcCmd = &cobra.Command{
	Use:   "oidc",
	Short: "OIDC-related commands",
	Long:  "Commands for managing and exporting OIDC credentials.",
}

// OIDCExportResult holds the exported OIDC credentials for piping to Vault.
type OIDCExportResult struct {
	FrontendClientID    string `json:"frontend_client_id"`
	BackendClientID     string `json:"backend_client_id"`
	BackendClientSecret string `json:"backend_client_secret,omitempty"`
	ProjectID           string `json:"project_id"`
}

var oidcExportRegenSecret bool

var oidcExportCmd = &cobra.Command{
	Use:   "export <project-name>",
	Short: "Export OIDC credentials for existing apps",
	Long: `Export OIDC credentials for existing frontend and backend apps.

This command reads existing OIDC app configurations from Zitadel and outputs
the credentials. Useful for disaster recovery, migration, or re-syncing to Vault.

By default, the backend client secret is NOT regenerated. Use --regenerate-secret
to generate a new secret (this invalidates the old secret).

Examples:
  # Export credentials to stdout (human-readable)
  zitadel-cli oidc export axialops

  # Export credentials as JSON for piping to Vault
  zitadel-cli oidc export axialops -o json | vault kv put secret/apps/axialops/oidc -

  # Export with new backend secret (invalidates old secret)
  zitadel-cli oidc export axialops --regenerate-secret -o json`,
	Args: cobra.ExactArgs(1),
	RunE: runOIDCExport,
}

func runOIDCExport(_ *cobra.Command, args []string) error {
	ctx := context.Background()
	projectName := args[0]

	// Find project
	project, err := apiClient.GetProjectByName(ctx, projectName)
	if err != nil {
		return fmt.Errorf("failed to find project: %w", err)
	}
	if project == nil {
		return fmt.Errorf("project '%s' not found", projectName)
	}

	// List apps in project
	apps, err := apiClient.ListApps(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	// Find frontend and backend apps
	frontendAppName := projectName + "-frontend"
	backendAppName := projectName + "-backend"

	result := &OIDCExportResult{
		ProjectID: project.ID,
	}

	for _, app := range apps {
		if app.Name == frontendAppName {
			result.FrontendClientID = app.ClientID
		}
		if app.Name == backendAppName {
			result.BackendClientID = app.ClientID

			// Regenerate secret if requested
			if oidcExportRegenSecret {
				secret, err := apiClient.RegenerateClientSecret(ctx, project.ID, app.ID)
				if err != nil {
					return fmt.Errorf("failed to regenerate backend secret: %w", err)
				}
				result.BackendClientSecret = secret
			}
		}
	}

	// Validate we found the apps
	if result.FrontendClientID == "" {
		return fmt.Errorf("frontend app '%s' not found in project", frontendAppName)
	}
	if result.BackendClientID == "" {
		return fmt.Errorf("backend app '%s' not found in project", backendAppName)
	}

	// Output using printer
	return printer.PrintObject(result)
}

func init() {
	oidcExportCmd.Flags().BoolVar(&oidcExportRegenSecret, "regenerate-secret", false, "Regenerate backend client secret (invalidates old secret)")

	oidcCmd.AddCommand(oidcExportCmd)
}
