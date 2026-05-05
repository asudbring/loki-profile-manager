package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allensu/loki-profile-manager/internal/activation"
	"github.com/allensu/loki-profile-manager/internal/app"
	diagnostics "github.com/allensu/loki-profile-manager/internal/doctor"
	"github.com/allensu/loki-profile-manager/internal/machine"
	"github.com/allensu/loki-profile-manager/internal/secrets"
	"github.com/allensu/loki-profile-manager/internal/storesync"
)

type fakeSwitchResult struct {
	result app.SwitchResult
	err    error
}

type fakeSyncResult struct {
	result app.SyncResult
	err    error
}

type fakeSnapshotShowResult struct {
	result app.SnapshotShowResult
	err    error
}

type fakeSnapshotRestoreResult struct {
	result app.SnapshotRestoreDryRunResult
	err    error
}

type fakeClient struct {
	status                 app.StatusResult
	catalog                app.ProfileCatalogResult
	doctor                 app.DoctorResult
	machine                app.MachineStatusResult
	secrets                app.SecretsStatusResult
	snapshots              app.SnapshotListResult
	statusErr              error
	catalogErr             error
	doctorErr              error
	machineErr             error
	secretsErr             error
	snapshotsErr           error
	switchResults          []fakeSwitchResult
	switchCalls            *[]app.SwitchRequest
	syncResults            []fakeSyncResult
	syncCalls              *[]app.SyncRequest
	snapshotShowResults    []fakeSnapshotShowResult
	snapshotShowCalls      *[]app.SnapshotShowRequest
	snapshotRestoreResults []fakeSnapshotRestoreResult
	snapshotRestoreCalls   *[]app.SnapshotRestoreDryRunRequest
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

func (f fakeClient) ShowSnapshot(ctx context.Context, req app.SnapshotShowRequest) (app.SnapshotShowResult, error) {
	_ = ctx
	idx := 0
	if f.snapshotShowCalls != nil {
		idx = len(*f.snapshotShowCalls)
		*f.snapshotShowCalls = append(*f.snapshotShowCalls, req)
	}
	if idx < len(f.snapshotShowResults) {
		return f.snapshotShowResults[idx].result, f.snapshotShowResults[idx].err
	}
	return snapshotShowResult(req.SnapshotID, "/tmp/loki/target.txt"), nil
}

func (f fakeClient) RestoreSnapshotDryRun(ctx context.Context, req app.SnapshotRestoreDryRunRequest) (app.SnapshotRestoreDryRunResult, error) {
	_ = ctx
	idx := 0
	if f.snapshotRestoreCalls != nil {
		idx = len(*f.snapshotRestoreCalls)
		*f.snapshotRestoreCalls = append(*f.snapshotRestoreCalls, req)
	}
	if idx < len(f.snapshotRestoreResults) {
		return f.snapshotRestoreResults[idx].result, f.snapshotRestoreResults[idx].err
	}
	return snapshotRestoreDryRunResult(req.SnapshotID, req.Target, false, true), nil
}

func (f fakeClient) Switch(ctx context.Context, req app.SwitchRequest) (app.SwitchResult, error) {
	_ = ctx
	idx := 0
	if f.switchCalls != nil {
		idx = len(*f.switchCalls)
		*f.switchCalls = append(*f.switchCalls, req)
	}
	if idx < len(f.switchResults) {
		return f.switchResults[idx].result, f.switchResults[idx].err
	}
	return switchResult(req.ParentProfile, req.Buckets, req.DryRun, 1), nil
}

func (f fakeClient) Sync(ctx context.Context, req app.SyncRequest) (app.SyncResult, error) {
	_ = ctx
	idx := 0
	if f.syncCalls != nil {
		idx = len(*f.syncCalls)
		*f.syncCalls = append(*f.syncCalls, req)
	}
	if idx < len(f.syncResults) {
		return f.syncResults[idx].result, f.syncResults[idx].err
	}
	if req.Yes {
		return syncResult(1, 0, "fp-1", false), nil
	}
	return syncResult(1, 1, "fp-1", true), nil
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
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

func TestSwitchDryRunAndConfirmExecute(t *testing.T) {
	calls := []app.SwitchRequest{}
	client := populatedFakeClient()
	client.switchCalls = &calls
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = updated.(Model)
	if model.screen != ScreenSwitch || !strings.Contains(model.View(), "Switch profile") {
		t.Fatalf("switch view = %s", model.View())
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	if !model.switchBusy || cmd == nil {
		t.Fatalf("dry-run not started: %+v cmd nil=%v", model, cmd == nil)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.switchDryRunErr != nil || model.switchDryRunFingerprint == "" || !strings.Contains(model.View(), "Ready to execute") {
		t.Fatalf("dry-run model/view = %+v\n%s", model, model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	if model.screen != ScreenConfirm || !strings.Contains(model.View(), "SWITCH work azure") {
		t.Fatalf("confirm view = %s", model.View())
	}
	for _, r := range "SWITCH work azure" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !model.switchBusy || cmd == nil {
		t.Fatalf("execute not started: %+v cmd nil=%v", model, cmd == nil)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.switchExecErr != nil || !strings.Contains(model.View(), "Switch complete") {
		t.Fatalf("execute model/view = %+v\n%s", model, model.View())
	}
	if len(calls) != 3 || !calls[0].DryRun || !calls[1].DryRun || calls[2].DryRun || !calls[2].Yes {
		t.Fatalf("switch calls = %+v", calls)
	}
}

func TestSwitchDryRunBlockerDisablesExecute(t *testing.T) {
	client := populatedFakeClient()
	client.switchResults = []fakeSwitchResult{{result: switchResult("work", []string{"azure"}, true, 1), err: errors.New("unsafe target overwrite blocked")}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.switchDryRunErr == nil || model.canExecuteSwitch() || !strings.Contains(model.View(), "Blocker:") {
		t.Fatalf("blocked dry-run model/view = %+v\n%s", model, model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	if model.screen != ScreenSwitch || model.switchExecErr == nil {
		t.Fatalf("execute should remain blocked: %+v", model)
	}
}

func TestSwitchWrongConfirmationBlocksExecution(t *testing.T) {
	calls := []app.SwitchRequest{}
	client := populatedFakeClient()
	client.switchCalls = &calls
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	for _, r := range "WRONG" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil || model.confirmErr == "" || len(calls) != 1 {
		t.Fatalf("wrong confirm model/calls = %+v %+v", model, calls)
	}
}

func TestSwitchConfirmIgnoresDuplicateEnterWhileBusy(t *testing.T) {
	client := populatedFakeClient()
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	for _, r := range "SWITCH work azure" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil || !model.switchBusy {
		t.Fatalf("first execute not started: %+v", model)
	}
	_, duplicate := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if duplicate != nil {
		t.Fatal("duplicate enter returned command")
	}
}

func TestSwitchDryRunDriftAbortsExecution(t *testing.T) {
	calls := []app.SwitchRequest{}
	client := populatedFakeClient()
	client.switchCalls = &calls
	client.switchResults = []fakeSwitchResult{
		{result: switchResult("work", []string{"azure"}, true, 1)},
		{result: switchResult("work", []string{"azure"}, true, 2)},
	}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	for _, r := range "SWITCH work azure" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.switchExecErr == nil || !strings.Contains(model.switchExecErr.Error(), "fingerprint changed") || len(calls) != 2 {
		t.Fatalf("drift model/calls = %+v %+v", model, calls)
	}
}

func TestSyncDryRunRendersDeletionsAndSkipped(t *testing.T) {
	client := populatedFakeClient()
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	if !model.syncBusy || cmd == nil {
		t.Fatalf("dry-run not started: %+v cmd nil=%v", model, cmd == nil)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	view := model.View()
	for _, want := range []string{"Sync conflicts", "Would delete:", "1", "settings conflicted copy.json", "case conflict notes.md", "conflict-copy name needs manual review"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestSyncExecuteDisabledBeforeDryRun(t *testing.T) {
	calls := []app.SyncRequest{}
	client := populatedFakeClient()
	client.syncCalls = &calls
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	if cmd != nil || len(calls) != 0 || model.syncExecErr == nil || !strings.Contains(model.View(), "successful dry-run required") {
		t.Fatalf("execute should be blocked: %+v calls=%+v cmd nil=%v", model, calls, cmd == nil)
	}
}

func TestSyncWrongConfirmationBlocksExecution(t *testing.T) {
	calls := []app.SyncRequest{}
	client := populatedFakeClient()
	client.syncCalls = &calls
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	for _, r := range "WRONG" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd != nil || model.syncConfirmErr == "" || len(calls) != 1 {
		t.Fatalf("wrong confirm model/calls = %+v %+v", model, calls)
	}
}

func TestSyncDryRunAndConfirmExecute(t *testing.T) {
	calls := []app.SyncRequest{}
	client := populatedFakeClient()
	client.syncCalls = &calls
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	for _, r := range "DELETE 1 CONFLICTS" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !model.syncBusy || cmd == nil {
		t.Fatalf("execute not started: %+v cmd nil=%v", model, cmd == nil)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.syncExecErr != nil || !strings.Contains(model.View(), "Deleted:") || !strings.Contains(model.View(), "Sync cleanup complete") {
		t.Fatalf("execute model/view = %+v\n%s", model, model.View())
	}
	if len(calls) != 3 || !calls[0].DryRun || !calls[1].DryRun || calls[2].DryRun || !calls[2].Yes || calls[2].ExpectedConflictFingerprint == "" {
		t.Fatalf("sync calls = %+v", calls)
	}
}

func TestSyncConflictDriftAbortsExecution(t *testing.T) {
	calls := []app.SyncRequest{}
	client := populatedFakeClient()
	client.syncCalls = &calls
	client.syncResults = []fakeSyncResult{
		{result: syncResult(1, 0, "fp-1", true)},
		{result: syncResult(2, 0, "fp-2", true)},
	}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	for _, r := range "DELETE 1 CONFLICTS" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.syncExecErr == nil || !strings.Contains(model.syncExecErr.Error(), "conflict list changed") || len(calls) != 2 || !strings.Contains(model.View(), "Would delete:") || !strings.Contains(model.View(), "2") {
		t.Fatalf("drift model/calls = %+v %+v\n%s", model, calls, model.View())
	}
}

func TestSyncConfirmIgnoresDuplicateEnterWhileBusy(t *testing.T) {
	client := populatedFakeClient()
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	for _, r := range "DELETE 1 CONFLICTS" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil || !model.syncBusy {
		t.Fatalf("first execute not started: %+v", model)
	}
	_, duplicate := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if duplicate != nil {
		t.Fatal("duplicate enter returned command")
	}
}

func TestSyncLockErrorRenders(t *testing.T) {
	calls := []app.SyncRequest{}
	client := populatedFakeClient()
	client.syncCalls = &calls
	client.syncResults = []fakeSyncResult{
		{result: syncResult(1, 0, "fp-1", true)},
		{result: syncResult(1, 0, "fp-1", true)},
		{result: app.SyncResult{}, err: errors.New("acquire store operation lock /tmp/loki/.loki-operation.lock: timed out waiting for sync")},
	}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	for _, r := range "DELETE 1 CONFLICTS" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.syncExecErr == nil || !strings.Contains(model.View(), "operation lock") {
		t.Fatalf("lock model/view = %+v\n%s", model, model.View())
	}
}

func TestSyncDryRunErrorRendersWithoutCrash(t *testing.T) {
	client := populatedFakeClient()
	client.syncResults = []fakeSyncResult{{result: app.SyncResult{}, err: errors.New("scan failed")}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.syncDryRunErr == nil || !strings.Contains(model.View(), "scan failed") {
		t.Fatalf("dry-run error model/view = %+v\n%s", model, model.View())
	}
}

func TestDashboardOpensSnapshots(t *testing.T) {
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	model := loadedModel(client)
	view := model.View()
	if !strings.Contains(view, "Snapshots") || !strings.Contains(view, "1 retained") {
		t.Fatalf("dashboard view missing snapshots action:\n%s", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	if model.screen != ScreenSnapshots || !strings.Contains(model.View(), "snap-1") {
		t.Fatalf("snapshots view = %s\n%s", model.screen, model.View())
	}
}

func TestSnapshotsShowRendersMetadataOnly(t *testing.T) {
	showCalls := []app.SnapshotShowRequest{}
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	client.snapshotShowCalls = &showCalls
	client.snapshotShowResults = []fakeSnapshotShowResult{{result: snapshotShowResult("snap-1", "/tmp/loki/target.txt")}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !model.snapshotBusy || cmd == nil {
		t.Fatalf("show not started: %+v cmd nil=%v", model, cmd == nil)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	view := model.View()
	for _, want := range []string{"Snapshot metadata", "snap-1", "target.txt", "hash=abcdef123456"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "dummy file content") || len(showCalls) != 1 || showCalls[0].SnapshotID != "snap-1" {
		t.Fatalf("show view/calls unsafe:\n%s\n%+v", view, showCalls)
	}
}

func TestSnapshotsShowRedactsSensitivePaths(t *testing.T) {
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	sensitive := "/Users/me/.ssh/id_rsa"
	client.snapshotShowResults = []fakeSnapshotShowResult{{result: snapshotShowResult("snap-1", sensitive)}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	view := model.View()
	if strings.Contains(view, sensitive) || !strings.Contains(view, "(redacted-sensitive-path)") {
		t.Fatalf("sensitive path not redacted:\n%s", view)
	}
}

func TestSnapshotRestoreDryRunFullShowsGuardedCommand(t *testing.T) {
	restoreCalls := []app.SnapshotRestoreDryRunRequest{}
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	client.snapshotRestoreCalls = &restoreCalls
	client.snapshotRestoreResults = []fakeSnapshotRestoreResult{{result: snapshotRestoreDryRunResult("snap-1", "", false, true)}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	if !model.snapshotBusy || cmd == nil {
		t.Fatalf("dry-run not started: %+v cmd nil=%v", model, cmd == nil)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	view := model.View()
	for _, want := range []string{"Mode: dry-run only", "Run:", "loki snapshots restore snap-1 --yes", "RESTORE snap-1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if len(restoreCalls) != 1 || !restoreCalls[0].DryRun || restoreCalls[0].Target != "" {
		t.Fatalf("restore calls = %+v", restoreCalls)
	}
}

func TestSnapshotRestoreDryRunTargetShowsGuardedCommand(t *testing.T) {
	restoreCalls := []app.SnapshotRestoreDryRunRequest{}
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	client.snapshotRestoreCalls = &restoreCalls
	client.snapshotShowResults = []fakeSnapshotShowResult{{result: snapshotShowResult("snap-1", "/tmp/loki/target.txt")}}
	client.snapshotRestoreResults = []fakeSnapshotRestoreResult{{result: snapshotRestoreDryRunResult("snap-1", "/tmp/loki/target.txt", false, true)}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	view := model.View()
	if !strings.Contains(view, "loki snapshots restore snap-1 --target /tmp/loki/target.txt --yes") || len(restoreCalls) != 1 || restoreCalls[0].Target != "/tmp/loki/target.txt" {
		t.Fatalf("target dry-run view/calls = %+v\n%s", restoreCalls, view)
	}
}

func TestSnapshotRestoreDryRunQuotesShellSensitiveTarget(t *testing.T) {
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	target := "/tmp/$(touch pwn) target.txt"
	client.snapshotShowResults = []fakeSnapshotShowResult{{result: snapshotShowResult("snap-1", target)}}
	client.snapshotRestoreResults = []fakeSnapshotRestoreResult{{result: snapshotRestoreDryRunResult("snap-1", target, false, true)}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	view := model.View()
	want := "--target '/tmp/$(touch pwn) target.txt' --yes"
	if !strings.Contains(view, want) || strings.Contains(view, "--target \"") {
		t.Fatalf("target command not shell-safe, want %q:\n%s", want, view)
	}
}

func TestSnapshotRestoreDryRunRendersBlockersWarnings(t *testing.T) {
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	result := snapshotRestoreDryRunResult("snap-1", "", false, false)
	result.Blockers = []string{"target changed"}
	result.Warnings = []string{"restore needs review"}
	client.snapshotRestoreResults = []fakeSnapshotRestoreResult{{result: result}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	view := model.View()
	if !strings.Contains(view, "Guard: not recorded") || !strings.Contains(view, "Blocker: target changed") || !strings.Contains(view, "Warning: restore needs review") || strings.Contains(view, "Run: loki snapshots restore") {
		t.Fatalf("blocker/warning view =\n%s", view)
	}
}

func TestSnapshotRestoreDryRunRespectsRedactionFields(t *testing.T) {
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	sensitive := "/Users/me/.ssh/id_rsa"
	result := snapshotRestoreDryRunResult("snap-1", sensitive, true, true)
	client.snapshotRestoreResults = []fakeSnapshotRestoreResult{{result: result}}
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	view := model.View()
	if strings.Contains(view, sensitive) || !strings.Contains(view, "Target filter: (redacted-sensitive-path)") || !strings.Contains(view, "Command hidden because target path is redacted") {
		t.Fatalf("restore redaction view =\n%s", view)
	}
}

func TestSnapshotRestoreNoExecuteKey(t *testing.T) {
	restoreCalls := []app.SnapshotRestoreDryRunRequest{}
	client := populatedFakeClient()
	client.snapshots = snapshotListResult()
	client.snapshotRestoreCalls = &restoreCalls
	model := loadedModel(client)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	if cmd != nil || len(restoreCalls) != 1 {
		t.Fatalf("x should not execute restore: calls=%+v cmd nil=%v", restoreCalls, cmd == nil)
	}
}

func loadedModel(client fakeClient) Model {
	model := NewModel(context.Background(), client)
	updated, _ := model.Update(dashboardLoadedMsg{status: client.status, catalog: client.catalog, doctor: client.doctor, machine: client.machine, secrets: client.secrets, snapshots: client.snapshots, catalogErr: client.catalogErr, doctorErr: client.doctorErr, machineErr: client.machineErr, secretsErr: client.secretsErr, snapshotsErr: client.snapshotsErr})
	return updated.(Model)
}

func switchResult(profile string, buckets []string, dryRun bool, operations int) app.SwitchResult {
	ops := make([]activation.Operation, 0, operations)
	for i := 0; i < operations; i++ {
		ops = append(ops, activation.Operation{
			ID:         fmt.Sprintf("op-%d", i+1),
			Type:       activation.OperationCopy,
			TargetPath: fmt.Sprintf("/tmp/target-%d", i+1),
			SourcePath: fmt.Sprintf("/tmp/source-%d", i+1),
			Safety:     activation.SafetyStatus{Class: activation.SafetyMissing, Safe: true, Message: "target is missing"},
		})
	}
	return app.SwitchResult{Plan: activation.Plan{StorePath: "/tmp/loki", Profile: profile, Buckets: buckets, Operations: ops}, DryRun: dryRun, Changed: operations}
}

func syncResult(deleteCount, skippedCount int, fingerprint string, dryRun bool) app.SyncResult {
	conflicts := make([]storesync.ConflictCopy, 0, deleteCount+skippedCount)
	for i := 0; i < deleteCount; i++ {
		name := "settings conflicted copy.json"
		if i > 0 {
			name = fmt.Sprintf("settings %d conflicted copy.json", i+1)
		}
		conflicts = append(conflicts, storesync.ConflictCopy{
			Path:         "/tmp/loki/profiles/work/core/files/" + name,
			RelativePath: "profiles/work/core/files/" + name,
			Name:         name,
			Kind:         "file",
			Action:       storesync.ConflictActionDelete,
		})
	}
	for i := 0; i < skippedCount; i++ {
		name := "case conflict notes.md"
		if i > 0 {
			name = fmt.Sprintf("case conflict notes %d.md", i+1)
		}
		conflicts = append(conflicts, storesync.ConflictCopy{
			Path:         "/tmp/loki/profiles/work/core/files/" + name,
			RelativePath: "profiles/work/core/files/" + name,
			Name:         name,
			Kind:         "file",
			Action:       storesync.ConflictActionSkip,
			Reason:       "conflict-copy name needs manual review",
		})
	}
	result := app.SyncResult{StorePath: "/tmp/loki", DryRun: dryRun, WouldDeleteCount: deleteCount, SkippedCount: skippedCount, Conflicts: conflicts, ConflictFingerprint: fingerprint}
	if !dryRun {
		result.DeletedCount = deleteCount
		result.HeartbeatUpdated = deleteCount > 0
	}
	return result
}

func snapshotListResult() app.SnapshotListResult {
	return app.SnapshotListResult{
		SnapshotDir: "/tmp/state/snapshots",
		Snapshots: []activation.SnapshotSummary{{
			SnapshotID:            "snap-1",
			MachineID:             "machine-1",
			CreatedAt:             "2026-05-05T00:00:00Z",
			PreviousActiveProfile: "work",
			PreviousActiveBuckets: []string{"azure"},
			TargetCount:           1,
			TargetKinds:           []string{"file"},
			Exists:                true,
		}},
	}
}

func snapshotShowResult(id string, targetPath string) app.SnapshotShowResult {
	return app.SnapshotShowResult{
		SnapshotDir: "/tmp/state/snapshots",
		Snapshot: activation.Snapshot{
			SnapshotID:            id,
			MachineID:             "machine-1",
			Path:                  "/tmp/state/snapshots/" + id,
			CreatedAt:             "2026-05-05T00:00:00Z",
			Reason:                "switch",
			PreviousActiveProfile: "work",
			PreviousActiveBuckets: []string{"azure"},
			Targets: []activation.SnapshotEntry{{
				TargetPath:   targetPath,
				Kind:         "file",
				Hash:         "abcdef1234567890",
				ExpectedHash: "123456abcdef7890",
				ExpectedMode: "0644",
			}},
			ManagedTargets: []activation.ManagedTargetSnapshot{{TargetPath: targetPath, Found: true}},
		},
	}
}

func snapshotRestoreDryRunResult(id, target string, redacted bool, guardRecorded bool) app.SnapshotRestoreDryRunResult {
	path := firstNonEmpty(target, "/tmp/loki/target.txt")
	result := app.SnapshotRestoreDryRunResult{
		SnapshotDir:          "/tmp/state/snapshots",
		SnapshotID:           id,
		DryRun:               true,
		GuardRecorded:        guardRecorded,
		GuardExpiresAt:       "2026-05-05T00:15:00Z",
		TargetFilter:         target,
		TargetFilterRedacted: redacted,
		Summary: app.SnapshotRestoreDryRunSummary{
			TargetCount:                   1,
			RestoreFileCount:              1,
			PreviousActiveProfile:         "work",
			PreviousActiveBuckets:         []string{"azure"},
			WouldRestoreManagedTargetRows: 1,
			WouldRestoreActiveState:       target == "",
		},
		Targets: []app.SnapshotRestoreDryRunTarget{{
			TargetPath:         path,
			TargetPathRedacted: redacted,
			Kind:               "file",
			Action:             activation.RestoreActionRestoreFile,
			CurrentExists:      true,
			CurrentKind:        "file",
			CurrentMode:        "0644",
			CurrentHashPrefix:  "abcdef123456",
			SnapshotHashPrefix: "123456abcdef",
			ExpectedHashPrefix: "7890abcdef12",
			ExpectedMode:       "0644",
			LinkTargetRedacted: redacted,
			SensitivePath:      redacted,
		}},
	}
	if redacted {
		result.TargetFilter = ""
		result.Targets[0].TargetPath = ""
	}
	return result
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
