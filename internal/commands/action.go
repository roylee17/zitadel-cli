package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/roylee17/zitadel-cli/internal/client"
)

var actionCmd = &cobra.Command{
	Use:   "action",
	Short: "Manage Zitadel Actions",
	Long: `Commands for managing Zitadel Actions (JavaScript functions that run during auth flows).

Actions can be triggered at various points in the authentication process to:
  - Add custom claims to tokens
  - Transform user data
  - Validate business rules
  - Integrate with external systems`,
}

var (
	actionScript    string
	actionScriptArg string
	actionTimeout   string
	actionAllowFail bool
)

var actionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all actions",
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx := context.Background()

		actions, err := apiClient.ListActions(ctx)
		if err != nil {
			return fmt.Errorf("failed to list actions: %w", err)
		}

		// Use printer for automatic format handling
		return printer.PrintObject(actions)
	},
}

var actionGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get action by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()

		action, err := apiClient.GetActionByName(ctx, args[0])
		if err != nil {
			return fmt.Errorf("failed to get action: %w", err)
		}
		if action == nil {
			return fmt.Errorf("action '%s' not found", args[0])
		}

		return printer.PrintObject(action)
	},
}

var actionCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create an action",
	Long: `Create a new Zitadel Action with JavaScript code.

The script can be provided inline via --script or from a file via --script-file.

Examples:
  # Create action from inline script
  zitadel-cli action create addCustomClaim \
    --script 'function addCustomClaim(ctx, api) { api.v1.claims.setClaim("custom_key", "custom_value"); }'

  # Create action from file
  zitadel-cli action create myAction \
    --script-file ./actions/my-action.js`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()

		script := actionScriptArg
		if actionScript != "" {
			// Read script from file
			content, err := os.ReadFile(actionScript) //nolint:gosec // user-provided flag path is intentional
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			script = string(content)
		}

		if script == "" {
			return fmt.Errorf("either --script or --script-file is required")
		}

		cfg := client.CreateActionConfig{
			Name:          args[0],
			Script:        script,
			Timeout:       actionTimeout,
			AllowedToFail: actionAllowFail,
		}

		action, err := apiClient.CreateAction(ctx, cfg)
		if err != nil {
			return fmt.Errorf("failed to create action: %w", err)
		}

		printer.Success("Action '%s' created (ID: %s)", action.Name, action.ID)
		return printer.PrintObject(action)
	},
}

var actionUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update an action",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()

		// First, find the action by name
		action, err := apiClient.GetActionByName(ctx, args[0])
		if err != nil {
			return fmt.Errorf("failed to find action: %w", err)
		}
		if action == nil {
			return fmt.Errorf("action '%s' not found", args[0])
		}

		script := actionScriptArg
		if actionScript != "" {
			content, err := os.ReadFile(actionScript) //nolint:gosec // user-provided flag path is intentional
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			script = string(content)
		}

		if script == "" {
			script = action.Script
		}

		timeout := actionTimeout
		if timeout == "" {
			timeout = action.Timeout
		}

		cfg := client.CreateActionConfig{
			Name:          args[0],
			Script:        script,
			Timeout:       timeout,
			AllowedToFail: actionAllowFail,
		}

		err = apiClient.UpdateAction(ctx, action.ID, cfg)
		if err != nil {
			return fmt.Errorf("failed to update action: %w", err)
		}

		printer.Success("Action '%s' updated", args[0])
		return nil
	},
}

var actionDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete an action",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()

		action, err := apiClient.GetActionByName(ctx, args[0])
		if err != nil {
			return fmt.Errorf("failed to find action: %w", err)
		}
		if action == nil {
			return fmt.Errorf("action '%s' not found", args[0])
		}

		err = apiClient.DeleteAction(ctx, action.ID)
		if err != nil {
			return fmt.Errorf("failed to delete action: %w", err)
		}

		printer.Success("Action '%s' deleted", args[0])
		return nil
	},
}

// Deploy-related variables
var (
	deployFlowType    string
	deployTriggerType string
)

var actionDeployCmd = &cobra.Command{
	Use:   "deploy <action-name>",
	Short: "Deploy an action to a trigger",
	Long: `Deploy an action to a specific authentication flow trigger.

Flow types:
  - external-authentication: User authentication via external IdP
  - complement-token: Token creation/modification

Trigger types for complement-token flow:
  - pre-access-token-creation: Before access token is created

Examples:
  # Deploy action to run before access token creation
  zitadel-cli action deploy addCanonicalTenant \
    --flow complement-token \
    --trigger pre-access-token-creation`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx := context.Background()
		actionName := args[0]

		// Find the action
		action, err := apiClient.GetActionByName(ctx, actionName)
		if err != nil {
			return fmt.Errorf("failed to find action: %w", err)
		}
		if action == nil {
			return fmt.Errorf("action '%s' not found", actionName)
		}

		// Map flow type string to enum
		flowType, err := parseFlowType(deployFlowType)
		if err != nil {
			return err
		}

		// Map trigger type string to enum
		triggerType, err := parseTriggerType(deployTriggerType, flowType)
		if err != nil {
			return err
		}

		// Deploy the action
		err = apiClient.SetActionOnTrigger(ctx, flowType, triggerType, []string{action.ID})
		if err != nil {
			return fmt.Errorf("failed to deploy action: %w", err)
		}

		printer.Success("Action '%s' deployed to %s/%s", actionName, deployFlowType, deployTriggerType)
		return nil
	},
}

