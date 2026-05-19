package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/asudbring/loki-profile-manager/internal/config"
)

type fakeUpdateRunner struct {
	lookPathErr error
	outputs     map[string]UpdateCommandResult
	errors      map[string]error
	calls       []string
}

func (r *fakeUpdateRunner) LookPath(file string) (string, error) {
	if r.lookPathErr != nil {
		return "", r.lookPathErr
	}
	return "/usr/local/bin/" + file, nil
}

func (r *fakeUpdateRunner) Run(ctx context.Context, name string, args ...string) (UpdateCommandResult, error) {
	key := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, key)
	if err := r.errors[key]; err != nil {
		return r.outputs[key], err
	}
	return r.outputs[key], nil
}

func TestCheckForUpdateFindsNewerNPMVersion(t *testing.T) {
	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{outputs: map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
	}}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	result, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: fixedUpdateNow})
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v", err)
	}
	if !result.Available || result.CurrentVersion != "v1.0.0" || result.LatestVersion != "1.2.0" {
		t.Fatalf("result = %+v, want update from v1.0.0 to 1.2.0", result)
	}
	if !strings.Contains(result.Message, "loki update") {
		t.Fatalf("message = %q, want update command", result.Message)
	}
}

func TestCheckForUpdateTreatsStableAsNewerThanPrerelease(t *testing.T) {
	oldVersion := Version
	Version = "v1.2.0-dogfood.1"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{outputs: map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
	}}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	result, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: fixedUpdateNow})
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v", err)
	}
	if !result.Available {
		t.Fatalf("result = %+v, want stable latest newer than prerelease current", result)
	}
}

func TestCheckForUpdateComparesPrereleaseIdentifiers(t *testing.T) {
	oldVersion := Version
	Version = "v1.2.0-dogfood.9"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{outputs: map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0-dogfood.10\n"},
	}}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	result, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: fixedUpdateNow})
	if err != nil {
		t.Fatalf("CheckForUpdate() error = %v", err)
	}
	if !result.Available {
		t.Fatalf("result = %+v, want dogfood.10 newer than dogfood.9", result)
	}
}

func TestCheckForUpdateUsesCachedLatestWithinTTL(t *testing.T) {
	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{outputs: map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
	}}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	first, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: fixedUpdateNow})
	if err != nil || !first.Available {
		t.Fatalf("first CheckForUpdate() = %+v, %v", first, err)
	}
	runner.outputs = map[string]UpdateCommandResult{}
	runner.errors = map[string]error{"npm view @asudbring/loki-profile-manager version --silent": errors.New("network down")}
	second, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: func() time.Time { return fixedUpdateNow().Add(time.Hour) }})
	if err != nil {
		t.Fatalf("second CheckForUpdate() error = %v", err)
	}
	if !second.FromCache || !second.Available || second.LatestVersion != "1.2.0" {
		t.Fatalf("second result = %+v, want cached update", second)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %+v, want cached second call", runner.calls)
	}
}

func TestCheckForUpdateRefreshesExpiredCache(t *testing.T) {
	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{outputs: map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.1.0\n"},
	}}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	first, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: fixedUpdateNow})
	if err != nil || first.LatestVersion != "1.1.0" {
		t.Fatalf("first CheckForUpdate() = %+v, %v", first, err)
	}
	runner.outputs = map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
	}
	second, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: func() time.Time { return fixedUpdateNow().Add(25 * time.Hour) }})
	if err != nil {
		t.Fatalf("second CheckForUpdate() error = %v", err)
	}
	if second.FromCache || second.LatestVersion != "1.2.0" {
		t.Fatalf("second result = %+v, want refreshed latest", second)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls = %+v, want expired cache refresh", runner.calls)
	}
}

func TestCheckForUpdateCachedCurrentVersionHasNoMessage(t *testing.T) {
	oldVersion := Version
	Version = "v1.2.0"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{outputs: map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
	}}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	first, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: fixedUpdateNow})
	if err != nil || first.Available || first.Message != "" {
		t.Fatalf("first CheckForUpdate() = %+v, %v; want current with no message", first, err)
	}
	runner.outputs = map[string]UpdateCommandResult{}
	second, err := svc.CheckForUpdate(context.Background(), UpdateCheckRequest{Now: func() time.Time { return fixedUpdateNow().Add(time.Hour) }})
	if err != nil {
		t.Fatalf("second CheckForUpdate() error = %v", err)
	}
	if !second.FromCache || second.Available || second.Message != "" {
		t.Fatalf("second result = %+v, want cached current with no message", second)
	}
}

