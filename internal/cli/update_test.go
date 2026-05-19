package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/config"
	lokiui "github.com/asudbring/loki-profile-manager/internal/tui"
)

func TestUpdateCLIPrintsUpdateResult(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	runner := &cliFakeUpdateRunner{outputs: map[string]app.UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
		"npm install -g @asudbring/loki-profile-manager@latest":     {Stdout: "changed 1 package\n"},
	}}
	cmd, out, _ := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"update"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update command error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Loki update complete") || !strings.Contains(got, "1.2.0") {
		t.Fatalf("update output = %q", got)
	}
}

func TestRootUpdateNoticePrintsForHumanCommand(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	runner := &cliFakeUpdateRunner{outputs: map[string]app.UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
	}}
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command error = %v", err)
	}
	got := errOut.String()
	if !strings.Contains(got, "Newer Loki version available: 1.2.0") || !strings.Contains(got, "loki update") {
		t.Fatalf("stderr update notice = %q", got)
	}
}

func TestRootUpdateNoticeSwallowsCheckFailure(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	runner := &cliFakeUpdateRunner{errors: map[string]error{
		"npm view @asudbring/loki-profile-manager version --silent": errors.New("network down"),
	}}
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command error = %v", err)
	}
	assertNoUpdateNoticeText(t, errOut)
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %+v, want one failed update check", runner.calls)
	}
}

func TestRootUpdateNoticeSkipsJSONCommand(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	runner := newNoticeTestRunner()
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status --json command error = %v", err)
	}
	assertNoUpdateNotice(t, runner, errOut)
}

func TestRootUpdateNoticeSkipsUpdateCommand(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	runner := &cliFakeUpdateRunner{outputs: map[string]app.UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
		"npm install -g @asudbring/loki-profile-manager@latest":     {Stdout: "changed 1 package\n"},
	}}
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"update"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update command error = %v", err)
	}
	assertNoUpdateNoticeText(t, errOut)
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls = %+v, want update check and install only", runner.calls)
	}
}

func TestRootUpdateNoticeSkipsVersionCommand(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	runner := newNoticeTestRunner()
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command error = %v", err)
	}
	assertNoUpdateNotice(t, runner, errOut)
}

func TestRootUpdateNoticeSkipsHelpCommand(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	runner := newNoticeTestRunner()
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"status", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help command error = %v", err)
	}
	assertNoUpdateNotice(t, runner, errOut)
}

func TestRootUpdateNoticeSkipsDevelopmentVersion(t *testing.T) {
	withUpdateTestVersion(t, "dev")

	runner := newNoticeTestRunner()
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command error = %v", err)
	}
	assertNoUpdateNotice(t, runner, errOut)
}

func TestRootUpdateNoticeSkipsDisabledEnv(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")
	t.Setenv(app.UpdateDisableEnvVar, "1")

	runner := newNoticeTestRunner()
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command error = %v", err)
	}
	assertNoUpdateNotice(t, runner, errOut)
}

func TestRootUpdateNoticeSkipsCIEnv(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")
	t.Setenv("GITHUB_ACTIONS", "true")

	runner := newNoticeTestRunner()
	cmd, _, errOut := testCommandWithUpdateRunner(t, runner)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command error = %v", err)
	}
	assertNoUpdateNotice(t, runner, errOut)
}

func TestRootUpdateNoticeSkipsTUICommand(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	runner := newNoticeTestRunner()
	var tuiCalled bool
	cmd, _, errOut := testCommandWithUpdateRunnerAndTUI(t, runner, func(ctx context.Context, client lokiui.Client, opts lokiui.Options) error {
		tuiCalled = true
		return nil
	})
	cmd.SetArgs([]string{"tui"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("tui command error = %v", err)
	}
	if !tuiCalled {
		t.Fatal("TUI runner was not called")
	}
	assertNoUpdateNotice(t, runner, errOut)
}

func TestRootUpdateNoticeSkipsNoninteractiveStderr(t *testing.T) {
	withUpdateTestVersion(t, "v1.0.0")

	errRead, errWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer errRead.Close()
	defer errWrite.Close()

	runner := newNoticeTestRunner()
	var out bytes.Buffer
	cmd := NewRootCommand(Options{
		Resolver:     config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()},
		Out:          &out,
		Err:          errWrite,
		UpdateRunner: runner,
	})
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status command error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %+v, want no update check", runner.calls)
	}
}

func withUpdateTestVersion(t *testing.T, version string) {
	t.Helper()
	oldVersion := app.Version
	app.Version = version
	t.Cleanup(func() { app.Version = oldVersion })
}

func newNoticeTestRunner() *cliFakeUpdateRunner {
	return &cliFakeUpdateRunner{outputs: map[string]app.UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
	}}
}

func assertNoUpdateNotice(t *testing.T, runner *cliFakeUpdateRunner, errOut *bytes.Buffer) {
	t.Helper()
	assertNoUpdateNoticeText(t, errOut)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %+v, want no update check", runner.calls)
	}
}

func assertNoUpdateNoticeText(t *testing.T, errOut *bytes.Buffer) {
	t.Helper()
	if got := errOut.String(); got != "" {
		t.Fatalf("stderr update notice = %q, want empty", got)
	}
}

type cliFakeUpdateRunner struct {
	lookPathErr error
	outputs     map[string]app.UpdateCommandResult
	errors      map[string]error
	calls       []string
}

func (r *cliFakeUpdateRunner) LookPath(file string) (string, error) {
	if r.lookPathErr != nil {
		return "", r.lookPathErr
	}
	return "/usr/local/bin/" + file, nil
}

func (r *cliFakeUpdateRunner) Run(ctx context.Context, name string, args ...string) (app.UpdateCommandResult, error) {
	key := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, key)
	if err := r.errors[key]; err != nil {
		return r.outputs[key], err
	}
	return r.outputs[key], nil
}

func testCommandWithUpdateRunner(t *testing.T, runner app.UpdateCommandRunner) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	return testCommandWithUpdateRunnerAndTUI(t, runner, nil)
}

func testCommandWithUpdateRunnerAndTUI(t *testing.T, runner app.UpdateCommandRunner, tuiRunner TUIRunner) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(Options{
		Resolver:     config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()},
		Out:          &out,
		Err:          &errOut,
		UpdateRunner: runner,
		TUIRunner:    tuiRunner,
	})
	return cmd, &out, &errOut
}
