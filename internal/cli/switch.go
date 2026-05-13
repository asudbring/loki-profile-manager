package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
)

func newSwitchCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var dryRun bool
	var yes bool
	var captureLocal bool
	var backupUnmanaged bool
	cmd := &cobra.Command{
		Use:   "switch <profile> [buckets...]",
		Short: "Activate a Loki profile and optional buckets.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := factory(cmd.Context(), app.Options{
				Resolver:      resolver,
				StoreOverride: globals.store,
				Verbose:       globals.verbose,
				Stderr:        cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.Switch(cmd.Context(), app.SwitchRequest{ParentProfile: args[0], Buckets: args[1:], DryRun: dryRun, Yes: yes, CaptureLocal: captureLocal, BackupUnmanaged: backupUnmanaged})
			printSwitchResult(cmd, result, globals.verbose, err)
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and show activation plan without writing files")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm safe prompts; does not bypass unmanaged overwrite protection")
	cmd.Flags().BoolVar(&captureLocal, "capture-local", false, "write safe local managed-target changes back to the store before switching")
	cmd.Flags().BoolVar(&backupUnmanaged, "backup-unmanaged", false, "move unmanaged blocker targets to a local backup before switching; requires --yes")
	return cmd
}

func unmanagedSwitchBlockers(plan activation.Plan) []string {
	var blockers []string
	for _, op := range plan.Operations {
		if op.Safety.Safe {
			continue
		}
		switch op.Safety.Class {
		case activation.SafetyUnmanagedFile, activation.SafetyUnmanagedDirectory:
			blockers = append(blockers, op.TargetPath)
		}
	}
	return blockers
}

func printSwitchResult(cmd *cobra.Command, result app.SwitchResult, verbose bool, resultErr error) {
	out := cmd.OutOrStdout()
	if result.DryRun {
		fmt.Fprintln(out, "Loki switch dry-run")
	} else if result.Plan.Profile != "" && resultErr != nil {
		fmt.Fprintln(out, "Loki switch failed")
	} else if result.Plan.Profile != "" {
		fmt.Fprintln(out, "Loki switch complete")
	}
	if result.Plan.Profile != "" {
		fmt.Fprintf(out, "Profile: %s\n", result.Plan.Profile)
	}
	if len(result.Plan.Buckets) > 0 {
		fmt.Fprintf(out, "Buckets: %s\n", strings.Join(result.Plan.Buckets, ", "))
	}
	fmt.Fprintf(out, "Operations: %d\n", len(result.Plan.Operations))
	if result.SnapshotID != "" {
		fmt.Fprintf(out, "Snapshot: %s\n", result.SnapshotID)
	}
	if len(result.CapturePlan.Changes) > 0 {
		fmt.Fprintf(out, "Local changes: %d\n", len(result.CapturePlan.Changes))
		if result.CaptureRequired {
			fmt.Fprintln(out, "Capture required: rerun with --capture-local to write safe changes back before switching")
		}
		for _, change := range result.CapturePlan.Changes {
			fmt.Fprintf(out, "- capture %s", change.TargetPath)
			if change.SourcePath != "" {
				fmt.Fprintf(out, " -> %s", change.SourcePath)
			}
			fmt.Fprintf(out, " [%s] [%s]", change.Mode, change.Status)
			if change.Message != "" {
				fmt.Fprintf(out, " %s", change.Message)
			}
			fmt.Fprintln(out)
		}
	}
	if result.Captured > 0 {
		fmt.Fprintf(out, "Captured: %d\n", result.Captured)
	}
	if len(result.CleanupPlan.Changes) > 0 {
		fmt.Fprintf(out, "Obsolete managed targets: %d\n", len(result.CleanupPlan.Changes))
		if result.DryRun || verbose {
			for _, change := range result.CleanupPlan.Changes {
				fmt.Fprintf(out, "- cleanup %s [%s]", change.TargetPath, change.Status)
				if change.Message != "" {
					fmt.Fprintf(out, " %s", change.Message)
				}
				fmt.Fprintln(out)
			}
		}
	}
	if result.Cleaned > 0 {
		fmt.Fprintf(out, "Cleaned: %d\n", result.Cleaned)
	}
	if len(result.UnmanagedBackups) > 0 {
		fmt.Fprintf(out, "Backed up unmanaged targets: %d\n", len(result.UnmanagedBackups))
		if result.UnmanagedBackupRoot != "" {
			fmt.Fprintf(out, "Backup root: %s\n", result.UnmanagedBackupRoot)
		}
		for _, backup := range result.UnmanagedBackups {
			fmt.Fprintf(out, "- backup %s -> %s [%s]\n", backup.TargetPath, backup.BackupPath, backup.SafetyClass)
		}
	}
	if blockers := unmanagedSwitchBlockers(result.Plan); len(blockers) > 0 && resultErr != nil && len(result.UnmanagedBackups) == 0 {
		fmt.Fprintf(out, "Unmanaged blockers: %d\n", len(blockers))
		fmt.Fprintln(out, "Remediation: rerun with `--backup-unmanaged --yes` to move blockers to a local backup before switching, or use `loki adopt <target> --profile <profile> [--bucket <bucket>] --yes` for local files that should become store source of truth.")
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "Warning: %s\n", warning)
	}
	if result.DryRun || verbose {
		for _, op := range result.Plan.Operations {
			fmt.Fprintf(out, "- %s %s", op.Type, op.TargetPath)
			if op.SourcePath != "" {
				fmt.Fprintf(out, " <- %s", op.SourcePath)
			}
			if op.Safety.Class != "" {
				fmt.Fprintf(out, " [%s]", op.Safety.Class)
			}
			fmt.Fprintln(out)
		}
	}
}
