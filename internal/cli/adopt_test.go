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

func TestAdoptDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliMigrationStore(t)
	target := filepath.Join(home, ".gitconfig")
	cliWrite(t, target, "[user]\n\tname = CLI\n")
	cmd, out, _ := migrationTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "adopt", target, "--profile", "work", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Loki adoption dry-run") || !strings.Contains(out.String(), "Items: 1") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestAdoptSymlinkDryRunCLI(t *testing.T) {
	home := t.TempDir()
	storePath := cliMigrationStore(t)
	realTarget := filepath.Join(home, "real.gitconfig")
	linkTarget := filepath.Join(home, ".gitconfig")
	cliWrite(t, realTarget, "[user]\n\tname = CLI\n")
	if err := os.Symlink(realTarget, linkTarget); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cmd, out, _ := migrationTestCommand(home)
	cmd.SetArgs([]string{"--store", storePath, "adopt", linkTarget, "--profile", "work", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Loki adoption dry-run") || !strings.Contains(out.String(), "Items: 1") {
		t.Fatalf("output = %s", out.String())
	}
}

func migrationTestCommand(home string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: home}, Out: &out, Err: &errOut})
	return cmd, &out, &errOut
}

func cliMigrationStore(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "loki")
	if _, err := store.EnsureLayout(root); err != nil {
		t.Fatalf("EnsureLayout() error = %v", err)
	}
	return root
}
