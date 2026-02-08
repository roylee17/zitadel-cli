package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/roylee17/zitadel-cli/internal/client"
	"github.com/roylee17/zitadel-cli/internal/output"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
	Long:  "Commands for managing human and machine users.",
}

var (
	userQuery     string
	userEmail     string
	userFirstName string
	userLastName  string
	userPassword  string
)

var userListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List users",
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx, cancel := commandContext()
		defer cancel()

		spin := output.NewSpinner("Fetching users...")
		spin.Start()

		users, err := apiClient.ListUsers(ctx, userQuery)
		spin.Stop()
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		if len(users) == 0 {
			printer.Info("No users found")
			return nil
		}

		return output.PrintTable(printer, []string{"ID", "USERNAME", "EMAIL", "NAME", "STATE"}, users, func(u client.User) []string {
			return []string{u.ID, u.Username, u.Email, u.DisplayName, u.State}
		})
	},
}

var userGetCmd = &cobra.Command{
	Use:   "get <username>",
	Short: "Get user by username",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx, cancel := commandContext()
		defer cancel()

		spin := output.NewSpinner("Fetching user...")
		spin.Start()

		user, err := apiClient.GetUserByUsername(ctx, args[0])
		spin.Stop()
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if user == nil {
			return fmt.Errorf("user '%s' not found", args[0])
		}

		return printer.PrintObject(user)
	},
}

var userCreateCmd = &cobra.Command{
	Use:   "create <username>",
	Short: "Create a new human user",
	Long: `Create a new human user.

Examples:
  # Create a user with all details
  zitadel-cli user create admin \
    --email admin@example.com \
    --first-name Admin \
    --last-name User \
    --password Password123!

  # Create a user (will prompt for password if not provided)
  zitadel-cli user create developer \
    --email dev@example.com \
    --first-name Dev \
    --last-name User`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx, cancel := commandContext()
		defer cancel()

		if userEmail == "" {
			return fmt.Errorf("--email is required")
		}

		// Prompt for password if not provided
		password := userPassword
		if password == "" {
			var err error
			password, err = output.PromptPassword("Password")
			if err != nil {
				return fmt.Errorf("prompt for password: %w", err)
			}
		}

		// Default first/last name from username if not provided
		firstName := userFirstName
		lastName := userLastName
		if firstName == "" {
			firstName = args[0]
		}
		if lastName == "" {
			lastName = "User"
		}

		spin := output.NewSpinner("Checking existing user...")
		spin.Start()

		// Check if user already exists
		existing, err := apiClient.GetUserByUsername(ctx, args[0])
		if err != nil {
			spin.Stop()
			return fmt.Errorf("failed to check existing user: %w", err)
		}
		if existing != nil {
			spin.Stop()
			printer.Warning("User '%s' already exists (ID: %s)", args[0], existing.ID)
			return printer.PrintObject(existing)
		}

		spin.UpdateMessage("Creating user...")
		cfg := client.CreateUserConfig{
			Username:  args[0],
			Email:     userEmail,
			FirstName: firstName,
			LastName:  lastName,
			Password:  password,
		}

		user, err := apiClient.CreateUser(ctx, cfg)
		spin.Stop()
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		printer.Success("User '%s' created", user.Username)
		return printer.PrintObject(user)
	},
}

var userSetPasswordCmd = &cobra.Command{
	Use:   "set-password <user-id>",
	Short: "Set user password",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx, cancel := commandContext()
		defer cancel()

		// Prompt for password if not provided
		password := userPassword
		if password == "" {
			var err error
			password, err = output.PromptPassword("New Password")
			if err != nil {
				return fmt.Errorf("prompt for password: %w", err)
			}
		}

		spin := output.NewSpinner("Setting password...")
		spin.Start()

		err := apiClient.SetUserPassword(ctx, args[0], password)
		spin.Stop()
		if err != nil {
			return fmt.Errorf("failed to set password: %w", err)
		}

		printer.Success("Password updated for user '%s'", args[0])
		return nil
	},
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete <user-id>",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !confirmDanger(fmt.Sprintf("Delete user '%s'?", args[0])) {
			printer.Info("Cancelled")
			return nil
		}

		ctx, cancel := commandContext()
		defer cancel()

		spin := output.NewSpinner("Deleting user...")
		spin.Start()

		err := apiClient.DeleteUser(ctx, args[0])
		spin.Stop()
		if err != nil {
			return fmt.Errorf("failed to delete user: %w", err)
		}

		printer.Success("User '%s' deleted", args[0])
		return nil
	},
}

func init() {
	// List command flags
	userListCmd.Flags().StringVarP(&userQuery, "query", "q", "", "Search query (username contains)")

	// Create command flags
	userCreateCmd.Flags().StringVar(&userEmail, "email", "", "User email (required)")
	userCreateCmd.Flags().StringVar(&userFirstName, "first-name", "", "First name")
	userCreateCmd.Flags().StringVar(&userLastName, "last-name", "", "Last name")
	userCreateCmd.Flags().StringVar(&userPassword, "password", "", "Password")

	// Set password command flags
	userSetPasswordCmd.Flags().StringVar(&userPassword, "password", "", "New password")

	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userGetCmd)
	userCmd.AddCommand(userCreateCmd)
	userCmd.AddCommand(userSetPasswordCmd)
	userCmd.AddCommand(userDeleteCmd)
	userCmd.AddCommand(userGrantCmd)
}
