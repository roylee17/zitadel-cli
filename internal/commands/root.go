// Package commands provides CLI command implementations.
package commands

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/roylee17/zitadel-cli/internal/client"
	"github.com/roylee17/zitadel-cli/internal/config"
	"github.com/roylee17/zitadel-cli/internal/output"
	"github.com/roylee17/zitadel-cli/internal/version"
)

var (
	// Global flags
	cfgFile     string
	contextName string
	zitadelURL  string
	zitadelPAT  string
	insecure    bool
	outputFmt   string
	verbose     int
	noConfirm   bool

	// Shared state
	cfg       *config.Config
	apiClient *client.Client
	printer   *output.Printer
)

var rootCmd = &cobra.Command{
	Use:   "zitadel-cli",
	Short: "CLI for Zitadel identity management",
	Long: `zitadel-cli is a comprehensive command-line tool for managing Zitadel resources.

Manage organizations, projects, applications, users, roles, and more from the command line.
Supports multiple Zitadel instances via contexts, flexible output formats, and automation-friendly features.

Configuration:
  The CLI reads configuration from ~/.zitadel/config.yaml by default.
  Use 'zitadel-cli context' commands to manage multiple Zitadel instances.

Authentication:
  Set your Personal Access Token (PAT) via:
  - Context configuration: zitadel-cli context set --token <PAT>
  - Environment variable: ZITADEL_PAT=<PAT>
  - Command line flag: --token <PAT>

Examples:
  # List projects
  zitadel-cli project list

  # Create an OIDC app
  zitadel-cli app create-oidc myapp --project myproject --type spa \
    --redirect-uri https://app.example.com/callback

  # Provision a complete project
  zitadel-cli provision myproject \
    --frontend-url https://frontend.example.com \
    --backend-url https://backend.example.com

Documentation:
  https://github.com/roylee17/zitadel-cli`,
	Version: version.Info(),
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		// Skip initialization for completion, help, and version commands
		if cmd.Name() == "completion" || cmd.Name() == "help" || cmd.Name() == "__complete" || cmd.Name() == "version" {
			return nil
		}

		// Load configuration
		var err error
		if cfgFile != "" {
			cfg, err = config.LoadFrom(cfgFile)
		} else {
			cfg, err = config.Load()
		}
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Use specified context or current
		if contextName != "" {
			cfg.CurrentContext = contextName
		}

		// Get context (may be nil for context commands)
		configCtx := cfg.CurrentCtx()

		// Skip client initialization for context management commands
		if isContextCommand(cmd) {
			return nil
		}

		// Build client configuration
		clientCfg := client.Config{
			Insecure: insecure,
		}

		// Priority: flags > env > context config
		if zitadelURL != "" {
			clientCfg.URL = zitadelURL
		} else if url := os.Getenv("ZITADEL_URL"); url != "" {
			clientCfg.URL = url
		} else if configCtx != nil && configCtx.URL != "" {
			clientCfg.URL = configCtx.URL
		}

		if zitadelPAT != "" {
			clientCfg.Token = zitadelPAT
		} else if token := os.Getenv("ZITADEL_PAT"); token != "" {
			clientCfg.Token = token
		} else if configCtx != nil {
			token, _ := configCtx.GetToken()
			clientCfg.Token = token
		}

		if configCtx != nil {
			clientCfg.Insecure = configCtx.Insecure || insecure
		}

		// Validate required config
		if clientCfg.URL == "" {
			return fmt.Errorf("zitadel URL required (--url, ZITADEL_URL, or configure context)")
		}
		if clientCfg.Token == "" {
			return fmt.Errorf("zitadel PAT required (--token, ZITADEL_PAT, or configure context)")
		}

		// Initialize client
		apiClient, err = client.New(clientCfg)
		if err != nil {
			return fmt.Errorf("create client: %w", err)
		}

		// Initialize printer
		format, err := output.ParseFormat(outputFmt)
		if err != nil {
			return err
		}

		templateStr := ""
		if strings.HasPrefix(outputFmt, "go-template=") {
			templateStr = strings.TrimPrefix(outputFmt, "go-template=")
		}
		printer = output.NewPrinter(format, templateStr)

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func isContextCommand(cmd *cobra.Command) bool {
	// Check if this is a context management command
	for p := cmd; p != nil; p = p.Parent() {
		if p.Name() == "context" || p.Name() == "config" {
			return true
		}
	}
	return false
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.zitadel/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&contextName, "context", "c", "", "context to use")
	rootCmd.PersistentFlags().StringVar(&zitadelURL, "url", "", "Zitadel instance URL")
	rootCmd.PersistentFlags().StringVar(&zitadelPAT, "token", "", "Personal Access Token")
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "skip TLS verification")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "output format (table|wide|json|yaml|name|go-template=...)")
	rootCmd.PersistentFlags().CountVarP(&verbose, "verbose", "v", "verbose output (use -v, -vv, -vvv for more)")
	rootCmd.PersistentFlags().BoolVarP(&noConfirm, "yes", "y", false, "skip confirmation prompts")

	// Bind to viper for env var support
	_ = viper.BindPFlag("url", rootCmd.PersistentFlags().Lookup("url"))
	_ = viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token"))

	// Add subcommands
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(orgCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(appCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(machineCmd)
	rootCmd.AddCommand(patCmd)
	rootCmd.AddCommand(roleCmd)
	rootCmd.AddCommand(provisionCmd)
	rootCmd.AddCommand(healthzCmd)
	rootCmd.AddCommand(whoamiCmd)
	rootCmd.AddCommand(versionCmd)

	// Custom version template
	rootCmd.SetVersionTemplate(version.Full() + "\n")
}

func initConfig() {
	viper.SetEnvPrefix("ZITADEL")
	viper.AutomaticEnv()
}

// Execute runs the CLI.
func Execute() error {
	return rootCmd.Execute()
}

// Helper functions available to all commands

func confirmAction(message string) bool {
	if noConfirm {
		return true
	}
	confirmed, err := output.Confirm(message, false)
	if err != nil {
		return false
	}
	return confirmed
}

func confirmDanger(message string) bool {
	if noConfirm {
		// Dangerous operations require explicit --yes
		printer.Error("This operation requires confirmation. Use --yes to skip prompts.")
		return false
	}
	confirmed, err := output.ConfirmDanger(message)
	if err != nil {
		return false
	}
	return confirmed
}

func debugf(format string, args ...interface{}) {
	if verbose >= 1 {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

// commandContext returns a context with a 30-second timeout for API calls.
func commandContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// isNilValue checks if an interface value is nil or contains a nil pointer.
// This is needed because in Go, an interface can be non-nil but contain a nil pointer.
func isNilValue(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// namedResource represents a resource with an ID and Name.
type namedResource interface {
	GetID() string
	GetName() string
}

// createResourceParams contains parameters for creating a resource.
type createResourceParams struct {
	resourceType string
	name         string
	getExisting  func(ctx context.Context, name string) (namedResource, error)
	create       func(ctx context.Context, name string) (namedResource, error)
}

// createOrGetResource implements the create-or-get pattern for resources.
func createOrGetResource(params createResourceParams) error {
	ctx, cancel := commandContext()
	defer cancel()

	spin := output.NewSpinner(fmt.Sprintf("Checking existing %s...", params.resourceType))
	spin.Start()

	existing, err := params.getExisting(ctx, params.name)
	if err != nil {
		spin.Stop()
		return fmt.Errorf("failed to check existing %s: %w", params.resourceType, err)
	}
	// Check if the interface is nil AND if the underlying value is nil
	// In Go, an interface can be non-nil but contain a nil pointer
	if existing != nil && !isNilValue(existing) {
		spin.Stop()
		printer.Warning("%s '%s' already exists (ID: %s)", params.resourceType, params.name, existing.GetID())
		return printer.PrintObject(existing)
	}

	spin.UpdateMessage(fmt.Sprintf("Creating %s...", params.resourceType))
	resource, err := params.create(ctx, params.name)
	spin.Stop()
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", params.resourceType, err)
	}

	printer.Success("%s '%s' created", params.resourceType, resource.GetName())
	return printer.PrintObject(resource)
}
