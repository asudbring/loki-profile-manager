package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allensu/loki-profile-manager/internal/app"
	diagnostics "github.com/allensu/loki-profile-manager/internal/doctor"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/secrets"
)

type fakeClient struct {
	status       app.StatusResult
	catalog      app.ProfileCatalogResult
	doctor       app.DoctorResult
	machine      app.MachineStatusResult
	secrets      app.SecretsStatusResult
	snapshots    app.SnapshotListResult
	statusErr    error
	catalogErr   error
	doctorErr    error
	machineErr   error
	secretsErr   error
	snapshotsErr error
}

func (f fakeClient) Status(context.Context) (app.StatusResult, error) {
	return f.status, f.statusErr
}

func (f fakeClient) ProfileCatalog(context.Context) (app.ProfileCatalogResult, error) {
	return f.catalog, f.catalogErr
}

func (f fakeClient) Doctor(context.Context) (app.DoctorResult, error) {
	return f.doctor, f.doctorErr
}

func (f fakeClient) MachineStatus(context.Context) (app.MachineStatusResult, error) {
	return f.machine, f.machineErr
}

func (f fakeClient) SecretsStatus(context.Context) (app.SecretsStatusResult, error) {
	return f.secrets, f.secretsErr
}

func (f fakeClient) ListSnapshots(context.Context) (app.SnapshotListResult, error) {
	return f.snapshots, f.snapshotsErr
}

