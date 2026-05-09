package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/storesync"
)

func newSyncCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var dryRun bool
	var yes bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Resolve local provider conflict-copy files in the Loki store.",
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

			result, err := svc.Sync(ctx, app.SyncRequest{DryRun: dryRun, Yes: yes})
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if encodeErr := encoder.Encode(result); encodeErr != nil {
					return encodeErr
				}
			} else if err == nil || len(result.Conflicts) > 0 || result.StorePath != "" {
				printSyncResult(cmd, result)
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "scan and report conflict-copy files without deleting them")
	cmd.Flags().BoolVar(&yes, "yes", false, "delete detected conflict-copy files and update heartbeat")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func printSyncResult(cmd *cobra.Command, result app.SyncResult) {
	out := cmd.OutOrStdout()
	if result.DryRun {
		fmt.Fprintln(out, "Loki sync dry-run")
		fmt.Fprintf(out, "Store: %s\n", result.StorePath)
		fmt.Fprintf(out, "Conflict copies: %d\n", len(result.Conflicts))
		fmt.Fprintf(out, "Would delete: %d\n", result.WouldDeleteCount)
		fmt.Fprintf(out, "Skipped: %d\n", result.SkippedCount)
	} else {
		fmt.Fprintln(out, "Loki sync complete")
		fmt.Fprintf(out, "Store: %s\n", result.StorePath)
		fmt.Fprintf(out, "Deleted: %d\n", result.DeletedCount)
		fmt.Fprintf(out, "Skipped: %d\n", result.SkippedCount)
		if result.HeartbeatUpdated {
			fmt.Fprintln(out, "Heartbeat: updated")
		} else {
			fmt.Fprintln(out, "Heartbeat: not updated")
		}
	}
	if result.Truncated {
		fmt.Fprintln(out, "Warning: conflict scan truncated")
	}
	printSyncConflicts(out, result.Conflicts)
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "Warning: %s\n", warning)
	}
}

func printSyncConflicts(out interface{ Write([]byte) (int, error) }, conflicts []storesync.ConflictCopy) {
	for _, conflict := range conflicts {
		fmt.Fprintf(out, "- %s %s", conflict.Action, conflict.RelativePath)
		if conflict.Kind != "" {
			fmt.Fprintf(out, " [%s]", conflict.Kind)
		}
		if conflict.Reason != "" {
			fmt.Fprintf(out, " reason=%s", conflict.Reason)
		}
		fmt.Fprintln(out)
	}
}
