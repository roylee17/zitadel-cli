package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var projectGrantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Manage project grants",
	Long: `Commands for managing project grants (Provider-Consumer model).

Project grants allow a provider organization to share a project with a
customer organization. Users in the customer org can then be granted
roles on that project.

Examples:
  # Grant "TMS" to "Acme Trucking" with specific roles
  zitadel-cli project grant create \
    --project "TMS" \
    --to-org "Acme Trucking" \
    --roles admin,dispatcher,driver,viewer

  # List all grants for a project
  zitadel-cli project grant list --project "TMS"

  # Delete a grant
  zitadel-cli project grant delete <grant-id> --project "TMS"`,
}

var (
	projectGrantProject string // --project flag
	projectGrantToOrg   string // --to-org flag
	projectGrantRoles   string // --roles flag (comma-separated)
)

var projectGrantCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Grant a project to another organization",
	Long: `Grant a project to another organization (customer org).

This allows users in the target organization to be granted roles
on the project's applications.

Examples:
  # Grant with specific roles
  zitadel-cli project grant create \
    --project "TMS" \
    --to-org "Acme Trucking" \
    --roles admin,dispatcher,driver,viewer

  # Grant all roles (when --roles is omitted, all project roles are included)
  zitadel-cli project grant create \
    --project "TMS" \
    --to-org "Acme Trucking"`,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx := context.Background()

		if projectGrantProject == "" {
			return fmt.Errorf("--project is required")
		}
		if projectGrantToOrg == "" {
			return fmt.Errorf("--to-org is required")
		}

		// Resolve project name to ID
		project, err := apiClient.GetProjectByName(ctx, projectGrantProject)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", projectGrantProject)
		}

		// Resolve org name to ID
		org, err := apiClient.GetOrgByName(ctx, projectGrantToOrg)
		if err != nil {
			return fmt.Errorf("failed to get organization: %w", err)
		}
		if org == nil {
			return fmt.Errorf("organization '%s' not found", projectGrantToOrg)
		}

		// Parse roles
		var roleKeys []string
		if projectGrantRoles != "" {
			roleKeys = strings.Split(projectGrantRoles, ",")
			for i, r := range roleKeys {
				roleKeys[i] = strings.TrimSpace(r)
			}
		} else {
			// If no roles specified, get all project roles
			roles, err := apiClient.ListProjectRoles(ctx, project.ID)
			if err != nil {
				return fmt.Errorf("failed to list project roles: %w", err)
			}
			for _, r := range roles {
				roleKeys = append(roleKeys, r.Key)
			}
		}

		// Create the grant
		grant, err := apiClient.CreateProjectGrant(ctx, project.ID, org.ID, roleKeys)
		if err != nil {
			return fmt.Errorf("failed to create project grant: %w", err)
		}

		printer.Success("Project grant created (ID: %s)", grant.ID)
		return printer.PrintObject(grant)
	},
}

var projectGrantListCmd = &cobra.Command{
	Use:   "list",
	Short: "List project grants",
	Long: `List all grants for a project.

Examples:
  zitadel-cli project grant list --project "TMS"
  zitadel-cli project grant list --project "TMS" -o json`,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx := context.Background()

		if projectGrantProject == "" {
			return fmt.Errorf("--project is required")
		}

		// Resolve project name to ID
		project, err := apiClient.GetProjectByName(ctx, projectGrantProject)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", projectGrantProject)
		}

		grants, err := apiClient.ListProjectGrants(ctx, project.ID)
		if err != nil {
			return fmt.Errorf("failed to list project grants: %w", err)
		}

		if len(grants) == 0 {
			printer.Info("No grants found for project '%s'", projectGrantProject)
			return nil
		}

		return printer.PrintObject(grants)
	},
}

var projectGrantGetCmd = &cobra.Command{
	Use:   "get <grant-id>",
	Short: "Get a specific project grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if projectGrantProject == "" {
			return fmt.Errorf("--project is required")
		}

		// Resolve project name to ID
		project, err := apiClient.GetProjectByName(ctx, projectGrantProject)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", projectGrantProject)
		}

		grant, err := apiClient.GetProjectGrant(ctx, project.ID, grantID)
		if err != nil {
			return fmt.Errorf("failed to get project grant: %w", err)
		}

		return printer.PrintObject(grant)
	},
}

