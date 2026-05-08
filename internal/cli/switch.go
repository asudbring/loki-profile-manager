package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
)

func newSwitchCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var dryRun bool
	var yes bool
	var captureLocal bool
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
			result, err := svc.Switch(cmd.Context(), app.SwitchRequest{ParentProfile: args[0], Buckets: args[1:], DryRun: dryRun, Yes: yes, CaptureLocal: captureLocal})
			printSwitchResult(cmd, result, globals.verbose)
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and show activation plan without writing files")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm safe prompts; does not bypass unmanaged overwrite protection")
	cmd.Flags().BoolVar(&captureLocal, "capture-local", false, "write safe local managed-target changes back to the store before switching")
	return cmd
}

func printSwitchResult(cmd *cobra.Command, result app.SwitchResult, verbose bool) {
	out := cmd.OutOrStdout()
	if result.DryRun {
		fmt.Fprintln(out, "Loki switch dry-run")
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