func TestModelLoadsDashboard(t *testing.T) {
	client := populatedFakeClient()
	model := NewModel(context.Background(), client)
	updated, _ := model.Update(dashboardLoadedMsg{status: client.status, catalog: client.catalog, doctor: client.doctor, machine: client.machine, secrets: client.secrets, snapshots: client.snapshots})
	got := updated.(Model)
	if got.screen != ScreenDashboard || got.loading || got.err != nil {
		t.Fatalf("model = %+v", got)
	}
	view := got.View()
	for _, want := range []string{"Loki Profile Manager", "Status:", "configured", "Active profile:", "work (azure)", "Doctor:", "1 blocking, 2 warning, 3 info", "Secrets:", "infisical ready", "Profiles:", "1 profiles, 1 buckets", "Quick actions"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestModelShowsError(t *testing.T) {
	model := NewModel(context.Background(), fakeClient{})
	updated, _ := model.Update(dashboardLoadedMsg{err: errors.New("boom")})
	got := updated.(Model)
	if got.screen != ScreenError || got.err == nil {
		t.Fatalf("model = %+v", got)
	}
	if !strings.Contains(got.View(), "boom") {
		t.Fatalf("error view = %s", got.View())
	}
}

func TestModelQuitKeyReturnsQuitCommand(t *testing.T) {
	model := NewModel(context.Background(), fakeClient{})
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit cmd = nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit cmd returned %T", cmd())
	}
}

func TestModelRefreshReturnsLoadCommand(t *testing.T) {
	model := NewModel(context.Background(), fakeClient{})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := updated.(Model)
	if got.screen != ScreenLoading || !got.loading || cmd == nil {
		t.Fatalf("model = %+v cmd nil=%v", got, cmd == nil)
	}
}

func TestDashboardNavigationOpensDoctorAndBack(t *testing.T) {
	model := loadedModel(populatedFakeClient())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	opened := updated.(Model)
	if opened.screen != ScreenDoctor {
		t.Fatalf("screen = %s, want doctor", opened.screen)
	}
	if view := opened.View(); !strings.Contains(view, "Doctor") || !strings.Contains(view, "store_missing") {
		t.Fatalf("doctor view = %s", view)
	}
	updated, _ = opened.Update(tea.KeyMsg{Type: tea.KeyEsc})
	back := updated.(Model)
	if back.screen != ScreenDashboard {
		t.Fatalf("screen = %s, want dashboard", back.screen)
	}
}

func TestDashboardSelectionWrapsAndOpensProfiles(t *testing.T) {
	model := loadedModel(populatedFakeClient())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	wrapped := updated.(Model)
	if wrapped.selected != len(wrapped.dashboardItems())-1 {
		t.Fatalf("selected = %d", wrapped.selected)
	}
	updated, _ = wrapped.Update(tea.KeyMsg{Type: tea.KeyEnter})
	opened := updated.(Model)
	if opened.screen != ScreenProfiles || !strings.Contains(opened.View(), "azure") {
		t.Fatalf("profiles screen/view = %s\n%s", opened.screen, opened.View())
	}
}

func TestDetailScreensRenderReadOnlyData(t *testing.T) {
	model := loadedModel(populatedFakeClient())
	for key, want := range map[rune]string{
		'm': "Allowed profiles:",
		's': "CLI installed:",
		'p': "Profiles and buckets",
	} {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		got := updated.(Model)
		if !strings.Contains(got.View(), want) {
			t.Fatalf("screen %c missing %q:\n%s", key, want, got.View())
		}
	}
}

func TestSecretsViewDoesNotRenderFreeformSecretValues(t *testing.T) {
	model := loadedModel(populatedFakeClient())
	model.secrets.Checks = []secrets.Check{{Severity: secrets.SeverityWarning, Code: "TOKEN", Message: "dummy-secret-value", Remediation: "dummy-secret-value"}}
	model.screen = ScreenSecrets
	view := model.View()
	if strings.Contains(view, "dummy-secret-value") {
		t.Fatalf("secret value leaked in view:\n%s", view)
	}
	if !strings.Contains(view, "TOKEN") || !strings.Contains(view, "fix available") {
		t.Fatalf("secrets status missing safe fields:\n%s", view)
	}
}

func TestOptionalDashboardErrorsStayOnDashboard(t *testing.T) {
	client := populatedFakeClient()
	client.doctorErr = errors.New("doctor unavailable")
	client.machineErr = errors.New("machine unavailable")
	client.secretsErr = errors.New("secrets unavailable")
	client.catalogErr = errors.New("catalog unavailable")
	model := NewModel(context.Background(), client)
	msg := loadDashboardCmd(context.Background(), client)().(dashboardLoadedMsg)
	updated, _ := model.Update(msg)
	got := updated.(Model)
	if got.screen != ScreenDashboard || got.err != nil {
		t.Fatalf("model = %+v", got)
	}
	view := got.View()
	for _, want := range []string{"Doctor: error: doctor unavailable", "Machine: error: machine unavailable", "Secrets: error: secrets unavailable", "Profiles: error: catalog unavailable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestStatusErrorStillShowsErrorScreen(t *testing.T) {
	client := populatedFakeClient()
	client.statusErr = errors.New("status failed")
	model := NewModel(context.Background(), client)
	msg := loadDashboardCmd(context.Background(), client)().(dashboardLoadedMsg)
	updated, _ := model.Update(msg)
	got := updated.(Model)
	if got.screen != ScreenError || got.err == nil || !strings.Contains(got.View(), "status failed") {
		t.Fatalf("model/view = %+v\n%s", got, got.View())
	}
}

func loadedModel(client fakeClient) Model {
	model := NewModel(context.Background(), client)
	updated, _ := model.Update(dashboardLoadedMsg{status: client.status, catalog: client.catalog, doctor: client.doctor, machine: client.machine, secrets: client.secrets, snapshots: client.snapshots, catalogErr: client.catalogErr, doctorErr: client.doctorErr, machineErr: client.machineErr, secretsErr: client.secretsErr, snapshotsErr: client.snapshotsErr})
	return updated.(Model)
}

func populatedFakeClient() fakeClient {
	return fakeClient{
		status:    app.StatusResult{Configured: true, StorePath: "/tmp/loki", LocalStatePath: "/tmp/state", ActiveProfile: "work", ActiveBuckets: []string{"azure"}, ManagedTargetCount: 2, MachineID: "machine-1", MachineRegistered: true, MachineDisplayName: "laptop"},
		catalog:   app.ProfileCatalogResult{StorePath: "/tmp/loki", Profiles: []app.ProfileSummary{{Name: "work", Buckets: []app.BucketSummary{{Name: "azure"}}}}},
		doctor:    app.DoctorResult{Healthy: false, Runtime: diagnostics.RuntimeInfo{GOOS: "darwin", GOARCH: "arm64"}, StorePath: "/tmp/loki", Summary: diagnostics.Summary{Blocking: 1, Warnings: 2, Info: 3}, Checks: []diagnostics.Check{{Severity: diagnostics.SeverityBlocking, Code: "store_missing", Message: "store missing"}}},
		machine:   app.MachineStatusResult{StorePath: "/tmp/loki", MachineIDPath: "/tmp/state/machine-id", MachineID: "machine-1", Registered: true, Message: "Machine is registered.", Record: &machine.Record{MachineID: "machine-1", DisplayName: "laptop", OS: "darwin", Hostname: "host", AllowedParentProfiles: []string{"work"}, AllowedBuckets: []string{"azure"}, ActiveProfile: "work", ActiveBuckets: []string{"azure"}, LastSeen: "2026-05-05T00:00:00Z", LokiVersion: "dev"}},
		secrets:   app.SecretsStatusResult{Provider: secrets.ProviderInfisical, CLIInstalled: true, Authenticated: true, Ready: true, Checks: []secrets.Check{{Severity: secrets.SeverityInfo, Code: "auth", Message: "authenticated"}}},
		snapshots: app.SnapshotListResult{},
	}
}
