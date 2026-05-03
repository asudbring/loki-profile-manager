package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/config"
)

func TestHelpPrints(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Loki Profile Manager") || !strings.Contains(got, "status") {
		t.Fatalf("help output missing expected text: %s", got)
	}
}

func TestNoArgsPrintsHelp(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("no-args output missing help: %s", out.String())
	}
}

func TestInvalidFlagReturnsError(t *testing.T) {
	cmd, _, _ := testCommand(t)
	cmd.SetArgs([]string{"--does-not-exist"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func testCommand(t *testing.T) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(Options{
		Resolver: config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()},
		Out:      &out,
		Err:      &errOut,
	})
	return cmd, &out, &errOut
}
