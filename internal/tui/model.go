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

type Model struct {
	ctx     context.Context
	client  Client
	screen  ScreenID
	back    []ScreenID
	width   int
	height  int
	loading bool
	err     error

	status    app.StatusResult
	catalog   app.ProfileCatalogResult
	machine   app.MachineStatusResult
	secrets   app.SecretsStatusResult
	snapshots app.SnapshotListResult

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
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.loading = true
			m.err = nil
			m.screen = ScreenLoading
			return m, tea.Batch(m.spinner.Tick, loadDashboardCmd(m.ctx, m.client))
		}
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
		if msg.err != nil {
			m.err = msg.err
			m.screen = ScreenError
			return m, nil
		}
		m.err = nil
		m.screen = ScreenDashboard
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	switch m.screen {
	case ScreenLoading:
		return m.loadingView()
	case ScreenDashboard:
		return m.dashboardView()
	case ScreenError:
		return m.errorView()
	default:
		return m.dashboardView()
	}
}
