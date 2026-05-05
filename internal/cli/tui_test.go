package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/config"
	lokiui "github.com/allensu/loki-profile-manager/internal/tui"
)

func TestTUIHelpPrints(t *testing.T) {
	cmd, out, _ := testCommand(t)
	cmd.SetArgs([]string{"tui", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Launch the interactive Loki terminal UI") {
		t.Fatalf("tui help missing description: %s", got)
	}
}

func TestTUIRejectsNonTTY(t *testing.T) {
	cmd, _, _ := testCommand(t)
	cmd.SetArgs([]string{"tui"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("tui error = %v", err)
	}
}

func TestTUIRunnerReceivesOptions(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	var called bool
	var gotAppOptions app.Options
	factory := func(ctx context.Context, opts app.Options) (*app.Service, error) {
		gotAppOptions = opts
		return app.NewService(ctx, opts)
	}
	runner := func(ctx context.Context, client lokiui.Client, opts lokiui.Options) error {
		called = true
		if opts.StorePath != "/tmp//loki/" || !opts.Verbose || opts.Output != out || opts.Err != errOut {
			t.Fatalf("tui options = %+v", opts)
		}
		if _, err := client.Status(ctx); err != nil {
			t.Fatalf("client status error = %v", err)
		}
		return nil
	}
	cmd := NewRootCommand(Options{
		Resolver:  config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()},
		Out:       out,
		Err:       errOut,
		Factory:   factory,
		TUIRunner: runner,
	})
	cmd.SetArgs([]string{"--store", "/tmp//loki/", "--verbose", "tui"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("tui execute error = %v", err)
	}
	if !called {
		t.Fatal("runner was not called")
	}
	if !gotAppOptions.Verbose || gotAppOptions.StoreOverride != "/tmp//loki/" {
		t.Fatalf("app options = %+v", gotAppOptions)
	}
}
