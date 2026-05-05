package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/allensu/loki-profile-manager/internal/app"
)

type ScreenID string

const (
	ScreenLoading   ScreenID = "loading"
	ScreenDashboard ScreenID = "dashboard"
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

	status    app.StatusResult
	catalog   app.ProfileCatalogResult
	doctor    app.DoctorResult
	machine   app.MachineStatusResult
	secrets   app.SecretsStatusResult
	snapshots app.SnapshotListResult

	catalogErr   error
	doctorErr    error
	machineErr   error
	secretsErr   error
	snapshotsErr error

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
		m.catalog = msg.catalog
		m.doctor = msg.doctor
		m.machine = msg.machine
		m.secrets = msg.secrets
		m.snapshots = msg.snapshots
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
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.screen == ScreenConfirm {
		return m.updateConfirmKey(msg)
	}
	if m.screen == ScreenSwitch {
		return m.updateSwitchKey(msg)
	}
	if m.screen == ScreenSync {
		return m.updateSyncKey(msg)
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
}

func (m Model) dashboardItems() []dashboardItem {
	return []dashboardItem{
		{Screen: ScreenSwitch, Key: "w", Label: "Switch", Description: "dry-run profile activation"},
		{Screen: ScreenSync, Key: "y", Label: "Sync", Description: "dry-run provider conflict cleanup"},
		{Screen: ScreenDoctor, Key: "d", Label: "Doctor", Description: formatSectionStatus(m.doctorErr, formatDoctorSummary(m.doctor))},
		{Screen: ScreenMachine, Key: "m", Label: "Machine", Description: formatSectionStatus(m.machineErr, formatMachineFromStatus(m.status, m.machine))},
		{Screen: ScreenSecrets, Key: "s", Label: "Secrets", Description: formatSectionStatus(m.secretsErr, formatSecretsReady(m.secrets))},
		{Screen: ScreenProfiles, Key: "p", Label: "Profiles", Description: formatSectionStatus(m.catalogErr, formatCatalogSummary(m.catalog))},
	}
}

func (m Model) View() string {
	switch m.screen {
	case ScreenLoading:
		return m.loadingView()
	case ScreenDashboard:
		return m.dashboardView()
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
	case ScreenConfirm:
		return m.confirmView()
	case ScreenError:
		return m.errorView()
	default:
		return m.dashboardView()
	}
}
