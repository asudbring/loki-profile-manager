package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
	lokiui "github.com/allensu/loki-profile-manager/internal/tui"
)

func TestHelpPrints(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Loki Profile Manager") || !strings.Contains(got, "status") || !strings.Contains(got, "machine") {
		t.Fatalf("help output missing expected text: %s", got)
	}
}

func TestNoArgsLaunchesTUI(t *testing.T) {
	var called bool
	cmd := NewRootCommand(Options{
		Resolver: config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()},
		TUIRunner: func(ctx context.Context, client lokiui.Client, opts lokiui.Options) error {
			called = true
			if opts.StorePath != "" || opts.Verbose {
				t.Fatalf("tui options = %+v", opts)
			}
			return nil
		},
	})
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("TUI runner was not called")
	}
}

func TestRootWithGlobalFlagPrintsHelp(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"--verbose"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("global-flag root output missing help: %s", out.String())
	}
}

func TestInvalidFlagReturnsError(t *testing.T) {
	cmd, _, _ := testCommand(t)
	cmd.SetArgs([]string{"--does-not-exist"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestVersionFlagPrintsAppVersion(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != app.Version {
		t.Fatalf("version output = %q, want %q", got, app.Version)
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
