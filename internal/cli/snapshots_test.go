package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
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

func TestSnapshotsRestoreRequiresMode(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "restore-required.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)
	switchCmd, switchOut, _ := switchTestCommand(home)
	switchCmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}
	snapshotID := snapshotIDFromSwitchOutput(t, switchOut.String())

	cmd, _, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"snapshots", "restore", snapshotID})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("restore without dry-run error = %v", err)
	}
}

func TestSnapshotsRestoreDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "restore.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)
	switchCmd, switchOut, _ := switchTestCommand(home)
	switchCmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}
	snapshotID := snapshotIDFromSwitchOutput(t, switchOut.String())
	target := filepath.ToSlash(filepath.Join(home, "restore.txt"))

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"snapshots", "restore", snapshotID, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("snapshots restore --dry-run error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Loki snapshot restore dry-run", snapshotID, "no files or local state were changed", "Guard: recorded", "remove_created_target", target} {
		if !strings.Contains(got, want) {
			t.Fatalf("restore dry-run missing %q: %s", want, got)
		}
	}
	if content := string(mustRead(t, target)); content != "hello" {
		t.Fatalf("target changed to %q", content)
	}
}

func TestSnapshotsRestoreDryRunTargetCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "restore-target.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)
	switchCmd, switchOut, _ := switchTestCommand(home)
	switchCmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}
	snapshotID := snapshotIDFromSwitchOutput(t, switchOut.String())
	target := filepath.ToSlash(filepath.Join(home, "restore-target.txt"))

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"snapshots", "restore", snapshotID, "--target", target, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("restore target dry-run error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Target filter:", target, "Guard: recorded", "--target " + target} {
		if !strings.Contains(got, want) {
			t.Fatalf("restore target dry-run missing %q: %s", want, got)
		}
	}
}

func TestSnapshotsRestoreYesAfterDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "restore-yes.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)
	switchCmd, switchOut, _ := switchTestCommand(home)
	switchCmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}
	snapshotID := snapshotIDFromSwitchOutput(t, switchOut.String())
	target := filepath.ToSlash(filepath.Join(home, "restore-yes.txt"))

	dryRunCmd, _, _ := switchTestCommand(home)
	dryRunCmd.SetArgs([]string{"snapshots", "restore", snapshotID, "--dry-run"})
	if err := dryRunCmd.Execute(); err != nil {
		t.Fatalf("restore dry-run error = %v", err)
	}
	restoreCmd, out, _ := switchTestCommand(home)
	restoreCmd.SetArgs([]string{"snapshots", "restore", snapshotID, "--yes"})
	if err := restoreCmd.Execute(); err != nil {
		t.Fatalf("restore --yes error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Loki snapshot restore complete", "Pre-restore snapshot:", "Changed targets: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("restore --yes output missing %q: %s", want, got)
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after restore or stat err = %v", err)
	}
}

func TestSnapshotsRestoreDryRunJSONCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "restore-json.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)
	switchCmd, switchOut, _ := switchTestCommand(home)
	switchCmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}
	snapshotID := snapshotIDFromSwitchOutput(t, switchOut.String())

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"snapshots", "restore", snapshotID, "--dry-run", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("snapshots restore --json error = %v", err)
	}
	var result app.SnapshotRestoreDryRunResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("restore JSON invalid: %v\n%s", err, out.String())
	}
	if !result.DryRun || result.WouldWrite || !result.GuardRecorded || result.Summary.TargetCount != 1 || result.Summary.RemoveCreatedTargetCount != 1 {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(out.String(), "hello") {
		t.Fatalf("restore JSON printed file content: %s", out.String())
	}
}

func TestSnapshotsRestoreRejectsDryRunAndYes(t *testing.T) {
	cmd, _, _ := switchTestCommand(t.TempDir())
	cmd.SetArgs([]string{"snapshots", "restore", "snapshot-id", "--dry-run", "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("restore --dry-run --yes error = %v", err)
	}
}

func TestSnapshotsRestoreYesRequiresGuard(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "restore-yes-guard.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)
	switchCmd, switchOut, _ := switchTestCommand(home)
	switchCmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := switchCmd.Execute(); err != nil {
		t.Fatalf("switch error = %v", err)
	}
	snapshotID := snapshotIDFromSwitchOutput(t, switchOut.String())

	cmd, _, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"snapshots", "restore", snapshotID, "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "dry-run guard") {
		t.Fatalf("restore --yes error = %v", err)
	}
}

func TestSnapshotsRestoreDryRunRedactsSensitivePaths(t *testing.T) {
	home := t.TempDir()
	resolver := config.PathResolver{GOOS: "darwin", HomeDir: home}
	paths, err := resolver.ResolveLocalPaths()
	if err != nil {
		t.Fatalf("ResolveLocalPaths() error = %v", err)
	}
	snapshotID := "20260504T000000Z-sensitive"
	snapshot := activation.Snapshot{
		SnapshotID: snapshotID,
		Path:       filepath.Join(paths.SnapshotDir, snapshotID),
		CreatedAt:  "2026-05-04T00:00:00Z",
		Targets: []activation.SnapshotEntry{{
			TargetPath: filepath.ToSlash(filepath.Join(home, ".ssh", "id_ed25519")),
			Kind:       "missing",
		}},
	}
	cliWrite(t, snapshot.Targets[0].TargetPath, "not-a-real-key")
	if err := activation.PersistSnapshot(context.Background(), nil, snapshot); err != nil {
		t.Fatalf("PersistSnapshot() error = %v", err)
	}

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"snapshots", "restore", snapshotID, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("snapshots restore sensitive error = %v", err)
	}
	got := out.String()
	for _, banned := range []string{".ssh", "id_ed25519", "not-a-real-key"} {
		if strings.Contains(got, banned) {
			t.Fatalf("restore output leaked sensitive path fragment %q: %s", banned, got)
		}
	}
	if !strings.Contains(got, "redacted-sensitive-path") {
		t.Fatalf("restore output did not redact sensitive path: %s", got)
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
