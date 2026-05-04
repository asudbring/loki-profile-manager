package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
)

func newSnapshotsCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshots",
		Short: "Inspect local activation snapshots.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSnapshotsListCommand(resolver, globals, factory))
	cmd.AddCommand(newSnapshotsShowCommand(resolver, globals, factory))
	cmd.AddCommand(newSnapshotsRestoreCommand(resolver, globals, factory))
	return cmd
}

func newSnapshotsListCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List retained local activation snapshots.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			svc, err := factory(ctx, app.Options{
				Resolver:      resolver,
				StoreOverride: globals.store,
				Verbose:       globals.verbose,
				Stderr:        cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			defer svc.Close()

			result, err := svc.ListSnapshots(ctx, app.SnapshotListRequest{})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			printSnapshotList(cmd, result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newSnapshotsShowCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <snapshot-id>",
		Short: "Show local activation snapshot metadata.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			svc, err := factory(ctx, app.Options{
				Resolver:      resolver,
				StoreOverride: globals.store,
				Verbose:       globals.verbose,
				Stderr:        cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			defer svc.Close()

			result, err := svc.ShowSnapshot(ctx, app.SnapshotShowRequest{SnapshotID: args[0]})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			printSnapshotShow(cmd, result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newSnapshotsRestoreCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var dryRun bool
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "restore <snapshot-id>",
		Short: "Preview a local activation snapshot restore without writing files.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			svc, err := factory(ctx, app.Options{
				Resolver:      resolver,
				StoreOverride: globals.store,
				Verbose:       globals.verbose,
				Stderr:        cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			defer svc.Close()

			result, err := svc.RestoreSnapshotDryRun(ctx, app.SnapshotRestoreDryRunRequest{SnapshotID: args[0], DryRun: dryRun})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}
			printSnapshotRestoreDryRun(cmd, result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview restore actions without writing files; required")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func printSnapshotList(cmd *cobra.Command, result app.SnapshotListResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Loki snapshots")
	fmt.Fprintf(out, "Local snapshot dir: %s\n", result.SnapshotDir)
	fmt.Fprintf(out, "Snapshots: %d\n", len(result.Snapshots))
	for _, snapshot := range result.Snapshots {
		previousProfile := snapshot.PreviousActiveProfile
		if previousProfile == "" {
			previousProfile = "not set"
		}
		fmt.Fprintf(out, "- %s", snapshot.SnapshotID)
		if snapshot.CreatedAt != "" {
			fmt.Fprintf(out, " created=%s", snapshot.CreatedAt)
		}
		fmt.Fprintf(out, " previous=%s buckets=%s targets=%d", previousProfile, formatSnapshotList(snapshot.PreviousActiveBuckets), snapshot.TargetCount)
		if len(snapshot.TargetKinds) > 0 {
			fmt.Fprintf(out, " kinds=%s", strings.Join(snapshot.TargetKinds, ","))
		}
		if !snapshot.Exists {
			fmt.Fprint(out, " exists=false")
		}
		if snapshot.Path != "" {
			fmt.Fprintf(out, " path=%s", snapshot.Path)
		}
		fmt.Fprintln(out)
		if snapshot.MetadataError != "" {
			fmt.Fprintf(out, "  metadata warning: %s\n", snapshot.MetadataError)
		}
	}
}

func printSnapshotShow(cmd *cobra.Command, result app.SnapshotShowResult) {
	out := cmd.OutOrStdout()
	snapshot := result.Snapshot
	fmt.Fprintln(out, "Loki snapshot")
	fmt.Fprintf(out, "ID: %s\n", snapshot.SnapshotID)
	fmt.Fprintf(out, "Local snapshot dir: %s\n", result.SnapshotDir)
	fmt.Fprintf(out, "Path: %s\n", snapshot.Path)
	if snapshot.CreatedAt != "" {
		fmt.Fprintf(out, "Created: %s\n", snapshot.CreatedAt)
	}
	previousProfile := snapshot.PreviousActiveProfile
	if previousProfile == "" {
		previousProfile = "not set"
	}
	fmt.Fprintf(out, "Previous active profile: %s\n", previousProfile)
	fmt.Fprintf(out, "Previous active buckets: %s\n", formatSnapshotList(snapshot.PreviousActiveBuckets))
	fmt.Fprintf(out, "Targets: %d\n", len(snapshot.Targets))
	for _, target := range snapshot.Targets {
		printSnapshotTarget(out, snapshot, target)
	}
}

func printSnapshotTarget(out interface{ Write([]byte) (int, error) }, snapshot activation.Snapshot, target activation.SnapshotEntry) {
	kind := target.Kind
	if kind == "" {
		kind = "unknown"
	}
	fmt.Fprintf(out, "- %s %s", kind, target.TargetPath)
	if value := shortSnapshotHash(target.Hash); value != "" {
		fmt.Fprintf(out, " hash=%s", value)
	}
	if value := shortSnapshotHash(target.ExpectedHash); value != "" {
		fmt.Fprintf(out, " expected_hash=%s", value)
	}
	if target.ExpectedMode != "" {
		fmt.Fprintf(out, " expected_mode=%s", target.ExpectedMode)
	}
	if target.SnapshotPath != "" {
		fmt.Fprintf(out, " snapshot_entry=%s", formatSnapshotEntryPath(snapshot.Path, target.SnapshotPath))
	}
	if target.LinkTarget != "" {
		fmt.Fprintf(out, " link_target=%s", target.LinkTarget)
	}
	fmt.Fprintln(out)
	if snapshotPathLooksSensitive(target.TargetPath) {
		fmt.Fprintln(out, "  warning: sensitive-looking target path; inspect carefully before manual recovery")
	}
}

func printSnapshotRestoreDryRun(cmd *cobra.Command, result app.SnapshotRestoreDryRunResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Loki snapshot restore dry-run")
	fmt.Fprintf(out, "ID: %s\n", result.SnapshotID)
	fmt.Fprintln(out, "Mode: dry-run only; no files or local state were changed")
	previousProfile := result.Summary.PreviousActiveProfile
	if previousProfile == "" {
		previousProfile = "not set"
	}
	fmt.Fprintf(out, "Would restore active profile: %s\n", previousProfile)
	fmt.Fprintf(out, "Would restore active buckets: %s\n", formatSnapshotList(result.Summary.PreviousActiveBuckets))
	fmt.Fprintf(out, "Would restore managed target rows: %d\n", result.Summary.WouldRestoreManagedTargetRows)
	fmt.Fprintf(out, "Targets: %d\n", result.Summary.TargetCount)
	for _, target := range result.Targets {
		printSnapshotRestoreTarget(out, target)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "Warning: %s\n", warning)
	}
}

func printSnapshotRestoreTarget(out interface{ Write([]byte) (int, error) }, target app.SnapshotRestoreDryRunTarget) {
	path := target.TargetPath
	if target.TargetPathRedacted || path == "" {
		path = "(redacted-sensitive-path)"
	}
	fmt.Fprintf(out, "- %s %s", target.Action, path)
	if target.CurrentKind != "" {
		fmt.Fprintf(out, " current=%s", target.CurrentKind)
	}
	if target.CurrentHashPrefix != "" {
		fmt.Fprintf(out, " hash=%s", target.CurrentHashPrefix)
	}
	if target.SnapshotHashPrefix != "" {
		fmt.Fprintf(out, " snapshot_hash=%s", target.SnapshotHashPrefix)
	}
	if target.ExpectedHashPrefix != "" {
		fmt.Fprintf(out, " expected_hash=%s", target.ExpectedHashPrefix)
	}
	if target.ExpectedMode != "" {
		fmt.Fprintf(out, " expected_mode=%s", target.ExpectedMode)
	}
	if target.LinkTarget != "" {
		fmt.Fprintf(out, " link_target=%s", target.LinkTarget)
	} else if target.LinkTargetRedacted {
		fmt.Fprint(out, " link_target=(redacted)")
	}
	fmt.Fprintln(out)
	if target.TargetPathRedacted {
		fmt.Fprintln(out, "  warning: sensitive-looking target path redacted")
	}
	for _, warning := range target.Warnings {
		fmt.Fprintf(out, "  warning: %s\n", warning)
	}
}

func formatSnapshotList(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func shortSnapshotHash(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func formatSnapshotEntryPath(snapshotPath, entryPath string) string {
	if snapshotPath == "" || entryPath == "" {
		return entryPath
	}
	if rel, err := filepath.Rel(snapshotPath, entryPath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return entryPath
}

func snapshotPathLooksSensitive(path string) bool {
	return activation.PathLooksSensitive(path)
}
