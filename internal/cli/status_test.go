package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asudbring/loki-profile-manager/internal/app"
)

func TestStatusHumanOutput(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Status: not configured", "Store: not configured", "Local state:", "Next step:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q: %s", want, got)
		}
	}
}

func TestStatusJSONOutput(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var status app.StatusResult
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("status JSON invalid: %v\n%s", err, out.String())
	}
	if status.Configured {
		t.Fatal("Configured = true, want false")
	}
	if status.LocalStatePath == "" || status.DatabasePath == "" {
		t.Fatalf("paths missing in status: %+v", status)
	}
}

func TestStatusHumanShowsActiveAndManagedCount(t *testing.T) {
	home, storePath, target := prepareStatusManagedTarget(t)
	cmd, out, _ := testCommandWithHome(home)
	cmd.SetArgs([]string{"--store", storePath, "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Active profile: work", "Managed targets: 1", "Machine: registered"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, target) {
		t.Fatalf("non-verbose status listed target %q: %s", target, got)
	}
}

func TestStatusVerboseListsManagedTargets(t *testing.T) {
	home, storePath, target := prepareStatusManagedTarget(t)
	cmd, out, _ := testCommandWithHome(home)
	cmd.SetArgs([]string{"--store", storePath, "--verbose", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status verbose error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Managed target list:") || !strings.Contains(got, target) || !strings.Contains(got, "[copy]") {
		t.Fatalf("verbose status missing target detail: %s", got)
	}
}

func TestStatusJSONIncludesManagedTargets(t *testing.T) {
	home, storePath, target := prepareStatusManagedTarget(t)
	cmd, out, _ := testCommandWithHome(home)
	cmd.SetArgs([]string{"--store", storePath, "status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status JSON error = %v", err)
	}
	var status app.StatusResult
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("status JSON invalid: %v\n%s", err, out.String())
	}
	if status.ActiveProfile != "work" || status.ManagedTargetCount != 1 || len(status.ManagedTargets) != 1 || status.ManagedTargets[0].TargetPath != target {
		t.Fatalf("status = %+v", status)
	}
}

func prepareStatusManagedTarget(t *testing.T) (home, storePath, target string) {
	t.Helper()
	home = t.TempDir()
	storePath = cliSwitchStore(t, "status.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)
	cmd, _, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}
	target = filepath.ToSlash(filepath.Join(home, "status.txt"))
	return home, storePath, target
}

func TestStatusStoreOverrideJSON(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"status", "--store", "/tmp//loki/", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var status app.StatusResult
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("status JSON invalid: %v\n%s", err, out.String())
	}
	if status.StoreOverride != "/tmp/loki" {
		t.Fatalf("StoreOverride = %q", status.StoreOverride)
	}
}
