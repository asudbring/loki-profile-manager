package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/app"
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
