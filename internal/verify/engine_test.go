package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/machine"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestRunValidStorePasses(t *testing.T) {
	root := testVerifyStore(t)
	report := Run(context.Background(), Request{StorePath: root, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}})
	if !report.Valid || report.Summary.Blocking != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunInvalidStoreBlocking(t *testing.T) {
	report := Run(context.Background(), Request{StorePath: filepath.Join(t.TempDir(), "missing")})
	if report.Valid || report.Summary.Blocking == 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunProfileAndPolicyViolation(t *testing.T) {
	root := testVerifyStore(t)
	id := "11111111-1111-4111-8111-111111111111"
	if err := machine.UpsertMachine(root, machine.Record{MachineID: id, AllowedParentProfiles: []string{"dev"}, AllowedBuckets: []string{}, LastSeen: "2026-05-03T00:00:00Z"}); err != nil {
		t.Fatalf("UpsertMachine() error = %v", err)
	}
	report := Run(context.Background(), Request{StorePath: root, ParentProfile: "work", MachineID: id, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}})
	if report.Valid || !hasIssue(report, "machine.policy_blocked") {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunMachineRecordMissingHasRemediation(t *testing.T) {
	root := testVerifyStore(t)
	machineIDPath := filepath.Join(t.TempDir(), "machine_id")
	if _, err := machine.EnsureID(machineIDPath); err != nil {
		t.Fatalf("EnsureID() error = %v", err)
	}
	report := Run(context.Background(), Request{StorePath: root, ParentProfile: "work", MachineIDPath: machineIDPath, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}})
	issue, ok := findIssue(report, "machine.record_missing")
	if !ok {
		t.Fatalf("report missing machine.record_missing: %+v", report)
	}
	if issue.Remediation == "" || !strings.Contains(issue.Remediation, "loki machine register --allow-profile work") {
		t.Fatalf("record_missing remediation = %q", issue.Remediation)
	}
}

func TestRunRegisteredMachineNoRecordMissing(t *testing.T) {
	root := testVerifyStore(t)
	machineIDPath := filepath.Join(t.TempDir(), "machine_id")
	id, err := machine.EnsureID(machineIDPath)
	if err != nil {
		t.Fatalf("EnsureID() error = %v", err)
	}
	if err := machine.UpsertMachine(root, machine.Record{MachineID: id, AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{}, LastSeen: "2026-05-03T00:00:00Z"}); err != nil {
		t.Fatalf("UpsertMachine() error = %v", err)
	}
	report := Run(context.Background(), Request{StorePath: root, ParentProfile: "work", MachineIDPath: machineIDPath, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}})
	if !report.Valid || hasIssue(report, "machine.record_missing") {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunDoesNotCreateMachineID(t *testing.T) {
	root := testVerifyStore(t)
	machineIDPath := filepath.Join(t.TempDir(), "machine_id")
	report := Run(context.Background(), Request{StorePath: root, ParentProfile: "work", MachineIDPath: machineIDPath, Resolver: config.PathResolver{GOOS: "darwin", HomeDir: "/Users/alice"}})
	if !report.Valid {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(machineIDPath); !os.IsNotExist(err) {
		t.Fatalf("machine id was created or stat failed: %v", err)
	}
}

func testVerifyStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}

func hasIssue(report Report, code string) bool {
	_, ok := findIssue(report, code)
	return ok
}

func findIssue(report Report, code string) (Issue, bool) {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return issue, true
		}
	}
	return Issue{}, false
}
