package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/roylee17/zitadel-cli/internal/client"
	"github.com/roylee17/zitadel-cli/internal/output"
)

var projectCmd = &cobra.Command{
	Use:     "project",
	Aliases: []string{"proj"},
	Short:   "Manage projects",
	Long:    "Commands for managing Zitadel projects.",
}

var projectListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all projects",
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx, cancel := commandContext()
		defer cancel()

		spin := output.NewSpinner("Fetching projects...")
		spin.Start()

		projects, err := apiClient.ListProjects(ctx)
		spin.Stop()
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		if len(projects) == 0 {
			printer.Info("No projects found")
			return nil
		}

		return output.PrintTable(printer, []string{"ID", "NAME", "STATE"}, projects, func(p client.Project) []string {
			return []string{p.ID, p.Name, p.State}
		})
	},
}

var projectGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get project by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		ctx, cancel := commandContext()
		defer cancel()

		spin := output.NewSpinner("Fetching project...")
		spin.Start()

		project, err := apiClient.GetProjectByName(ctx, args[0])
		spin.Stop()
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}
		if project == nil {
			return fmt.Errorf("project '%s' not found", args[0])
		}

		return printer.PrintObject(project)
	},
}

var projectCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return createOrGetResource(createResourceParams{
			resourceType: "Project",
			name:         args[0],
			getExisting: func(ctx context.Context, name string) (namedResource, error) {
				return apiClient.GetProjectByName(ctx, name)
			},
			create: func(ctx context.Context, name string) (namedResource, error) {
				return apiClient.CreateProject(ctx, name)
			},
		})
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <project-id>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if !confirmDanger(fmt.Sprintf("Delete project '%s'?", args[0])) {
			printer.Info("Cancelled")
			return nil
		}

		ctx, cancel := commandContext()
		defer cancel()

		spin := output.NewSpinner("Deleting project...")
		spin.Start()

		err := apiClient.DeleteProject(ctx, args[0])
		spin.Stop()
		if err != nil {
			return fmt.Errorf("failed to delete project: %w", err)
		}

		printer.Success("Project '%s' deleted", args[0])
		return nil
	},
}

func init() {
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectGetCmd)
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectDeleteCmd)
	projectCmd.AddCommand(projectGrantCmd)
}
