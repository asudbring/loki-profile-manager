package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/machine"
)

type ScreenID string

const (
	ScreenLoading   ScreenID = "loading"
	ScreenDashboard ScreenID = "dashboard"
	ScreenStore     ScreenID = "store"
	ScreenDoctor    ScreenID = "doctor"
	ScreenMachine   ScreenID = "machine"
	ScreenSecrets   ScreenID = "secrets"
	ScreenProfiles  ScreenID = "profiles"
	ScreenSwitch    ScreenID = "switch"
	ScreenSync      ScreenID = "sync"
	ScreenSnapshots ScreenID = "snapshots"
	ScreenConfirm   ScreenID = "confirm"
	ScreenError     ScreenID = "error"
)

type dashboardItem struct {
	Screen      ScreenID
	Key         string
	Label       string
	Description string
}

type Model struct {
	ctx     context.Context
	client  Client
	screen  ScreenID
	back    []ScreenID
	width   int
	height  int
	loading bool
	err     error

	selected int

	status          app.StatusResult
	storeStatus     app.StoreStatusResult
	storeCandidates app.DiscoverStoresResult
	catalog         app.ProfileCatalogResult
	doctor          app.DoctorResult
	machine         app.MachineStatusResult
	secrets         app.SecretsStatusResult
	snapshots       app.SnapshotListResult

	storeStatusErr   error
	storeDiscoverErr error
	catalogErr       error
	doctorErr        error
	machineErr       error
	secretsErr       error
	snapshotsErr     error

	storeCandidateIndex int
	storeManualMode     bool
	storeManualInput    string
	storeConfirmAction  string
	storeConfirmPath    string
	storeConfirmInput   string
	storeConfirmErr     string
	storeActionErr      error
	storeMessage        string
	storeBusy           bool

	switchInitialized       bool
	switchProfileIndex      int
	switchBucketIndex       int
	switchSelectedBuckets   map[string]bool
	switchDryRun            app.SwitchResult
	switchDryRunErr         error
	switchDryRunFingerprint string
	switchExecResult        app.SwitchResult
	switchExecErr           error
	switchBusy              bool
	confirmInput            string
	confirmErr              string

	syncDryRun            app.SyncResult
	syncDryRunErr         error
	syncDryRunFingerprint string
	syncExecResult        app.SyncResult
	syncExecErr           error
	syncBusy              bool
	syncConfirmInput      string
	syncConfirmErr        string

	snapshotIndex               int
	snapshotTargetIndex         int
	snapshotShow                app.SnapshotShowResult
	snapshotShowErr             error
	snapshotRestoreDryRun       app.SnapshotRestoreDryRunResult
	snapshotRestoreDryRunErr    error
	snapshotRestoreDryRunTarget string
	snapshotBusy                bool

	machineEdit           bool
	machineConfirm        bool
	machineFieldIndex     int
	machineInputs         []string
	machineConfirmInput   string
	machineConfirmErr     string
	machineRegisterErr    error
	machineRegisterRecord machine.Record
	machineBusy           bool

	secretsConfigure        bool
	secretsConfigureField   int
	secretsConfigureInputs  []string
	secretsConfigureErr     error
	secretsConfigureMessage string
	secretsConfigureBusy    bool

	spinner spinner.Model
}

