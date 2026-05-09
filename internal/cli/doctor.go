package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	diagnostics "github.com/asudbring/loki-profile-manager/internal/doctor"
)

func newDoctorCommand(resolver config.PathResolver, globals *globalOptions, _ ServiceFactory) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect Loki environment, store, machine, and recovery health.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			report, err := app.RunDoctor(ctx, app.Options{
				Resolver:      resolver,
				StoreOverride: globals.store,
				Verbose:       globals.verbose,
				Stderr:        cmd.ErrOrStderr(),
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
	return cmd
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
