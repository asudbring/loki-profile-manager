package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/app"
)

func TestSnapshotsListAndShowCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "snapshot.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)

	switchCmd, switchOut, _ := switchTestCommand(home)
	switchCmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}
	snapshotID := snapshotIDFromSwitchOutput(t, switchOut.String())
	target := filepath.ToSlash(filepath.Join(home, "snapshot.txt"))

	listCmd, listOut, _ := switchTestCommand(home)
	listCmd.SetArgs([]string{"snapshots", "list"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("snapshots list error = %v", err)
	}
	listText := listOut.String()
	for _, want := range []string{"Loki snapshots", snapshotID, "targets=1"} {
		if !strings.Contains(listText, want) {
			t.Fatalf("snapshots list missing %q: %s", want, listText)
		}
	}

	showCmd, showOut, _ := switchTestCommand(home)
	showCmd.SetArgs([]string{"snapshots", "show", snapshotID})
	if err := showCmd.Execute(); err != nil {
		t.Fatalf("snapshots show error = %v", err)
	}
	showText := showOut.String()
	for _, want := range []string{"Loki snapshot", snapshotID, "Targets: 1", target, "expected_hash="} {
		if !strings.Contains(showText, want) {
			t.Fatalf("snapshots show missing %q: %s", want, showText)
		}
	}
	if strings.Contains(showText, "hello") {
		t.Fatalf("snapshots show printed target file content: %s", showText)
	}
}

func TestSnapshotsListJSONCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "snapshot-json.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)

	switchCmd, _, _ := switchTestCommand(home)
	switchCmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"snapshots", "list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("snapshots list --json error = %v", err)
	}
	var result app.SnapshotListResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("snapshots list JSON invalid: %v\n%s", err, out.String())
	}
	if result.SnapshotDir == "" || len(result.Snapshots) != 1 || result.Snapshots[0].TargetCount != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func snapshotIDFromSwitchOutput(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Snapshot: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Snapshot: "))
		}
	}
	t.Fatalf("switch output missing snapshot id: %s", output)
	return ""
}
