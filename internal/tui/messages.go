package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allensu/loki-profile-manager/internal/app"
)

type dashboardLoadedMsg struct {
	status  app.StatusResult
	catalog app.ProfileCatalogResult
	err     error
}

func loadDashboardCmd(ctx context.Context, client Client) tea.Cmd {
	return func() tea.Msg {
		status, err := client.Status(ctx)
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		var catalog app.ProfileCatalogResult
		if status.Configured {
			catalog, err = client.ProfileCatalog(ctx)
			if err != nil {
				return dashboardLoadedMsg{status: status, err: err}
			}
		}
		return dashboardLoadedMsg{status: status, catalog: catalog}
	}
}