func NewModel(ctx context.Context, client Client) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	return Model{
		ctx:     ctx,
		client:  client,
		screen:  ScreenLoading,
		loading: true,
		spinner: spin,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, loadDashboardCmd(m.ctx, m.client))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	case dashboardLoadedMsg:
		m.loading = false
		m.status = msg.status
		m.storeStatus = msg.storeStatus
		m.storeCandidates = msg.storeCandidates
		m.catalog = msg.catalog
		m.doctor = msg.doctor
		m.machine = msg.machine
		m.secrets = msg.secrets
		m.snapshots = msg.snapshots
		m.storeStatusErr = msg.storeStatusErr
		m.storeDiscoverErr = msg.storeDiscoverErr
		m.catalogErr = msg.catalogErr
		m.doctorErr = msg.doctorErr
		m.machineErr = msg.machineErr
		m.secretsErr = msg.secretsErr
		m.snapshotsErr = msg.snapshotsErr
		if msg.err != nil {
			m.err = msg.err
			m.screen = ScreenError
			return m, nil
		}
		m.err = nil
		m.screen = ScreenDashboard
		m.ensureSwitchSelection()
		m.ensureStoreSelection()
		return m, nil
	case switchDryRunMsg:
		m.switchBusy = false
		m.switchDryRun = msg.result
		m.switchDryRunErr = msg.err
		m.switchDryRunFingerprint = msg.fingerprint
		m.switchExecResult = app.SwitchResult{}
		m.switchExecErr = nil
		m.screen = ScreenSwitch
		return m, nil
	case switchExecuteMsg:
		m.switchBusy = false
		m.switchExecResult = msg.result
		m.switchExecErr = msg.err
		m.confirmInput = ""
		m.confirmErr = ""
		m.screen = ScreenSwitch
		return m, nil
	case syncDryRunMsg:
		m.syncBusy = false
		m.syncDryRun = msg.result
		m.syncDryRunErr = msg.err
		m.syncDryRunFingerprint = msg.fingerprint
		m.syncExecResult = app.SyncResult{}
		m.syncExecErr = nil
		m.syncConfirmInput = ""
		m.syncConfirmErr = ""
		m.screen = ScreenSync
		return m, nil
	case syncExecuteMsg:
		m.syncBusy = false
		m.syncExecResult = msg.result
		m.syncExecErr = msg.err
		m.syncConfirmInput = ""
		m.syncConfirmErr = ""
		if msg.err != nil && msg.fingerprint != "" {
			m.syncDryRun = msg.result
			m.syncDryRunFingerprint = msg.fingerprint
		}
		m.screen = ScreenSync
		return m, nil
	case snapshotShowMsg:
		m.snapshotBusy = false
		m.snapshotShow = msg.result
		m.snapshotShowErr = msg.err
		m.snapshotRestoreDryRun = app.SnapshotRestoreDryRunResult{}
		m.snapshotRestoreDryRunErr = nil
		m.snapshotRestoreDryRunTarget = ""
		m.ensureSnapshotTargetSelection()
		m.screen = ScreenSnapshots
		return m, nil
	case snapshotRestoreDryRunMsg:
		m.snapshotBusy = false
		m.snapshotRestoreDryRun = msg.result
		m.snapshotRestoreDryRunErr = msg.err
		m.snapshotRestoreDryRunTarget = msg.target
		m.screen = ScreenSnapshots
		return m, nil
	case storeDiscoverMsg:
		m.storeBusy = false
		m.storeCandidates = msg.result
		m.storeDiscoverErr = msg.err
		m.storeManualMode = false
		m.ensureStoreSelection()
		m.screen = ScreenStore
		return m, nil
	case storeActionMsg:
		m.storeBusy = false
		m.storeActionErr = msg.err
		m.storeConfirmAction = ""
		m.storeConfirmPath = ""
		m.storeConfirmInput = ""
		m.storeConfirmErr = ""
		if msg.err == nil {
			m.storeMessage = msg.message
			m.storeStatus = msg.status
			m.storeCandidates = msg.candidates
			m.status.Configured = msg.status.Valid
			m.status.StorePath = msg.status.EffectiveStorePath
		}
		m.screen = ScreenStore
		return m, nil
	case machineRegisterMsg:
		m.machineBusy = false
		m.machineRegisterRecord = msg.record
		m.machineRegisterErr = msg.err
		m.machineConfirm = false
		m.machineConfirmInput = ""
		m.machineConfirmErr = ""
		if msg.err == nil {
			m.machineEdit = false
			m.machine.Registered = true
			m.machine.Record = &m.machineRegisterRecord
			m.machine.MachineID = msg.record.MachineID
			m.status.MachineRegistered = true
			m.status.MachineID = msg.record.MachineID
			m.status.MachineDisplayName = msg.record.DisplayName
			m.status.MachineAllowedParentProfiles = cloneStringSlice(msg.record.AllowedParentProfiles)
			m.status.MachineAllowedBuckets = cloneStringSlice(msg.record.AllowedBuckets)
		}
		m.screen = ScreenMachine
		return m, nil
	case secretsConfigureMsg:
		m.secretsConfigureBusy = false
		m.secretsConfigureErr = msg.err
		m.scrubSecretsConfigureInputs()
		if msg.err == nil {
			m.secretsConfigure = false
			m.secretsConfigureMessage = "Infisical configuration saved. Run secrets status to verify readiness."
			if msg.result.Verified {
				m.secrets = msg.result.Status
				m.secretsErr = nil
			}
		}
		m.screen = ScreenSecrets
		return m, nil
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.screen == ScreenConfirm {
		return m.updateConfirmKey(msg)
	}
	if m.screen == ScreenStore {
		return m.updateStoreKey(msg)
	}
	if m.screen == ScreenMachine {
		return m.updateMachineKey(msg)
	}
	if m.screen == ScreenSwitch {
		return m.updateSwitchKey(msg)
	}
	if m.screen == ScreenSync {
		return m.updateSyncKey(msg)
	}
	if m.screen == ScreenSnapshots {
		return m.updateSnapshotsKey(msg)
	}
	if m.screen == ScreenSecrets {
		return m.updateSecretsKey(msg)
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.loading = true
		m.err = nil
		m.screen = ScreenLoading
		return m, tea.Batch(m.spinner.Tick, loadDashboardCmd(m.ctx, m.client))
	case "esc", "backspace":
		if m.screen != ScreenDashboard && m.screen != ScreenLoading {
			m.screen = ScreenDashboard
			m.err = nil
		}
		return m, nil
	case "up", "k":
		if m.screen == ScreenDashboard {
			m.moveSelection(-1)
		}
		return m, nil
	case "down", "j":
		if m.screen == ScreenDashboard {
			m.moveSelection(1)
		}
		return m, nil
	case "enter":
		if m.screen == ScreenDashboard {
			m.openSelected()
		}
		return m, nil
	case "g":
		m.screen = ScreenStore
		m.ensureStoreSelection()
		return m, nil
	case "d":
		m.screen = ScreenDoctor
		return m, nil
	case "m":
		m.screen = ScreenMachine
		return m, nil
	case "s":
		m.screen = ScreenSecrets
		return m, nil
	case "p":
		m.screen = ScreenProfiles
		return m, nil
	case "w":
		m.screen = ScreenSwitch
		m.ensureSwitchSelection()
		return m, nil
	case "y":
		m.screen = ScreenSync
		return m, nil
	case "n":
		m.screen = ScreenSnapshots
		m.ensureSnapshotSelection()
		return m, nil
	}
	return m, nil
}

