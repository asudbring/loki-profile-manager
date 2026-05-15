package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/store"
	"github.com/asudbring/loki-profile-manager/internal/storemigrate"
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
	cmd.AddCommand(newStoreMigrateCommand(resolver, globals, factory))
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

func newStoreMigrateCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var fromPath string
	var toPath string
	var providerValue string
	var dryRun bool
	var yes bool
	var copyOnly bool
	var captureLocal bool
	var hydrate bool
	var cleanup bool
	var fileTimeout time.Duration
	var progressInterval time.Duration
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "migrate --to <path> (--dry-run|--yes)",
		Short: "Copy the current Loki store to a new cloud-provider path and optionally rewire local state.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, err := parseStoreMigrateProvider(providerValue)
			if err != nil {
				return err
			}
			svc, err := newCLIService(cmd, resolver, globals, factory)
			if err != nil {
				return err
			}
			defer svc.Close()
			reporter := storemigrate.Reporter(nil)
			if !jsonOutput && yes {
				reporter = newStoreMigrateReporter(cmd)
			}
			result, err := svc.StoreMigrate(cmd.Context(), app.StoreMigrateRequest{FromPath: fromPath, ToPath: toPath, Provider: provider, DryRun: dryRun, Yes: yes, CopyOnly: copyOnly, CaptureLocal: captureLocal, Hydrate: hydrate, Cleanup: cleanup, FileTimeout: fileTimeout, ProgressInterval: progressInterval, Reporter: reporter})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			printStoreMigrate(cmd, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "source Loki store path (default: current effective store)")
	cmd.Flags().StringVar(&toPath, "to", "", "destination Loki store path; must be missing or empty")
	cmd.Flags().StringVar(&providerValue, "provider", "", "destination provider label: onedrive-business, onedrive-personal, onedrive, dropbox, or manual")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan migration without copying or rewiring")
	cmd.Flags().BoolVar(&yes, "yes", false, "copy and rewire local state after validation")
	cmd.Flags().BoolVar(&copyOnly, "copy-only", false, "copy and validate destination without changing local store configuration")
	cmd.Flags().BoolVar(&captureLocal, "capture-local", false, "write safe local copy-mode changes back to the source store before copying")
	cmd.Flags().BoolVar(&hydrate, "hydrate", false, "explicitly materialize cloud-only source files before copying")
	cmd.Flags().BoolVar(&cleanup, "cleanup", false, "remove interrupted staging directories for the destination and exit")
	cmd.Flags().DurationVar(&fileTimeout, "file-timeout", 2*time.Minute, "maximum time to spend on one source file before failing")
	cmd.Flags().DurationVar(&progressInterval, "progress-interval", 2*time.Second, "minimum interval between same-phase progress messages")
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

func printStoreMigrate(cmd *cobra.Command, result app.StoreMigrateResult) {
	out := cmd.OutOrStdout()
	if result.CleanedStaging != nil {
		fmt.Fprintf(out, "Interrupted staging directories removed: %d\n", len(result.CleanedStaging))
		for _, path := range result.CleanedStaging {
			fmt.Fprintf(out, "Removed: %s\n", path)
		}
		return
	}
	if result.DryRun {
		fmt.Fprintln(out, "Store migration dry-run")
	} else if result.CopyOnly {
		fmt.Fprintln(out, "Store copied")
	} else {
		fmt.Fprintln(out, "Store migrated")
	}
	fmt.Fprintf(out, "Source: %s\n", result.OldStorePath)
	fmt.Fprintf(out, "Destination: %s\n", result.NewStorePath)
	if result.Provider != "" {
		fmt.Fprintf(out, "Provider: %s\n", result.Provider)
	}
	fmt.Fprintf(out, "Files: %d\n", result.Plan.Summary.FileCount)
	fmt.Fprintf(out, "Directories: %d\n", result.Plan.Summary.DirCount)
	fmt.Fprintf(out, "Symlinks: %d\n", result.Plan.Summary.SymlinkCount)
	fmt.Fprintf(out, "Cloud-only files: %d\n", result.Plan.Summary.DatalessCount)
	fmt.Fprintf(out, "Bytes: %d\n", result.Plan.Summary.ByteCount)
	if !result.DryRun {
		if result.HydratedFiles > 0 {
			fmt.Fprintf(out, "Hydrated files: %d\n", result.HydratedFiles)
		}
		fmt.Fprintf(out, "Copied files: %d\n", result.CopiedFiles)
		fmt.Fprintf(out, "Copied directories: %d\n", result.CopiedDirs)
		fmt.Fprintf(out, "Copied symlinks: %d\n", result.CopiedSymlinks)
		fmt.Fprintf(out, "Rebased managed targets: %d\n", result.RebasedManagedTargets)
		fmt.Fprintf(out, "Retargeted symlinks: %d\n", result.RetargetedSymlinks)
		fmt.Fprintf(out, "Local store switched: %s\n", formatBoolCLI(result.Switched))
	} else if result.Plan.Summary.DatalessCount > 0 {
		fmt.Fprintln(out, "Next step: make cloud-only files available locally or rerun with --yes --hydrate to materialize and copy.")
	} else {
		fmt.Fprintln(out, "Next step: rerun with --yes to copy and rewire, or --yes --copy-only to stage the copy only.")
	}
	if result.CaptureRequired {
		fmt.Fprintln(out, "Local changes detected: rerun with --capture-local before migrating.")
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "Warning: %s\n", warning)
	}
}

func newStoreMigrateReporter(cmd *cobra.Command) storemigrate.Reporter {
	return storemigrate.ReporterFunc(func(_ context.Context, event storemigrate.Event) {
		out := cmd.ErrOrStderr()
		message := event.Message
		if message == "" {
			message = string(event.Phase)
		}
		if event.TotalFiles > 0 {
			fmt.Fprintf(out, "[%s] %s (%d/%d files, %d/%d bytes)", event.Phase, message, event.DoneFiles, event.TotalFiles, event.DoneBytes, event.TotalBytes)
		} else {
			fmt.Fprintf(out, "[%s] %s", event.Phase, message)
		}
		if event.CurrentPath != "" {
			fmt.Fprintf(out, ": %s", event.CurrentPath)
		}
		fmt.Fprintln(out)
	})
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

func parseStoreMigrateProvider(value string) (store.ProviderType, error) {
	value = strings.TrimSpace(value)
	switch store.ProviderType(value) {
	case "":
		return "", nil
	case store.ProviderOneDrive, store.ProviderOneDriveBusiness, store.ProviderOneDrivePersonal, store.ProviderDropbox, store.ProviderManual:
		return store.ProviderType(value), nil
	default:
		return "", fmt.Errorf("store migrate: unsupported provider %q", value)
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