var projectGrantUpdateCmd = &cobra.Command{ //nolint:dupl // similar structure to userGrantUpdateCmd but different entity
	Use:   "update <grant-id>",
	Short: "Update roles on a project grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if projectGrantProject == "" {
			return fmt.Errorf("--project is required")
		}
		if projectGrantRoles == "" {
			return fmt.Errorf("--roles is required")
		}

		// Resolve project name to ID
		project, err := apiClient.GetProjectByName(ctx, projectGrantProject)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", projectGrantProject)
		}

		// Parse roles
		roleKeys := strings.Split(projectGrantRoles, ",")
		for i, r := range roleKeys {
			roleKeys[i] = strings.TrimSpace(r)
		}

		err = apiClient.UpdateProjectGrantRoles(ctx, project.ID, grantID, roleKeys)
		if err != nil {
			return fmt.Errorf("failed to update project grant: %w", err)
		}

		fmt.Printf("Project grant %s updated with roles: %s\n", grantID, strings.Join(roleKeys, ", "))
		return nil
	},
}

var projectGrantDeleteCmd = &cobra.Command{
	Use:   "delete <grant-id>",
	Short: "Delete (revoke) a project grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if projectGrantProject == "" {
			return fmt.Errorf("--project is required")
		}

		// Resolve project name to ID
		project, err := apiClient.GetProjectByName(ctx, projectGrantProject)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", projectGrantProject)
		}

		err = apiClient.DeleteProjectGrant(ctx, project.ID, grantID)
		if err != nil {
			return fmt.Errorf("failed to delete project grant: %w", err)
		}

		fmt.Printf("Project grant %s deleted\n", grantID)
		return nil
	},
}

var projectGrantDeactivateCmd = &cobra.Command{
	Use:   "deactivate <grant-id>",
	Short: "Deactivate a project grant (soft disable)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if projectGrantProject == "" {
			return fmt.Errorf("--project is required")
		}

		// Resolve project name to ID
		project, err := apiClient.GetProjectByName(ctx, projectGrantProject)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", projectGrantProject)
		}

		err = apiClient.DeactivateProjectGrant(ctx, project.ID, grantID)
		if err != nil {
			return fmt.Errorf("failed to deactivate project grant: %w", err)
		}

		fmt.Printf("Project grant %s deactivated\n", grantID)
		return nil
	},
}

var projectGrantReactivateCmd = &cobra.Command{
	Use:   "reactivate <grant-id>",
	Short: "Reactivate a deactivated project grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if projectGrantProject == "" {
			return fmt.Errorf("--project is required")
		}

		// Resolve project name to ID
		project, err := apiClient.GetProjectByName(ctx, projectGrantProject)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", projectGrantProject)
		}

		err = apiClient.ReactivateProjectGrant(ctx, project.ID, grantID)
		if err != nil {
			return fmt.Errorf("failed to reactivate project grant: %w", err)
		}

		fmt.Printf("Project grant %s reactivated\n", grantID)
		return nil
	},
}

func init() {
	// Add grant subcommand to project command
	projectCmd.AddCommand(projectGrantCmd)

	// Flags for create
	projectGrantCreateCmd.Flags().StringVar(&projectGrantProject, "project", "", "Project name (required)")
	projectGrantCreateCmd.Flags().StringVar(&projectGrantToOrg, "to-org", "", "Target organization name (required)")
	projectGrantCreateCmd.Flags().StringVar(&projectGrantRoles, "roles", "", "Comma-separated role keys (optional, defaults to all roles)")

	// Flags for list
	projectGrantListCmd.Flags().StringVar(&projectGrantProject, "project", "", "Project name (required)")

	// Flags for get
	projectGrantGetCmd.Flags().StringVar(&projectGrantProject, "project", "", "Project name (required)")

	// Flags for update
	projectGrantUpdateCmd.Flags().StringVar(&projectGrantProject, "project", "", "Project name (required)")
	projectGrantUpdateCmd.Flags().StringVar(&projectGrantRoles, "roles", "", "Comma-separated role keys (required)")

	// Flags for delete
	projectGrantDeleteCmd.Flags().StringVar(&projectGrantProject, "project", "", "Project name (required)")

	// Flags for deactivate/reactivate
	projectGrantDeactivateCmd.Flags().StringVar(&projectGrantProject, "project", "", "Project name (required)")
	projectGrantReactivateCmd.Flags().StringVar(&projectGrantProject, "project", "", "Project name (required)")

	// Add subcommands
	projectGrantCmd.AddCommand(projectGrantCreateCmd)
	projectGrantCmd.AddCommand(projectGrantListCmd)
	projectGrantCmd.AddCommand(projectGrantGetCmd)
	projectGrantCmd.AddCommand(projectGrantUpdateCmd)
	projectGrantCmd.AddCommand(projectGrantDeleteCmd)
	projectGrantCmd.AddCommand(projectGrantDeactivateCmd)
	projectGrantCmd.AddCommand(projectGrantReactivateCmd)
}
