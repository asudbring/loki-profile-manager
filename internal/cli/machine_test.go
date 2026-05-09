package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/config"
	"github.com/asudbring/loki-profile-manager/internal/machine"
	"github.com/asudbring/loki-profile-manager/internal/store"
)

func TestMachineRegisterJSONWritesRegistry(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}

	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{
		"--store", storePath,
		"machine", "register",
		"--name", "test machine",
		"--allow-profile", "work",
		"--allow-profile", "dev, writer",
		"--allow-bucket", "azure,content-dev",
		"--allow-bucket", "azure",
		"--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var record machine.Record
	if err := json.Unmarshal(out.Bytes(), &record); err != nil {
		t.Fatalf("machine register JSON invalid: %v\n%s", err, out.String())
	}
	if record.MachineID == "" || record.DisplayName != "test machine" {
		t.Fatalf("record = %+v", record)
	}
	if got := strings.Join(record.AllowedParentProfiles, ","); got != "work,dev,writer" {
		t.Fatalf("AllowedParentProfiles = %q", got)
	}
	if got := strings.Join(record.AllowedBuckets, ","); got != "azure,content-dev" {
		t.Fatalf("AllowedBuckets = %q", got)
	}
	stored, ok, err := machine.GetMachine(storePath, record.MachineID)
	if err != nil || !ok {
		t.Fatalf("GetMachine() ok=%v err=%v", ok, err)
	}
	if stored.DisplayName != record.DisplayName {
		t.Fatalf("stored = %+v, record = %+v", stored, record)
	}
}

func TestMachineStatusShowsUnregisteredMachine(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	home := t.TempDir()
	cmd, _, _ := testCommandWithHome(home)
	cmd.SetArgs([]string{"--store", storePath, "switch", "work", "--dry-run"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("switch error = %v", err)
	}

	cmd, out, _ := testCommandWithHome(home)
	cmd.SetArgs([]string{"--store", storePath, "machine", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("machine status error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Machine: unregistered") || !strings.Contains(got, "loki machine register") {
		t.Fatalf("machine status output missing warning: %s", got)
	}
}

func testCommandWithHome(home string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(Options{
		Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home},
		Out:      &out,
		Err:      &errOut,
	})
	return cmd, &out, &errOut
}

func TestMachineRegisterRequiresAllowedProfileForNewMachine(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(storePath); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	cmd, _, _ := testCommand(t)
	cmd.SetArgs([]string{"--store", storePath, "machine", "register"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--allow-profile") {
		t.Fatalf("Execute() error = %v", err)
	}
}
