package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
)

func newStoreCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "store",
		Short: "Manage persistent Loki store configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newStoreStatusCommand(resolver, globals, factory))
	cmd.AddCommand(newStoreDiscoverCommand(resolver, globals, factory))
	cmd.AddCommand(newStoreUseCommand(resolver, globals, factory))
	cmd.AddCommand(newStoreInitCommand(resolver, globals, factory))
	cmd.AddCommand(newStoreUnsetCommand(resolver, globals, factory))
	return cmd
}

func newStoreStatusCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show persistent Loki store configuration.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCLIService(cmd, resolver, globals, factory)
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.StoreStatus(cmd.Context(), app.StoreStatusRequest{})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			printStoreStatus(cmd, result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newStoreDiscoverCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var manualPath string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover likely OneDrive, Dropbox, or manual Loki store paths.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCLIService(cmd, resolver, globals, factory)
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.DiscoverStores(cmd.Context(), app.DiscoverStoresRequest{ManualPath: manualPath})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			printStoreDiscover(cmd, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&manualPath, "manual", "", "manual store path candidate")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newStoreUseCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "use <path>",
		Short: "Persist an existing valid Loki store path.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCLIService(cmd, resolver, globals, factory)
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.UseStore(cmd.Context(), app.UseStoreRequest{StorePath: args[0]})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			printStoreEnsureResult(cmd, "Store configured", result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newStoreInitCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "init <path>",
		Short: "Create or validate a Loki store layout and persist it.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCLIService(cmd, resolver, globals, factory)
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.EnsureStore(cmd.Context(), app.EnsureStoreRequest{StorePath: args[0]})
			if err != nil {
				return err
			}
			if !result.Valid {
				return fmt.Errorf("store init: invalid store layout: missing %v", result.Missing)
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			printStoreEnsureResult(cmd, "Store initialized", result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newStoreUnsetCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "unset",
		Short: "Clear persisted local Loki store configuration.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newCLIService(cmd, resolver, globals, factory)
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.ForgetStore(cmd.Context(), app.ForgetStoreRequest{})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Store configuration cleared")
			if result.StoreOverride != "" {
				fmt.Fprintf(out, "Note: --store override is still active for this command: %s\n", result.StoreOverride)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newCLIService(cmd *cobra.Command, resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) (*app.Service, error) {
	return factory(cmd.Context(), app.Options{
		Resolver:      resolver,
		StoreOverride: globals.store,
		Verbose:       globals.verbose,
		Stderr:        cmd.ErrOrStderr(),
	})
}

func printStoreStatus(cmd *cobra.Command, result app.StoreStatusResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Loki store")
	fmt.Fprintf(out, "Local state: %s\n", result.LocalStatePath)
	fmt.Fprintf(out, "Database: %s\n", result.DatabasePath)
	fmt.Fprintf(out, "Persisted store: %s\n", firstStoreValue(result.PersistedStorePath))
	fmt.Fprintf(out, "Store override: %s\n", firstStoreValue(result.StoreOverride))
	fmt.Fprintf(out, "Effective store: %s\n", firstStoreValue(result.EffectiveStorePath))
	fmt.Fprintf(out, "Effective source: %s\n", result.EffectiveSource)
	fmt.Fprintf(out, "Valid: %s\n", formatBoolCLI(result.Valid))
	if len(result.Missing) > 0 {
		fmt.Fprintf(out, "Missing: %d paths\n", len(result.Missing))
	}
	fmt.Fprintf(out, "Message: %s\n", result.Message)
}

func printStoreDiscover(cmd *cobra.Command, result app.DiscoverStoresResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Loki store candidates")
	if len(result.Candidates) == 0 {
		fmt.Fprintln(out, "No OneDrive, Dropbox, or manual candidates found.")
		return
	}
	for i, candidate := range result.Candidates {
		fmt.Fprintf(out, "%d. %s %s\n", i+1, candidate.Provider, candidate.StorePath)
		fmt.Fprintf(out, "   Provider root: %s\n", candidate.ProviderPath)
		fmt.Fprintf(out, "   Source: %s\n", candidate.Source)
		fmt.Fprintf(out, "   Provider exists: %s\n", formatBoolCLI(candidate.ProviderExists))
		fmt.Fprintf(out, "   Store exists: %s\n", formatBoolCLI(candidate.StoreExists))
		fmt.Fprintf(out, "   Store valid: %s\n", formatBoolCLI(candidate.StoreValid))
		if len(candidate.Missing) > 0 {
			fmt.Fprintf(out, "   Missing: %d paths\n", len(candidate.Missing))
		}
	}
}

func printStoreEnsureResult(cmd *cobra.Command, title string, result app.EnsureStoreResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, title)
	fmt.Fprintf(out, "Store: %s\n", result.StorePath)
	fmt.Fprintf(out, "Created: %s\n", formatBoolCLI(result.Created))
	fmt.Fprintf(out, "Valid: %s\n", formatBoolCLI(result.Valid))
	if len(result.Missing) > 0 {
		fmt.Fprintf(out, "Missing: %s\n", strings.Join(result.Missing, ", "))
	}
	if result.Valid {
		fmt.Fprintln(out, "Next step: loki machine register --allow-profile <profile>")
	}
}

func firstStoreValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not configured"
	}
	return value
}

func formatBoolCLI(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
