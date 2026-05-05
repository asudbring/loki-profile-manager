package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allensu/loki-profile-manager/internal/app"
)

type dashboardLoadedMsg struct {
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
	err          error
}

func loadDashboardCmd(ctx context.Context, client Client) tea.Cmd {
	return func() tea.Msg {
		status, err := client.Status(ctx)
		if err != nil {
			return dashboardLoadedMsg{err: err}
		}
		doctor, doctorErr := client.Doctor(ctx)
		secrets, secretsErr := client.SecretsStatus(ctx)
		snapshots, snapshotsErr := client.ListSnapshots(ctx)

		var catalog app.ProfileCatalogResult
		var catalogErr error
		var machine app.MachineStatusResult
		var machineErr error
		if status.Configured {
			catalog, catalogErr = client.ProfileCatalog(ctx)
			machine, machineErr = client.MachineStatus(ctx)
		}
		return dashboardLoadedMsg{
			status:       status,
			catalog:      catalog,
			doctor:       doctor,
			machine:      machine,
			secrets:      secrets,
			snapshots:    snapshots,
			catalogErr:   catalogErr,
			doctorErr:    doctorErr,
			machineErr:   machineErr,
			secretsErr:   secretsErr,
			snapshotsErr: snapshotsErr,
		}
	}
}
