package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	diagnostics "github.com/asudbring/loki-profile-manager/internal/doctor"
)

func newDoctorCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	var repairManagedState bool
	var writeSafeFiles bool
	var resolveBlockers bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect Loki environment, store, machine, and recovery health.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if writeSafeFiles && !repairManagedState {
				return fmt.Errorf("doctor: --write-safe-files requires --repair-managed-state")
			}
			ctx := cmd.Context()

			if resolveBlockers {
				return runDoctorResolveBlockers(cmd, resolver, globals, factory)
			}

			report, err := app.RunDoctor(ctx, app.Options{
				Resolver:                 resolver,
				StoreOverride:            globals.store,
				Verbose:                  globals.verbose,
				Stderr:                   cmd.ErrOrStderr(),
				DoctorRepairManagedState: repairManagedState,
				DoctorWriteSafeFiles:     writeSafeFiles,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(report); err != nil {
					return err
				}
			} else {
				printDoctorReport(cmd, report)
			}
			if !report.Healthy {
				return fmt.Errorf("doctor found %d blocking issue(s)", report.Summary.Blocking)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	cmd.Flags().BoolVar(&repairManagedState, "repair-managed-state", false, "repair safe stale managed-target state records")
	cmd.Flags().BoolVar(&writeSafeFiles, "write-safe-files", false, "with --repair-managed-state, canonicalize safe local files before repairing state")
	cmd.Flags().BoolVar(&resolveBlockers, "resolve-blockers", false, "interactively resolve switch blockers by promoting local overrides into a store layer")
	return cmd
}

func runDoctorResolveBlockers(cmd *cobra.Command, resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	svc, err := factory(ctx, app.Options{
		Resolver:      resolver,
		StoreOverride: globals.store,
		Verbose:       globals.verbose,
		Stderr:        cmd.ErrOrStderr(),
	})
	if err != nil {
		return fmt.Errorf("doctor --resolve-blockers: %w", err)
	}
	defer svc.Close()

	blockers, err := svc.FindSwitchBlockers(ctx)
	if err != nil {
		return fmt.Errorf("doctor --resolve-blockers: %w", err)
	}
	if len(blockers) == 0 {
		fmt.Fprintln(out, "No switch blockers found. Switch should proceed without issues.")
		return nil
	}

	fmt.Fprintf(out, "Found %d switch blocker(s):\n\n", len(blockers))
	reader := bufio.NewReader(os.Stdin)

	for i, blocker := range blockers {
		fmt.Fprintf(out, "Blocker %d/%d\n", i+1, len(blockers))
		fmt.Fprintf(out, "  Target: %s\n", blocker.TargetPath)
		fmt.Fprintf(out, "  Mode:   %s\n", blocker.Change.Mode)
		fmt.Fprintf(out, "  Status: %s\n", blocker.Change.Status)
		if blocker.Change.Message != "" {
			fmt.Fprintf(out, "  Reason: %s\n", blocker.Change.Message)
		}
		fmt.Fprintf(out, "\n  Available layers to own these overrides:\n")
		for j, layer := range blocker.AvailableLayers {
			fmt.Fprintf(out, "    [%d] %s (%s)\n", j+1, layer.Name, layer.Kind)
			fmt.Fprintf(out, "        source: %s\n", layer.SourcePath)
		}
		fmt.Fprintf(out, "    [s] skip this blocker\n")
		fmt.Fprintf(out, "\n  Which layer should own the local overrides? ")

		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("doctor --resolve-blockers: reading input: %w", err)
		}
		input = strings.TrimSpace(input)

		if strings.EqualFold(input, "s") || input == "" {
			fmt.Fprintln(out, "  Skipped.")
			fmt.Fprintln(out)
			continue
		}

		choice, err := strconv.Atoi(input)
		if err != nil || choice < 1 || choice > len(blocker.AvailableLayers) {
			fmt.Fprintf(out, "  Invalid choice %q, skipping.\n\n", input)
			continue
		}

		chosen := blocker.AvailableLayers[choice-1]
		fmt.Fprintf(out, "  Promoting local content to layer %q (%s)...\n", chosen.Name, chosen.SourcePath)

		if err := svc.ResolveBlocker(ctx, blocker, chosen); err != nil {
			fmt.Fprintf(out, "  Error: %v\n\n", err)
			continue
		}
		fmt.Fprintln(out, "  Resolved.")
		fmt.Fprintln(out)
	}

	// Run a verification dry-run.
	fmt.Fprintln(out, "Running verification dry-run...")
	verifyBlockers, err := svc.FindSwitchBlockers(ctx)
	if err != nil {
		fmt.Fprintf(out, "Verification failed: %v\n", err)
		return fmt.Errorf("doctor --resolve-blockers: verification: %w", err)
	}
	if len(verifyBlockers) == 0 {
		fmt.Fprintln(out, "All switch blockers resolved. Switch should now proceed without issues.")
		return nil
	}
	fmt.Fprintf(out, "%d blocker(s) remain. Run `loki doctor --resolve-blockers` again or resolve manually.\n", len(verifyBlockers))
	return fmt.Errorf("doctor: %d switch blocker(s) still unresolved", len(verifyBlockers))
}

func printDoctorReport(cmd *cobra.Command, report app.DoctorResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Loki doctor")
	fmt.Fprintf(out, "Version: %s\n", report.Version)
	fmt.Fprintf(out, "Runtime: %s/%s\n", report.Runtime.GOOS, report.Runtime.GOARCH)
	if report.StorePath != "" {
		fmt.Fprintf(out, "Store: %s\n", report.StorePath)
	} else {
		fmt.Fprintln(out, "Store: not configured")
	}
	if report.StoreOverride != "" {
		fmt.Fprintf(out, "Store override: %s\n", report.StoreOverride)
	}
	fmt.Fprintf(out, "Local state: %s\n", report.LocalPaths.StateDir)
	fmt.Fprintf(out, "Database: %s\n", report.LocalPaths.DBPath)
	fmt.Fprintf(out, "Summary: %d blocking, %d warning, %d info\n", report.Summary.Blocking, report.Summary.Warnings, report.Summary.Info)
	printDoctorChecks(out, "Blocking", report.Checks, diagnostics.SeverityBlocking)
	printDoctorChecks(out, "Warning", report.Checks, diagnostics.SeverityWarning)
	printDoctorChecks(out, "Info", report.Checks, diagnostics.SeverityInfo)
}

func printDoctorChecks(out interface{ Write([]byte) (int, error) }, heading string, checks []diagnostics.Check, severity diagnostics.Severity) {
	printed := false
	for _, check := range checks {
		if check.Severity != severity {
			continue
		}
		if !printed {
			fmt.Fprintf(out, "\n%s\n", heading)
			printed = true
		}
		fmt.Fprintf(out, "- %s: %s", check.Code, check.Message)
		if check.Path != "" {
			fmt.Fprintf(out, " path=%s", check.Path)
		}
		if len(check.Details) > 0 {
			fmt.Fprintf(out, " details=%s", formatDoctorDetails(check.Details))
		}
		if check.Remediation != "" {
			fmt.Fprintf(out, " fix=%s", check.Remediation)
		}
		fmt.Fprintln(out)
	}
}

func formatDoctorDetails(details map[string]string) string {
	parts := make([]string, 0, len(details))
	for key, value := range details {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
