package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/verify"
)

func newVerifyCommand(resolver config.PathResolver, globals *globalOptions, factory ServiceFactory) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "verify [profile] [buckets...]",
		Short: "Verify Loki store, manifests, skills, and machine policy.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var profile string
			var buckets []string
			if len(args) > 0 {
				profile = args[0]
				buckets = args[1:]
			}
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
			report, err := svc.Verify(cmd.Context(), app.VerifyRequest{ParentProfile: profile, Buckets: buckets})
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
				printVerifyReport(cmd, report)
			}
			if !report.Valid {
				return fmt.Errorf("verification failed: %d blocking issue(s)", report.Summary.Blocking)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON")
	return cmd
}

func printVerifyReport(cmd *cobra.Command, report verify.Report) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Loki verification")
	fmt.Fprintf(out, "Store: %s\n", report.StorePath)
	if report.Profile != "" {
		fmt.Fprintf(out, "Profile: %s\n", report.Profile)
		if len(report.Buckets) > 0 {
			fmt.Fprintf(out, "Buckets: %s\n", strings.Join(report.Buckets, ", "))
		}
	}
	fmt.Fprintf(out, "Summary: %d blocking, %d warning, %d info\n", report.Summary.Blocking, report.Summary.Warnings, report.Summary.Info)
	printIssues(out, "Blocking", report.Issues, verify.SeverityBlocking)
	printIssues(out, "Warning", report.Issues, verify.SeverityWarning)
	printIssues(out, "Info", report.Issues, verify.SeverityInfo)
}

func printIssues(out interface{ Write([]byte) (int, error) }, heading string, issues []verify.Issue, severity verify.Severity) {
	printed := false
	for _, issue := range issues {
		if issue.Severity != severity {
			continue
		}
		if !printed {
			fmt.Fprintf(out, "\n%s\n", heading)
			printed = true
		}
		fmt.Fprintf(out, "- %s: %s", issue.Code, issue.Message)
		if issue.Layer != "" {
			fmt.Fprintf(out, " layer=%s", issue.Layer)
		}
		if issue.Path != "" {
			fmt.Fprintf(out, " path=%s", issue.Path)
		}
		if issue.Target != "" {
			fmt.Fprintf(out, " target=%s", issue.Target)
		}
		if issue.Remediation != "" {
			fmt.Fprintf(out, " fix=%s", issue.Remediation)
		}
		fmt.Fprintln(out)
	}
}
