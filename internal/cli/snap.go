package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) newSnapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "snap",
		Short:   "Manage snaps",
		Long:    "Import, inspect, list, and remove snaps through portal-api.",
		Example: "  run9 snap import public.ecr.aws/docker/library/alpine:3.20\n  run9 snap ls --attached true\n  run9 snap inspect snap-1",
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError(cmd, "missing snap subcommand (expected: import|ls|inspect|rm)")
		},
	}

	cmd.AddCommand(
		a.newSnapImportCommand(),
		a.newSnapListCommand(),
		a.newSnapInspectCommand(),
		a.newSnapRemoveCommand(),
	)
	return cmd
}

func (a *app) newSnapImportCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "import <image-ref>",
		Short:   "Import one image into a snap",
		Long:    "Import one OCI image reference and print the resulting snap view as JSON.",
		Example: "  run9 snap import public.ecr.aws/docker/library/alpine:3.20",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "usage: %s <image-ref>", cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			view, err := client.ImportSnap(cmd.Context(), creds, args[0])
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, view); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}

func (a *app) newSnapListCommand() *cobra.Command {
	var attached string

	cmd := &cobra.Command{
		Use:     "ls",
		Short:   "List snaps",
		Long:    "List snaps in the current org. The optional --attached filter accepts true or false.",
		Example: "  run9 snap ls\n  run9 snap ls --attached true",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(attached) != "" && attached != "true" && attached != "false" {
				return usageError(cmd, "--attached must be true or false")
			}

			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			views, err := client.Snaps(cmd.Context(), creds, attached)
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, views); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&attached, "attached", "", "attached filter (true|false)")
	cmd.Flags().SortFlags = false
	return cmd
}

func (a *app) newSnapInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "inspect <snap-id>",
		Short:   "Inspect one snap",
		Long:    "Fetch one snap view from portal-api and print it as indented JSON.",
		Example: "  run9 snap inspect snap-1",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "usage: %s <snap-id>", cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			view, err := client.Snap(cmd.Context(), creds, args[0])
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, view); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}

func (a *app) newSnapRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "rm <snap-id>",
		Short:   "Remove one snap",
		Long:    "Delete one snap object through portal-api and print the removed snap view as JSON.",
		Example: "  run9 snap rm snap-1",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "usage: %s <snap-id>", cmd.CommandPath())
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}
			view, err := client.RemoveSnap(cmd.Context(), creds, args[0])
			if err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, view); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}
