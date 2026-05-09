package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/secrets"
)

func newSecretsCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage Infisical-backed secret readiness.",
	}
	cmd.AddCommand(newSecretsLoginCommand(resolver, globals, factory))
	cmd.AddCommand(newSecretsStatusCommand(resolver, globals, factory))
	cmd.AddCommand(newSecretsCheckCommand(resolver, globals, factory))
	return cmd
}

func newSecretsLoginCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate the Infisical CLI for Loki render templates.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := factory(cmd.Context(), app.Options{Resolver: resolver, StoreOverride: globals.store, Verbose: globals.verbose, Stderr: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			defer svc.Close()
			if err := svc.SecretsLogin(cmd.Context(), app.SecretsLoginRequest{Domain: domain}); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Infisical login command completed.")
			fmt.Fprintln(cmd.OutOrStdout(), "Run `loki secrets status` to verify readiness.")
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "Infisical domain URL")
	return cmd
}

func newSecretsStatusCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check Infisical CLI installation and authentication.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := factory(cmd.Context(), app.Options{Resolver: resolver, StoreOverride: globals.store, Verbose: globals.verbose, Stderr: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.SecretsStatus(cmd.Context(), app.SecretsStatusRequest{})
			if err != nil {
				return err
			}
			if outputErr := printSecretsStatus(cmd.OutOrStdout(), result, jsonOutput); outputErr != nil {
				return outputErr
			}
			if !result.Ready {
				return fmt.Errorf("secrets status: Infisical is not ready")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newSecretsCheckCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "check <SECRET_NAME...>",
		Short: "Check required Infisical secrets without printing values.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := factory(cmd.Context(), app.Options{Resolver: resolver, StoreOverride: globals.store, Verbose: globals.verbose, Stderr: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.SecretsCheck(cmd.Context(), app.SecretsCheckRequest{Names: args})
			if err == nil || len(result.Checked) > 0 {
				if outputErr := printSecretsCheck(cmd.OutOrStdout(), result, jsonOutput); outputErr != nil {
					return outputErr
				}
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func printSecretsStatus(out io.Writer, result app.SecretsStatusResult, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	fmt.Fprintln(out, "Loki secrets")
	fmt.Fprintf(out, "Provider: %s\n", result.Provider)
	fmt.Fprintf(out, "CLI: %s\n", foundState(result.CLIInstalled))
	fmt.Fprintf(out, "Auth: %s\n", authState(result.Ready))
	if !result.Ready {
		if next := nextSecretStep(result.Checks); next != "" {
			fmt.Fprintf(out, "Next step: %s\n", next)
		}
	}
	return nil
}

func printSecretsCheck(out io.Writer, result app.SecretsCheckResult, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	fmt.Fprintln(out, "Loki secrets check")
	fmt.Fprintf(out, "Provider: %s\n", result.Provider)
	fmt.Fprintf(out, "Checked: %d\n", len(result.Checked))
	if len(result.Available) > 0 {
		fmt.Fprintf(out, "Available: %s\n", strings.Join(result.Available, ", "))
	}
	if len(result.Missing) > 0 {
		fmt.Fprintf(out, "Missing: %s\n", strings.Join(result.Missing, ", "))
	}
	return nil
}

func foundState(found bool) string {
	if found {
		return "found"
	}
	return "missing"
}

func authState(ready bool) string {
	if ready {
		return "authenticated"
	}
	return "not authenticated or project not initialized"
}

func nextSecretStep(checks []secrets.Check) string {
	for _, check := range checks {
		if strings.TrimSpace(check.Remediation) != "" {
			return strings.TrimSpace(check.Remediation)
		}
	}
	return "run `loki secrets login`, then run `infisical init` or configure an Infisical project if needed."
}
