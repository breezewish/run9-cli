package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/breezewish/run9-cli/internal/buildinfo"
	"github.com/breezewish/run9-cli/internal/config"
	"github.com/spf13/cobra"
)

type app struct {
	configPath string
	stdout     io.Writer
	stderr     io.Writer
}

type cliError struct {
	Code    int
	Message string
	Usage   string
}

func (e *cliError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Main runs the run9 CLI.
func Main(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	cobra.EnableCommandSorting = false

	cliApp := &app{
		configPath: config.DefaultPath(),
		stdout:     stdout,
		stderr:     stderr,
	}

	rootCmd := cliApp.newRootCommand()
	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetContext(ctx)

	if err := rootCmd.Execute(); err != nil {
		return cliApp.renderError(err)
	}
	return 0
}

func (a *app) newRootCommand() *cobra.Command {
	version := buildinfo.DisplayVersion()

	cmd := &cobra.Command{
		Use:              "run9",
		Short:            "Manage run9 boxes and snaps",
		Long:             "run9 is the CLI-first entrypoint for run9. It authenticates with an org-scoped API key and sends all user-visible resource operations through portal-api.",
		Example:          "  run9 auth login --endpoint https://api.run9.example.com --ak ak-... --sk sk-...\n  run9 box create my-box --image public.ecr.aws/docker/library/alpine:3.20\n  run9 box exec my-box /bin/sh -lc 'echo hello'\n  run9 --version\n  run9 completion zsh > \"${fpath[1]}/_run9\"",
		SilenceErrors:    true,
		SilenceUsage:     true,
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError(cmd, "missing command (expected: auth|box|snap|completion|version)")
		},
		Version: version,
	}
	cmd.SetVersionTemplate("{{printf \"%s version %s\\n\" .Name .Version}}")
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().StringVar(&a.configPath, "config", config.DefaultPath(), "path to CLI config")
	cmd.PersistentFlags().SortFlags = false

	cmd.AddCommand(
		a.newAuthCommand(),
		a.newBoxCommand(),
		a.newSnapCommand(),
		a.newCompletionCommand(),
		a.newVersionCommand(version),
	)
	return cmd
}

func (a *app) newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the run9 CLI version",
		Long:    "Print the version embedded into the current run9 binary.",
		Example: "  run9 version\n  run9 --version",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "run9 version %s\n", version)
			return err
		},
	}
}

func (a *app) newCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:       "completion <bash|zsh|fish|powershell>",
		Short:     "Generate shell completion scripts",
		Long:      "Generate a shell completion script for run9 and write it to standard output.",
		Example:   "  run9 completion bash > /etc/bash_completion.d/run9\n  run9 completion zsh > \"${fpath[1]}/_run9\"\n  run9 completion fish > ~/.config/fish/completions/run9.fish\n  run9 completion powershell > run9.ps1",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError(cmd, "usage: %s <bash|zsh|fish|powershell>", cmd.CommandPath())
			}
			switch args[0] {
			case "bash", "zsh", "fish", "powershell":
				return nil
			default:
				return usageError(cmd, "unsupported shell %q (expected: bash|zsh|fish|powershell)", args[0])
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			default:
				return usageError(cmd, "unsupported shell %q (expected: bash|zsh|fish|powershell)", args[0])
			}
		},
	}
}

func (a *app) renderError(err error) int {
	var cliErr *cliError
	if errors.As(err, &cliErr) {
		if cliErr.Message != "" {
			fmt.Fprintln(a.stderr, cliErr.Message)
		}
		if cliErr.Usage != "" {
			fmt.Fprint(a.stderr, cliErr.Usage)
		}
		if cliErr.Code == 0 {
			return 1
		}
		return cliErr.Code
	}

	fmt.Fprintln(a.stderr, err)
	return 1
}

func commandErrorf(format string, args ...any) error {
	return &cliError{
		Code:    1,
		Message: fmt.Sprintf(format, args...),
	}
}

func usageError(cmd *cobra.Command, format string, args ...any) error {
	return &cliError{
		Code:    1,
		Message: fmt.Sprintf(format, args...),
		Usage:   cmd.UsageString(),
	}
}

func exitCodeError(code int) error {
	if code == 0 {
		return nil
	}
	return &cliError{Code: code}
}

func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return usageError(cmd, "unexpected args: %v", args)
	}
	return nil
}
