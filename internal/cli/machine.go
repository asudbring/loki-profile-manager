package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/machine"
)

func newMachineCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machine",
		Short: "Manage current machine registration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newMachineRegisterCommand(resolver, globals, factory))
	cmd.AddCommand(newMachineStatusCommand(resolver, globals, factory))
	return cmd
}

func newMachineRegisterCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var displayName string
	var allowProfileFlags []string
	var allowBucketFlags []string
	var activeProfile string
	var activeBucketFlags []string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register this machine in the synced Loki registry.",
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

			allowedProfiles := normalizeMachineFlagValues(allowProfileFlags)
			allowedBuckets := normalizeMachineFlagValues(allowBucketFlags)
			activeBuckets := normalizeMachineFlagValues(activeBucketFlags)

			status, statusErr := svc.MachineStatus(ctx, app.MachineStatusRequest{})
			if statusErr == nil && status.Record != nil {
				if !cmd.Flags().Changed("name") {
					displayName = status.Record.DisplayName
				}
				if !cmd.Flags().Changed("allow-profile") {
					allowedProfiles = cloneMachineCLIStrings(status.Record.AllowedParentProfiles)
				}
				if !cmd.Flags().Changed("allow-bucket") {
					allowedBuckets = cloneMachineCLIStrings(status.Record.AllowedBuckets)
				}
				if !cmd.Flags().Changed("active-profile") {
					activeProfile = status.Record.ActiveProfile
				}
				if !cmd.Flags().Changed("active-bucket") {
					activeBuckets = cloneMachineCLIStrings(status.Record.ActiveBuckets)
				}
			}

			if len(allowedProfiles) == 0 {
				return fmt.Errorf("machine register: at least one --allow-profile is required for new registration")
			}

			record, err := svc.RegisterMachine(ctx, app.RegisterMachineRequest{
				DisplayName:           displayName,
				AllowedParentProfiles: allowedProfiles,
				AllowedBuckets:        allowedBuckets,
				ActiveProfile:         strings.TrimSpace(activeProfile),
				ActiveBuckets:         activeBuckets,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(record)
			}
			printMachineRecord(cmd, record)
			return nil
		},
	}
	cmd.Flags().StringVar(&displayName, "name", "", "human-readable machine name; defaults to hostname")
	cmd.Flags().StringArrayVar(&allowProfileFlags, "allow-profile", nil, "parent profile this machine may activate; repeat or comma-separate")
	cmd.Flags().StringArrayVar(&allowBucketFlags, "allow-bucket", nil, "bucket this machine may activate; repeat or comma-separate")
	cmd.Flags().StringVar(&activeProfile, "active-profile", "", "currently active parent profile to record")
	cmd.Flags().StringArrayVar(&activeBucketFlags, "active-bucket", nil, "currently active bucket to record; repeat or comma-separate")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func newMachineStatusCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show this machine's registration status.",
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

			status, err := svc.MachineStatus(ctx, app.MachineStatusRequest{})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(status)
			}
			printMachineStatus(cmd, status)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func printMachineRecord(cmd *cobra.Command, record machine.Record) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Machine registered")
	fmt.Fprintf(out, "ID: %s\n", record.MachineID)
	fmt.Fprintf(out, "Name: %s\n", record.DisplayName)
	fmt.Fprintf(out, "OS: %s\n", record.OS)
	fmt.Fprintf(out, "Hostname: %s\n", record.Hostname)
	fmt.Fprintf(out, "Allowed profiles: %s\n", formatMachineList(record.AllowedParentProfiles))
	fmt.Fprintf(out, "Allowed buckets: %s\n", formatMachineList(record.AllowedBuckets))
	if record.ActiveProfile != "" {
		fmt.Fprintf(out, "Active profile: %s\n", record.ActiveProfile)
	}
	if len(record.ActiveBuckets) > 0 {
		fmt.Fprintf(out, "Active buckets: %s\n", formatMachineList(record.ActiveBuckets))
	}
	fmt.Fprintln(out, "Next step: loki verify <profile> [buckets...]")
}

func printMachineStatus(cmd *cobra.Command, status app.MachineStatusResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Loki machine")
	fmt.Fprintf(out, "Store: %s\n", status.StorePath)
	fmt.Fprintf(out, "Machine ID path: %s\n", status.MachineIDPath)
	if status.MachineID == "" {
		fmt.Fprintln(out, "Machine: not registered")
		fmt.Fprintf(out, "Next step: %s\n", status.Message)
		return
	}
	fmt.Fprintf(out, "ID: %s\n", status.MachineID)
	if !status.Registered {
		fmt.Fprintln(out, "Machine: unregistered")
		if status.Warning != "" {
			fmt.Fprintf(out, "Warning: %s\n", status.Warning)
		}
		return
	}
	fmt.Fprintln(out, "Machine: registered")
	if status.Record != nil {
		fmt.Fprintf(out, "Name: %s\n", status.Record.DisplayName)
		fmt.Fprintf(out, "Allowed profiles: %s\n", formatMachineList(status.Record.AllowedParentProfiles))
		fmt.Fprintf(out, "Allowed buckets: %s\n", formatMachineList(status.Record.AllowedBuckets))
		if status.Record.ActiveProfile != "" {
			fmt.Fprintf(out, "Active profile: %s\n", status.Record.ActiveProfile)
		}
		if len(status.Record.ActiveBuckets) > 0 {
			fmt.Fprintf(out, "Active buckets: %s\n", formatMachineList(status.Record.ActiveBuckets))
		}
	}
}

func normalizeMachineFlagValues(values []string) []string {
	seen := map[string]bool{}
	normalized := []string{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			normalized = append(normalized, part)
		}
	}
	return normalized
}

func formatMachineList(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func cloneMachineCLIStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
