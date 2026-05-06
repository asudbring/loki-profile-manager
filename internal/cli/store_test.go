package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestStoreInitUseStatusAndUnsetCLI(t *testing.T) {
	home := t.TempDir()
	storePath := filepath.Join(t.TempDir(), "loki")
	cmd, out, _ := testCommandWithHome(home)
	cmd.SetArgs([]string{"store", "init", storePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("store init error = %v", err)
	}
	if !strings.Contains(out.String(), "Store initialized") || !strings.Contains(out.String(), "Valid: yes") {
		t.Fatalf("store init output = %s", out.String())
	}

	cmd, out, _ = testCommandWithHome(home)
	cmd.SetArgs([]string{"store", "status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("store status error = %v", err)
	}
	var status app.StoreStatusResult
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("store status JSON invalid: %v\n%s", err, out.String())
	}
	if status.EffectiveSource != "persisted" || !status.Valid || status.EffectiveStorePath == "" {
		t.Fatalf("status = %+v", status)
	}

	cmd, out, _ = testCommandWithHome(home)
	cmd.SetArgs([]string{"store", "unset"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("store unset error = %v", err)
	}
	if !strings.Contains(out.String(), "Store configuration cleared") {
		t.Fatalf("store unset output = %s", out.String())
	}

	cmd, out, _ = testCommandWithHome(home)
	cmd.SetArgs([]string{"store", "use", storePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("store use error = %v", err)
	}
	if !strings.Contains(out.String(), "Store configured") {
		t.Fatalf("store use output = %s", out.String())
	}
}

func TestStoreUseMissingFails(t *testing.T) {
	cmd, _, _ := testCommand(t)
	cmd.SetArgs([]string{"store", "use", filepath.Join(t.TempDir(), "missing")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("store use missing error = %v", err)
	}
}

func TestStoreDiscoverManualJSON(t *testing.T) {
	manual := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(manual); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"store", "discover", "--manual", manual, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("store discover error = %v", err)
	}
	var result app.DiscoverStoresResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("discover JSON invalid: %v\n%s", err, out.String())
	}
	if len(result.Candidates) == 0 || result.Candidates[0].StorePath == "" || !result.Candidates[0].StoreValid {
		t.Fatalf("discover result = %+v", result)
	}
}