func TestUpdateRunsNPMInstallWhenNewerVersionExists(t *testing.T) {
	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{outputs: map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
		"npm install -g @asudbring/loki-profile-manager@latest":     {Stdout: "changed 1 package\n"},
	}}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	result, err := svc.Update(context.Background(), UpdateRequest{Now: fixedUpdateNow})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Updated || result.LatestVersion != "1.2.0" {
		t.Fatalf("result = %+v, want updated to 1.2.0", result)
	}
	want := "npm install -g @asudbring/loki-profile-manager@latest"
	if len(runner.calls) != 2 || runner.calls[1] != want {
		t.Fatalf("runner calls = %+v, want second %q", runner.calls, want)
	}
}

func TestUpdateRunsNPMInstallEvenWhenAlreadyCurrent(t *testing.T) {
	oldVersion := Version
	Version = "v1.2.0"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{outputs: map[string]UpdateCommandResult{
		"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
		"npm install -g @asudbring/loki-profile-manager@latest":     {Stdout: "changed 1 package\n"},
	}}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	result, err := svc.Update(context.Background(), UpdateRequest{Now: fixedUpdateNow})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Updated {
		t.Fatalf("result = %+v, want npm install even when current", result)
	}
	want := "npm install -g @asudbring/loki-profile-manager@latest"
	if len(runner.calls) != 2 || runner.calls[1] != want {
		t.Fatalf("runner calls = %+v, want second %q", runner.calls, want)
	}
}

func TestUpdateRunsNPMInstallWhenVersionCheckFails(t *testing.T) {
	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{
		outputs: map[string]UpdateCommandResult{
			"npm install -g @asudbring/loki-profile-manager@latest": {Stdout: "changed 1 package\n"},
		},
		errors: map[string]error{
			"npm view @asudbring/loki-profile-manager version --silent": errors.New("network down"),
		},
	}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	result, err := svc.Update(context.Background(), UpdateRequest{Now: fixedUpdateNow})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !result.Updated || result.LatestVersion != "" {
		t.Fatalf("result = %+v, want install with unknown latest version", result)
	}
	want := "npm install -g @asudbring/loki-profile-manager@latest"
	if len(runner.calls) != 2 || runner.calls[1] != want {
		t.Fatalf("runner calls = %+v, want second %q", runner.calls, want)
	}
}

func TestUpdateReportsNPMInstallStderr(t *testing.T) {
	oldVersion := Version
	Version = "v1.0.0"
	defer func() { Version = oldVersion }()

	runner := &fakeUpdateRunner{
		outputs: map[string]UpdateCommandResult{
			"npm view @asudbring/loki-profile-manager version --silent": {Stdout: "1.2.0\n"},
			"npm install -g @asudbring/loki-profile-manager@latest":     {Stderr: "permission denied\n"},
		},
		errors: map[string]error{
			"npm install -g @asudbring/loki-profile-manager@latest": errors.New("exit status 1"),
		},
	}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	_, err := svc.Update(context.Background(), UpdateRequest{Now: fixedUpdateNow})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("Update() error = %v, want npm stderr", err)
	}
}

func TestUpdateReportsMissingNPM(t *testing.T) {
	runner := &fakeUpdateRunner{lookPathErr: errors.New("not found")}
	svc := newUpdateTestService(t, runner)
	defer svc.Close()

	_, err := svc.Update(context.Background(), UpdateRequest{Now: fixedUpdateNow})
	if err == nil || !strings.Contains(err.Error(), "npm not found") {
		t.Fatalf("Update() error = %v, want npm missing", err)
	}
}

func newUpdateTestService(t *testing.T, runner UpdateCommandRunner) *Service {
	t.Helper()
	svc, err := NewService(context.Background(), Options{Resolver: config.PathResolver{GOOS: "darwin", HomeDir: t.TempDir()}, UpdateRunner: runner})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func fixedUpdateNow() time.Time {
	return time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
}
