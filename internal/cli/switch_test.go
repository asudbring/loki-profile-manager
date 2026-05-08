package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/config"
	"github.com/allensu/loki-profile-manager/internal/store"
)

func TestSwitchDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "cli.txt", "hello")
	registerSwitchTestMachine(t, home, storePath)
	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "switch", "work", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Loki switch dry-run") || !strings.Contains(out.String(), "Operations: 1") {
		t.Fatalf("output = %s", out.String())
	}
	if _, err := os.Stat(filepath.ToSlash(filepath.Join(home, "cli.txt"))); !os.IsNotExist(err) {
		t.Fatalf("dry-run target exists or stat err = %v", err)
	}
}

func TestSwitchCLILocalChangesPrintCapturePlan(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "capture.txt", "store")
	registerSwitchTestMachine(t, home, storePath)
	cmd, _, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("initial switch error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "capture.txt"))
	if err := os.WriteFile(target, []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "switch", "work", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("capture dry-run error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Local changes: 1") || !strings.Contains(got, "Capture required") || !strings.Contains(got, "--capture-local") || !strings.Contains(got, "capture "+target) {
		t.Fatalf("output = %s", got)
	}
}

func TestSwitchCLICaptureLocalYesWritesBackAndSwitches(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "capture-yes.txt", "store")
	registerSwitchTestMachine(t, home, storePath)
	cmd, _, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "switch", "work", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("initial switch error = %v", err)
	}
	target := filepath.ToSlash(filepath.Join(home, "capture-yes.txt"))
	if err := os.WriteFile(target, []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "switch", "work", "--capture-local", "--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("capture switch error = %v output=%s", err, out.String())
	}
	if got := string(mustRead(t, filepath.Join(storePath, "profiles", "work", "core", "files", "capture-yes.txt"))); got != "local" {
		t.Fatalf("store source = %q", got)
	}
	if !strings.Contains(out.String(), "Captured: 1") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestSwitchCLIUnsafeOverwriteReturnsError(t *testing.T) {
	home := t.TempDir()
	storePath := cliSwitchStore(t, "unsafe.txt", "new")
	registerSwitchTestMachine(t, home, storePath)
	target := filepath.ToSlash(filepath.Join(home, "unsafe.txt"))
	if err := os.WriteFile(target, []byte("local"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd, out, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "switch", "work"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unsafe target overwrite") {
		t.Fatalf("Execute() error = %v output=%s", err, out.String())
	}
	if got := string(mustRead(t, target)); got != "local" {
		t.Fatalf("target changed to %q", got)
	}
}

func registerSwitchTestMachine(t *testing.T, home, storePath string) {
	t.Helper()
	cmd, _, _ := switchTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "machine", "register", "--allow-profile", "work"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("machine register error = %v", err)
	}
}

func switchTestCommand(home string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, Out: &out, Err: &errOut})
	return cmd, &out, &errOut
}

func cliSwitchStore(t *testing.T, targetName, content string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	cliWrite(t, filepath.Join(root, "profiles", "work", "core", "files", targetName), content)
	cliWrite(t, filepath.Join(root, "profiles", "work", "core", "manifest.yaml"), `version: 1
name: work-core
files:
  - id: cli-file
    source: files/`+targetName+`
    target: "~/`+targetName+`"
    mode: copy
skills: []
ignore: []
merge_rules: {}
targets: {}
`)
	return root
}

func cliWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return content
}
