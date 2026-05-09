package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/app"
)

func TestDoctorHumanOutput(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Loki doctor", "Store: not configured", "Local state:", "Database:", "Summary:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q: %s", want, got)
		}
	}
}

func TestDoctorJSONOutput(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"doctor", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor JSON error = %v", err)
	}
	var report app.DoctorResult
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("doctor JSON invalid: %v\n%s", err, out.String())
	}
	if report.Version == "" || report.LocalPaths.StateDir == "" || len(report.Checks) == 0 {
		t.Fatalf("doctor report incomplete: %+v", report)
	}
	if !hasDoctorCheck(report, "store.not_configured") {
		t.Fatalf("doctor report missing store.not_configured: %+v", report.Checks)
	}
}

func TestDoctorInvalidStorePrintsReportAndFails(t *testing.T) {
	missingStore := filepath.Join(t.TempDir(), "missing-loki")
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"--store", missingStore, "doctor"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "doctor found") {
		t.Fatalf("doctor error = %v, want blocking issue", err)
	}
	got := out.String()
	if !strings.Contains(got, "store.root_missing") || !strings.Contains(got, missingStore) {
		t.Fatalf("doctor output missing blocking store issue: %s", got)
	}
}

func hasDoctorCheck(report app.DoctorResult, code string) bool {
	for _, check := range report.Checks {
		if check.Code == code {
			return true
		}
	}
	return false
}
