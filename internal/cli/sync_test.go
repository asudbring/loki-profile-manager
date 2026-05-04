package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestSyncDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliSyncStore(t)
	conflict := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.json")
	cliWrite(t, conflict, "losing")

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "sync", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync dry-run error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Loki sync dry-run") || !strings.Contains(got, "Would delete: 1") || !strings.Contains(got, "settings conflicted copy.json") {
		t.Fatalf("sync output = %s", got)
	}
	if _, err := os.Stat(conflict); err != nil {
		t.Fatalf("dry-run deleted conflict or stat failed: %v", err)
	}
}

func TestSyncYesJSON(t *testing.T) {
	home := t.TempDir()
	storePath := cliSyncStore(t)
	registerSwitchTestMachine(t, home, storePath)
	conflict := filepath.Join(storePath, "profiles", "work", "core", "files", "settings conflicted copy.json")
	cliWrite(t, conflict, "losing")

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "sync", "--yes", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync yes error = %v", err)
	}
	var result app.SyncResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("sync JSON invalid: %v\n%s", err, out.String())
	}
	if result.DeletedCount != 1 || !result.HeartbeatUpdated || result.WouldDeleteCount != 1 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(conflict); !os.IsNotExist(err) {
		t.Fatalf("conflict still exists or stat err = %v", err)
	}
}

func TestSyncRequiresOneMode(t *testing.T) {
	cmd, _, _ := testCommand(t)
	cmd.SetArgs([]string{"sync"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("sync no-mode error = %v", err)
	}

	cmd, _, _ = testCommand(t)
	cmd.SetArgs([]string{"sync", "--dry-run", "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("sync both-mode error = %v", err)
	}
}

func cliSyncStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}
