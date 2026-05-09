package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
)

func newAdoptCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var profile string
	var bucket string
	var mode string
	var sourceName string
	var dryRun bool
	var yes bool
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "adopt <target>",
		Short: "Adopt an existing local target into the Loki store.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := factory(cmd.Context(), app.Options{Resolver: resolver, StoreOverride: globals.store, Verbose: globals.verbose, Stderr: cmd.ErrOrStderr()})
			if err != nil {
				return err
			}
			defer svc.Close()
			result, err := svc.Adopt(cmd.Context(), app.AdoptRequest{Target: args[0], Profile: profile, Bucket: bucket, Mode: mode, SourceName: sourceName, DryRun: dryRun, Yes: yes})
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if encodeErr := encoder.Encode(result); encodeErr != nil {
					return encodeErr
				}
			} else {
				printAdoptResult(cmd, result)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "profile to adopt into")
	cmd.Flags().StringVar(&bucket, "bucket", "", "optional bucket to adopt into")
	cmd.Flags().StringVar(&mode, "mode", "", "adoption mode: copy, symlink, merge, or render")
	cmd.Flags().StringVar(&sourceName, "source-name", "", "relative source path to use inside the store")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show adoption plan without writing files")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm store and local-state writes")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	_ = cmd.MarkFlagRequired("profile")
	return cmd
}

func printAdoptResult(cmd *cobra.Command, result app.AdoptResult) {
	out := cmd.OutOrStdout()
	if result.DryRun {
		fmt.Fprintln(out, "Loki adoption dry-run")
	} else if result.Changed > 0 {
		fmt.Fprintln(out, "Loki adoption complete")
	}
	printMigrationPlanSummary(out, result.Plan)
}
