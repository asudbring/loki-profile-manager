package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allensu/loki-profile-manager/internal/app"
)

type fakeClient struct {
	status     app.StatusResult
	catalog    app.ProfileCatalogResult
	statusErr  error
	catalogErr error
}

func (f fakeClient) Status(context.Context) (app.StatusResult, error) {
	return f.status, f.statusErr
}

func (f fakeClient) ProfileCatalog(context.Context) (app.ProfileCatalogResult, error) {
	return f.catalog, f.catalogErr
}

func (f fakeClient) Doctor(context.Context) (app.DoctorResult, error) {
	return app.DoctorResult{}, nil
}

func (f fakeClient) MachineStatus(context.Context) (app.MachineStatusResult, error) {
	return app.MachineStatusResult{}, nil
}

func (f fakeClient) SecretsStatus(context.Context) (app.SecretsStatusResult, error) {
	return app.SecretsStatusResult{}, nil
}

func (f fakeClient) ListSnapshots(context.Context) (app.SnapshotListResult, error) {
	return app.SnapshotListResult{}, nil
}

func TestModelLoadsDashboard(t *testing.T) {
	client := fakeClient{
		status:  app.StatusResult{Configured: true, StorePath: "/tmp/loki", ActiveProfile: "work", ActiveBuckets: []string{"azure"}, ManagedTargetCount: 2, MachineID: "machine-1", MachineRegistered: true, MachineDisplayName: "laptop"},
		catalog: app.ProfileCatalogResult{Profiles: []app.ProfileSummary{{Name: "work", Buckets: []app.BucketSummary{{Name: "azure"}}}}},
	}
	model := NewModel(context.Background(), client)
	updated, _ := model.Update(dashboardLoadedMsg{status: client.status, catalog: client.catalog})
	got := updated.(Model)
	if got.screen != ScreenDashboard || got.loading || got.err != nil {
		t.Fatalf("model = %+v", got)
	}
	view := got.View()
	for _, want := range []string{"Loki Profile Manager", "Status:", "configured", "Active profile:", "work (azure)", "Profiles:", "1 profiles, 1 buckets"} {
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