var actionUndeployCmd = &cobra.Command{
	Use:   "undeploy",
	Short: "Remove all actions from a trigger",
	Long: `Remove all actions from a specific authentication flow trigger.

Examples:
  # Clear pre-access-token-creation trigger
  zitadel-cli action undeploy \
    --flow complement-token \
    --trigger pre-access-token-creation`,
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx := context.Background()

		flowType, err := parseFlowType(deployFlowType)
		if err != nil {
			return err
		}

		triggerType, err := parseTriggerType(deployTriggerType, flowType)
		if err != nil {
			return err
		}

		err = apiClient.ClearActionTrigger(ctx, flowType, triggerType)
		if err != nil {
			return fmt.Errorf("failed to clear trigger: %w", err)
		}

		printer.Success("Cleared actions from %s/%s", deployFlowType, deployTriggerType)
		return nil
	},
}

func parseFlowType(s string) (client.FlowType, error) {
	switch strings.ToLower(s) {
	case "external-authentication", "external_authentication":
		return client.FlowTypeExternalAuthentication, nil
	case "complement-token", "complement_token":
		return client.FlowTypeComplimentToken, nil
	default:
		return "", fmt.Errorf("unknown flow type: %s (valid: external-authentication, complement-token)", s)
	}
}

func parseTriggerType(s string, flow client.FlowType) (client.TriggerType, error) {
	switch strings.ToLower(s) {
	case "pre-access-token-creation", "pre_access_token_creation":
		if flow != client.FlowTypeComplimentToken {
			return "", fmt.Errorf("trigger 'pre-access-token-creation' is only valid for complement-token flow")
		}
		return client.TriggerTypePreAccessTokenCreation, nil
	case "pre-userinfo-creation", "pre_userinfo_creation":
		if flow != client.FlowTypeComplimentToken {
			return "", fmt.Errorf("trigger 'pre-userinfo-creation' is only valid for complement-token flow")
		}
		return client.TriggerTypePreUserinfoCreation, nil
	case "post-authentication", "post_authentication":
		if flow != client.FlowTypeExternalAuthentication {
			return "", fmt.Errorf("trigger 'post-authentication' is only valid for external-authentication flow")
		}
		return client.TriggerTypePostAuthentication, nil
	case "pre-creation", "pre_creation":
		if flow != client.FlowTypeExternalAuthentication {
			return "", fmt.Errorf("trigger 'pre-creation' is only valid for external-authentication flow")
		}
		return client.TriggerTypePreCreation, nil
	case "post-creation", "post_creation":
		if flow != client.FlowTypeExternalAuthentication {
			return "", fmt.Errorf("trigger 'post-creation' is only valid for external-authentication flow")
		}
		return client.TriggerTypePostCreation, nil
	default:
		return "", fmt.Errorf("unknown trigger type: %s", s)
	}
}

func init() {
	// Create command flags
	actionCreateCmd.Flags().StringVar(&actionScript, "script-file", "", "Path to JavaScript file")
	actionCreateCmd.Flags().StringVar(&actionScriptArg, "script", "", "Inline JavaScript code")
	actionCreateCmd.Flags().StringVar(&actionTimeout, "timeout", "10s", "Execution timeout")
	actionCreateCmd.Flags().BoolVar(&actionAllowFail, "allow-fail", false, "Allow action to fail without blocking auth")

	// Update command flags
	actionUpdateCmd.Flags().StringVar(&actionScript, "script-file", "", "Path to JavaScript file")
	actionUpdateCmd.Flags().StringVar(&actionScriptArg, "script", "", "Inline JavaScript code")
	actionUpdateCmd.Flags().StringVar(&actionTimeout, "timeout", "", "Execution timeout")
	actionUpdateCmd.Flags().BoolVar(&actionAllowFail, "allow-fail", false, "Allow action to fail without blocking auth")

	// Deploy command flags
	actionDeployCmd.Flags().StringVar(&deployFlowType, "flow", "", "Flow type (external-authentication, complement-token)")
	actionDeployCmd.Flags().StringVar(&deployTriggerType, "trigger", "", "Trigger type (pre-access-token-creation, etc.)")
	_ = actionDeployCmd.MarkFlagRequired("flow")
	_ = actionDeployCmd.MarkFlagRequired("trigger")

	// Undeploy command flags
	actionUndeployCmd.Flags().StringVar(&deployFlowType, "flow", "", "Flow type")
	actionUndeployCmd.Flags().StringVar(&deployTriggerType, "trigger", "", "Trigger type")
	_ = actionUndeployCmd.MarkFlagRequired("flow")
	_ = actionUndeployCmd.MarkFlagRequired("trigger")

	actionCmd.AddCommand(actionListCmd)
	actionCmd.AddCommand(actionGetCmd)
	actionCmd.AddCommand(actionCreateCmd)
	actionCmd.AddCommand(actionUpdateCmd)
	actionCmd.AddCommand(actionDeleteCmd)
	actionCmd.AddCommand(actionDeployCmd)
	actionCmd.AddCommand(actionUndeployCmd)
}
