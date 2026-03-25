package cli

import (
	"strings"

	"github.com/breezewish/run9-cli/internal/api"
	"github.com/breezewish/run9-cli/internal/config"
	"github.com/spf13/cobra"
)

func (a *app) newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auth",
		Short:   "Manage the local API key login state",
		Long:    "Authenticate the CLI with one org-scoped API key and persist the verified login state to the local config file.",
		Example: "  run9 auth login --endpoint https://api.run9.example.com --ak ak-... --sk sk-...\n  run9 auth whoami\n  run9 auth logout",
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageError(cmd, "missing auth subcommand (expected: login|logout|whoami)")
		},
	}

	cmd.AddCommand(
		a.newAuthLoginCommand(),
		a.newAuthLogoutCommand(),
		a.newAuthWhoAmICommand(),
	)
	return cmd
}

func (a *app) newAuthLoginCommand() *cobra.Command {
	var endpoint string
	var ak string
	var sk string

	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Verify one API key and save it locally",
		Long:    "Call portal-api /whoami with the provided API key. The CLI only saves credentials after the remote identity check succeeds.",
		Example: "  run9 auth login --endpoint https://api.run9.example.com --ak ak-... --sk sk-...",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			existing, exists, err := config.LoadOptional(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}

			loginEndpoint := strings.TrimSpace(endpoint)
			if loginEndpoint == "" && exists {
				loginEndpoint = strings.TrimSpace(existing.Endpoint)
			}
			if loginEndpoint == "" {
				return usageError(cmd, "missing --endpoint")
			}
			if strings.TrimSpace(ak) == "" || strings.TrimSpace(sk) == "" {
				return usageError(cmd, "missing --ak or --sk")
			}

			client := api.NewClient(loginEndpoint)
			identity, err := client.WhoAmI(cmd.Context(), api.Credentials{
				AK: strings.TrimSpace(ak),
				SK: strings.TrimSpace(sk),
			})
			if err != nil {
				return commandErrorf("%v", err)
			}

			cfg := config.File{
				Endpoint: loginEndpoint,
				AK:       strings.TrimSpace(ak),
				SK:       strings.TrimSpace(sk),
				Identity: identityToConfig(identity),
			}
			if err := config.Save(a.configPath, cfg); err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, identity); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "portal-api endpoint")
	cmd.Flags().StringVar(&ak, "ak", "", "API key access key")
	cmd.Flags().StringVar(&sk, "sk", "", "API key secret key")
	cmd.Flags().SortFlags = false
	return cmd
}

func (a *app) newAuthLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Short:   "Delete the local CLI login state",
		Long:    "Delete the current CLI config file. If the file does not exist, logout still succeeds.",
		Example: "  run9 auth logout",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Remove(a.configPath); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}

func (a *app) newAuthWhoAmICommand() *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Short:   "Print the currently logged-in identity",
		Long:    "Refresh the remote /whoami view for the saved API key and print the current org-scoped identity as JSON.",
		Example: "  run9 auth whoami",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, client, creds, err := loadAuthenticatedClient(a.configPath)
			if err != nil {
				return commandErrorf("%v", err)
			}

			identity, err := client.WhoAmI(cmd.Context(), creds)
			if err != nil {
				return commandErrorf("%v", err)
			}

			cfg.Identity = identityToConfig(identity)
			if err := config.Save(a.configPath, cfg); err != nil {
				return commandErrorf("%v", err)
			}
			if err := writeJSON(a.stdout, identity); err != nil {
				return commandErrorf("%v", err)
			}
			return nil
		},
	}
}
