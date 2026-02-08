package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var userGrantCmd = &cobra.Command{
	Use:   "grant",
	Short: "Manage user grants",
	Long: `Commands for managing user grants (project role assignments).

User grants assign project roles to users. This can be used for:
- Internal grants: Users in the same org as the project
- External grants: Users from a different org (via project grants)

Examples:
  # Grant roles to a user in your org
  zitadel-cli user grant create --user admin@acme.com \
    --project "TMS" \
    --roles admin,dispatcher

  # Grant roles to an external user (support staff accessing customer org)
  zitadel-cli user grant create --user support@ironkernel.io \
    --to-org "Acme Trucking" \
    --project "TMS" \
    --roles support-admin

  # List grants for a user
  zitadel-cli user grant list --user admin@acme.com

  # Delete a grant
  zitadel-cli user grant delete <grant-id> --user admin@acme.com`,
}

var (
	userGrantUser      string // --user flag (username/email)
	userGrantProject   string // --project flag
	userGrantToOrg     string // --to-org flag (for external grants)
	userGrantRoles     string // --roles flag (comma-separated)
	userGrantProjectID string // --project-id flag (for filtering)
)

var userGrantCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Grant project roles to a user",
	Long: `Grant project roles to a user.

For internal users (same org as project):
  zitadel-cli user grant create --user admin@acme.com \
    --project "TMS" \
    --roles admin,dispatcher

For external users (granting access to a different org):
  zitadel-cli user grant create --user support@ironkernel.io \
    --to-org "Acme Trucking" \
    --project "TMS" \
    --roles support-admin`,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx := context.Background()

		if userGrantUser == "" {
			return fmt.Errorf("--user is required")
		}
		if userGrantProject == "" {
			return fmt.Errorf("--project is required")
		}
		if userGrantRoles == "" {
			return fmt.Errorf("--roles is required")
		}

		// Resolve user
		user, err := apiClient.GetUserByUsername(ctx, userGrantUser)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if user == nil {
			return fmt.Errorf("user '%s' not found", userGrantUser)
		}

		// Resolve project
		project, err := apiClient.GetProjectByName(ctx, userGrantProject)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", userGrantProject)
		}

		// Parse roles
		roleKeys := strings.Split(userGrantRoles, ",")
		for i, r := range roleKeys {
			roleKeys[i] = strings.TrimSpace(r)
		}

		// For external grants, we need to find the project grant
		var grantID string
		if userGrantToOrg != "" {
			// Resolve target org
			org, err := apiClient.GetOrgByName(ctx, userGrantToOrg)
			if err != nil {
				return fmt.Errorf("failed to get target organization: %w", err)
			}
			if org == nil {
				return fmt.Errorf("organization '%s' not found", userGrantToOrg)
			}

			// Find the project grant for this org
			grants, err := apiClient.ListProjectGrants(ctx, project.ID)
			if err != nil {
				return fmt.Errorf("failed to list project grants: %w", err)
			}

			for _, g := range grants {
				if g.GrantedOrgID == org.ID {
					grantID = g.ID
					break
				}
			}

			if grantID == "" {
				return fmt.Errorf("no project grant found for project '%s' to org '%s'", userGrantProject, userGrantToOrg)
			}
		}

		// Create the user grant
		grant, err := apiClient.CreateUserGrant(ctx, user.ID, project.ID, roleKeys, grantID)
		if err != nil {
			return fmt.Errorf("failed to create user grant: %w", err)
		}

		printer.Success("User grant created (ID: %s)", grant.ID)
		return printer.PrintObject(grant)
	},
}

var userGrantListCmd = &cobra.Command{
	Use:   "list",
	Short: "List user grants",
	Long: `List grants for a user or search across all users.

Examples:
  # List grants for a specific user
  zitadel-cli user grant list --user admin@acme.com

  # List all grants for a project
  zitadel-cli user grant list --project-id <project-id>

  # List all grants
  zitadel-cli user grant list`,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx := context.Background()

		var grants []struct {
			ID        string
			UserID    string
			ProjectID string
			RoleKeys  []string
			State     string
			OrgID     string
			GrantID   string
		}

		if userGrantUser != "" {
			// List grants for a specific user
			user, err := apiClient.GetUserByUsername(ctx, userGrantUser)
			if err != nil {
				return fmt.Errorf("failed to get user: %w", err)
			}
			if user == nil {
				return fmt.Errorf("user '%s' not found", userGrantUser)
			}

			userGrants, err := apiClient.ListUserGrants(ctx, user.ID)
			if err != nil {
				return fmt.Errorf("failed to list user grants: %w", err)
			}

			for _, g := range userGrants {
				grants = append(grants, struct {
					ID        string
					UserID    string
					ProjectID string
					RoleKeys  []string
					State     string
					OrgID     string
					GrantID   string
				}{g.ID, g.UserID, g.ProjectID, g.RoleKeys, g.State, g.OrgID, g.GrantID})
			}
		} else {
			// Search all grants with optional filters
			searchGrants, err := apiClient.SearchUserGrants(ctx, userGrantProjectID, "")
			if err != nil {
				return fmt.Errorf("failed to search user grants: %w", err)
			}

			for _, g := range searchGrants {
				grants = append(grants, struct {
					ID        string
					UserID    string
					ProjectID string
					RoleKeys  []string
					State     string
					OrgID     string
					GrantID   string
				}{g.ID, g.UserID, g.ProjectID, g.RoleKeys, g.State, g.OrgID, g.GrantID})
			}
		}

		if len(grants) == 0 {
			printer.Info("No user grants found")
			return nil
		}

		return printer.PrintObject(grants)
	},
}

