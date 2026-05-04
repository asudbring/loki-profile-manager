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
	lower := strings.ToLower(filepath.ToSlash(path))
	for _, term := range []string{"/.ssh", ".env", "token", "credential", "private", "key"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
