package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
)

func newStatusCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local Loki status.",
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

			status, err := svc.Status(ctx, app.StatusRequest{})
			if err != nil {
				return err
			}

			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(status)
			}
			printHumanStatus(cmd, status, globals.verbose)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func printHumanStatus(cmd *cobra.Command, status app.StatusResult, verbose bool) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Loki Profile Manager")
	fmt.Fprintln(out)
	if status.Configured {
		fmt.Fprintln(out, "Status: configured")
	} else {
		fmt.Fprintln(out, "Status: not configured")
	}
	if status.StoreOverride != "" {
		fmt.Fprintf(out, "Store override: %s\n", status.StoreOverride)
	} else if status.StorePath != "" {
		fmt.Fprintf(out, "Store: %s\n", status.StorePath)
	} else {
		fmt.Fprintln(out, "Store: not configured")
	}
	fmt.Fprintf(out, "Local state: %s\n", status.LocalStatePath)
	fmt.Fprintf(out, "Database: %s\n", status.DatabasePath)
	if status.ActiveProfile != "" {
		fmt.Fprintf(out, "Active profile: %s", status.ActiveProfile)
		if status.ActiveSource != "" {
			fmt.Fprintf(out, " (%s)", status.ActiveSource)
		}
		fmt.Fprintln(out)
		if len(status.ActiveBuckets) > 0 {
			fmt.Fprintf(out, "Active buckets: %s\n", strings.Join(status.ActiveBuckets, ", "))
		}
	} else {
		fmt.Fprintln(out, "Active profile: not set")
	}
	fmt.Fprintf(out, "Managed targets: %d\n", status.ManagedTargetCount)
	if verbose && len(status.ManagedTargets) > 0 {
		printManagedTargets(out, status.ManagedTargets)
	}
	if status.Configured {
		printStatusMachine(out, status)
	}
	if len(status.Missing) > 0 {
		fmt.Fprintf(out, "Missing: %d paths\n", len(status.Missing))
	}
	fmt.Fprintln(out)
	if !status.Configured && status.StorePath == "" {
		fmt.Fprintf(out, "Next step: %s\n", status.Message)
		return
	}
	fmt.Fprintln(out, status.Message)
}

func printManagedTargets(out interface{ Write([]byte) (int, error) }, targets []app.StatusManagedTarget) {
	fmt.Fprintln(out, "Managed target list:")
	for _, target := range targets {
		fmt.Fprintf(out, "- %s [%s]", target.TargetPath, target.Mode)
		if target.LayerKind != "" || target.LayerName != "" {
			fmt.Fprintf(out, " layer=%s/%s", target.LayerKind, target.LayerName)
		}
		if target.SourcePath != "" {
			fmt.Fprintf(out, " source=%s", target.SourcePath)
		}
		if target.LastAppliedAt != "" {
			fmt.Fprintf(out, " last_applied=%s", target.LastAppliedAt)
		}
		fmt.Fprintln(out)
	}
}

func printStatusMachine(out interface{ Write([]byte) (int, error) }, status app.StatusResult) {
	if status.MachineID == "" {
		fmt.Fprintln(out, "Machine: not registered")
		if status.MachineWarning != "" {
			fmt.Fprintf(out, "Machine warning: %s\n", status.MachineWarning)
		}
		if status.MachineMessage != "" {
			fmt.Fprintf(out, "Machine next step: %s\n", status.MachineMessage)
		}
		return
	}
	if status.MachineRegistered {
		fmt.Fprintf(out, "Machine: registered (%s)\n", status.MachineID)
		if status.MachineDisplayName != "" {
			fmt.Fprintf(out, "Machine name: %s\n", status.MachineDisplayName)
		}
		if len(status.MachineAllowedParentProfiles) > 0 {
			fmt.Fprintf(out, "Allowed profiles: %s\n", strings.Join(status.MachineAllowedParentProfiles, ", "))
		}
		if len(status.MachineAllowedBuckets) > 0 {
			fmt.Fprintf(out, "Allowed buckets: %s\n", strings.Join(status.MachineAllowedBuckets, ", "))
		}
		return
	}
	fmt.Fprintf(out, "Machine: unregistered (%s)\n", status.MachineID)
	if status.MachineWarning != "" {
		fmt.Fprintf(out, "Machine warning: %s\n", status.MachineWarning)
	}
}