func (m *Model) moveSelection(delta int) {
	items := m.dashboardItems()
	if len(items) == 0 {
		m.selected = 0
		return
	}
	m.selected = (m.selected + delta + len(items)) % len(items)
}

func (m *Model) openSelected() {
	items := m.dashboardItems()
	if len(items) == 0 {
		return
	}
	if m.selected < 0 || m.selected >= len(items) {
		m.selected = 0
	}
	m.screen = items[m.selected].Screen
	if m.screen == ScreenSwitch {
		m.ensureSwitchSelection()
	}
	if m.screen == ScreenSnapshots {
		m.ensureSnapshotSelection()
	}
}

func (m Model) dashboardItems() []dashboardItem {
	storeItem := dashboardItem{Screen: ScreenStore, Key: "g", Label: "Store", Description: formatSectionStatus(firstError(m.storeStatusErr, m.storeDiscoverErr), formatStoreSummary(m.storeStatus))}
	items := []dashboardItem{
		{Screen: ScreenSwitch, Key: "w", Label: "Switch", Description: "dry-run profile activation"},
		{Screen: ScreenSync, Key: "y", Label: "Sync", Description: "dry-run provider conflict cleanup"},
		{Screen: ScreenSnapshots, Key: "n", Label: "Snapshots", Description: formatSectionStatus(m.snapshotsErr, fmt.Sprintf("%d retained", len(m.snapshots.Snapshots)))},
		{Screen: ScreenDoctor, Key: "d", Label: "Doctor", Description: formatSectionStatus(m.doctorErr, formatDoctorSummary(m.doctor))},
		{Screen: ScreenMachine, Key: "m", Label: "Machine", Description: formatSectionStatus(m.machineErr, formatMachineFromStatus(m.status, m.machine))},
		{Screen: ScreenSecrets, Key: "s", Label: "Secrets", Description: formatSectionStatus(m.secretsErr, formatSecretsReady(m.secrets))},
		{Screen: ScreenProfiles, Key: "p", Label: "Profiles", Description: formatSectionStatus(m.catalogErr, formatCatalogSummary(m.catalog))},
	}
	if !m.status.Configured {
		return append([]dashboardItem{storeItem}, items...)
	}
	return append(items, storeItem)
}

func (m Model) View() string {
	switch m.screen {
	case ScreenLoading:
		return m.loadingView()
	case ScreenDashboard:
		return m.dashboardView()
	case ScreenStore:
		return m.storeView()
	case ScreenDoctor:
		return m.doctorView()
	case ScreenMachine:
		return m.machineView()
	case ScreenSecrets:
		return m.secretsView()
	case ScreenProfiles:
		return m.profilesView()
	case ScreenSwitch:
		return m.switchView()
	case ScreenSync:
		return m.syncView()
	case ScreenSnapshots:
		return m.snapshotsView()
	case ScreenConfirm:
		return m.confirmView()
	case ScreenError:
		return m.errorView()
	default:
		return m.dashboardView()
	}
}
