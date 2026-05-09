package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/store"
	"github.com/asudbring/loki-profile-manager/internal/verify"
)

func TestVerifyJSONValidStore(t *testing.T) {
	storePath := testCLIStore(t)
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"--store", storePath, "verify", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var report verify.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if !report.Valid {
		t.Fatalf("report = %+v", report)
	}
}

func TestVerifyNoStoreJSONReturnsReport(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"verify", "--json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want verification failure")
	}
	var report verify.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if report.Valid || !hasCLIIssue(report, "store.not_configured") {
		t.Fatalf("report = %+v", report)
	}
}

func TestVerifyInvalidStoreReturnsError(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"--store", filepath.Join(t.TempDir(), "missing"), "verify"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want error. output=%s", out.String())
	}
	if !strings.Contains(out.String(), "Blocking") {
		t.Fatalf("human output missing blocking group: %s", out.String())
	}
}

func TestHelpIncludesVerify(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "verify") {
		t.Fatalf("help missing verify: %s", out.String())
	}
}

func testCLIStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}

func hasCLIIssue(report verify.Report, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