var userGrantGetCmd = &cobra.Command{
	Use:   "get <grant-id>",
	Short: "Get a specific user grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if userGrantUser == "" {
			return fmt.Errorf("--user is required")
		}

		// Resolve user
		user, err := apiClient.GetUserByUsername(ctx, userGrantUser)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if user == nil {
			return fmt.Errorf("user '%s' not found", userGrantUser)
		}

		grant, err := apiClient.GetUserGrant(ctx, user.ID, grantID)
		if err != nil {
			return fmt.Errorf("failed to get user grant: %w", err)
		}

		return printer.PrintObject(grant)
	},
}

var userGrantUpdateCmd = &cobra.Command{ //nolint:dupl // similar structure to projectGrantUpdateCmd but different entity
	Use:   "update <grant-id>",
	Short: "Update roles on a user grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if userGrantUser == "" {
			return fmt.Errorf("--user is required")
		}
		if userGrantRoles == "" {
			return fmt.Errorf("--roles is required")
		}

		// Resolve user
		user, err := apiClient.GetUserByUsername(ctx, userGrantUser)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if user == nil {
			return fmt.Errorf("user '%s' not found", userGrantUser)
		}

		// Parse roles
		roleKeys := strings.Split(userGrantRoles, ",")
		for i, r := range roleKeys {
			roleKeys[i] = strings.TrimSpace(r)
		}

		err = apiClient.UpdateUserGrantRoles(ctx, user.ID, grantID, roleKeys)
		if err != nil {
			return fmt.Errorf("failed to update user grant: %w", err)
		}

		fmt.Printf("User grant %s updated with roles: %s\n", grantID, strings.Join(roleKeys, ", "))
		return nil
	},
}

var userGrantDeleteCmd = &cobra.Command{
	Use:   "delete <grant-id>",
	Short: "Delete (revoke) a user grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if userGrantUser == "" {
			return fmt.Errorf("--user is required")
		}

		// Resolve user
		user, err := apiClient.GetUserByUsername(ctx, userGrantUser)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if user == nil {
			return fmt.Errorf("user '%s' not found", userGrantUser)
		}

		err = apiClient.DeleteUserGrant(ctx, user.ID, grantID)
		if err != nil {
			return fmt.Errorf("failed to delete user grant: %w", err)
		}

		fmt.Printf("User grant %s deleted\n", grantID)
		return nil
	},
}

var userGrantDeactivateCmd = &cobra.Command{
	Use:   "deactivate <grant-id>",
	Short: "Deactivate a user grant (soft disable)",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if userGrantUser == "" {
			return fmt.Errorf("--user is required")
		}

		// Resolve user
		user, err := apiClient.GetUserByUsername(ctx, userGrantUser)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if user == nil {
			return fmt.Errorf("user '%s' not found", userGrantUser)
		}

		err = apiClient.DeactivateUserGrant(ctx, user.ID, grantID)
		if err != nil {
			return fmt.Errorf("failed to deactivate user grant: %w", err)
		}

		fmt.Printf("User grant %s deactivated\n", grantID)
		return nil
	},
}

var userGrantReactivateCmd = &cobra.Command{
	Use:   "reactivate <grant-id>",
	Short: "Reactivate a deactivated user grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		grantID := args[0]

		if userGrantUser == "" {
			return fmt.Errorf("--user is required")
		}

		// Resolve user
		user, err := apiClient.GetUserByUsername(ctx, userGrantUser)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if user == nil {
			return fmt.Errorf("user '%s' not found", userGrantUser)
		}

		err = apiClient.ReactivateUserGrant(ctx, user.ID, grantID)
		if err != nil {
			return fmt.Errorf("failed to reactivate user grant: %w", err)
		}

		fmt.Printf("User grant %s reactivated\n", grantID)
		return nil
	},
}

func init() {
	// Add grant subcommand to user command
	userCmd.AddCommand(userGrantCmd)

	// Flags for create
	userGrantCreateCmd.Flags().StringVar(&userGrantUser, "user", "", "Username/email (required)")
	userGrantCreateCmd.Flags().StringVar(&userGrantProject, "project", "", "Project name (required)")
	userGrantCreateCmd.Flags().StringVar(&userGrantToOrg, "to-org", "", "Target organization for external grants")
	userGrantCreateCmd.Flags().StringVar(&userGrantRoles, "roles", "", "Comma-separated role keys (required)")

	// Flags for list
	userGrantListCmd.Flags().StringVar(&userGrantUser, "user", "", "Username/email (optional)")
	userGrantListCmd.Flags().StringVar(&userGrantProjectID, "project-id", "", "Filter by project ID")

	// Flags for get
	userGrantGetCmd.Flags().StringVar(&userGrantUser, "user", "", "Username/email (required)")

	// Flags for update
	userGrantUpdateCmd.Flags().StringVar(&userGrantUser, "user", "", "Username/email (required)")
	userGrantUpdateCmd.Flags().StringVar(&userGrantRoles, "roles", "", "Comma-separated role keys (required)")

	// Flags for delete
	userGrantDeleteCmd.Flags().StringVar(&userGrantUser, "user", "", "Username/email (required)")

	// Flags for deactivate/reactivate
	userGrantDeactivateCmd.Flags().StringVar(&userGrantUser, "user", "", "Username/email (required)")
	userGrantReactivateCmd.Flags().StringVar(&userGrantUser, "user", "", "Username/email (required)")

	// Add subcommands
	userGrantCmd.AddCommand(userGrantCreateCmd)
	userGrantCmd.AddCommand(userGrantListCmd)
	userGrantCmd.AddCommand(userGrantGetCmd)
	userGrantCmd.AddCommand(userGrantUpdateCmd)
	userGrantCmd.AddCommand(userGrantDeleteCmd)
	userGrantCmd.AddCommand(userGrantDeactivateCmd)
	userGrantCmd.AddCommand(userGrantReactivateCmd)
}
