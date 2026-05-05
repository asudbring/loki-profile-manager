package tui

import (
	"fmt"
	"strings"

	"github.com/allensu/loki-profile-manager/internal/app"
)

func (m Model) loadingView() string {
	return frame(
		titleStyle.Render("Loki TUI"),
		"",
		fmt.Sprintf("%s Loading dashboard...", m.spinner.View()),
		"",
		helpStyle.Render("q quit"),
	)
}

func (m Model) dashboardView() string {
	status := "not configured"
	if m.status.Configured {
		status = "configured"
	}
	lines := []string{
		titleStyle.Render("Loki Profile Manager"),
		"",
		labelValue("Status", status),
		labelValue("Store", firstNonEmpty(m.status.StorePath, "not configured")),
		labelValue("Local state", firstNonEmpty(m.status.LocalStatePath, "unknown")),
		labelValue("Active profile", formatActiveProfile(m.status.ActiveProfile, m.status.ActiveBuckets)),
		labelValue("Managed targets", fmt.Sprintf("%d", m.status.ManagedTargetCount)),
		labelValue("Machine", formatSectionStatus(m.machineErr, formatMachineFromStatus(m.status, m.machine))),
		labelValue("Doctor", formatSectionStatus(m.doctorErr, formatDoctorSummary(m.doctor))),
		labelValue("Secrets", formatSectionStatus(m.secretsErr, formatSecretsReady(m.secrets))),
	}
	if m.status.Configured {
		lines = append(lines,
			labelValue("Profiles", formatSectionStatus(m.catalogErr, formatCatalogSummary(m.catalog))),
			labelValue("Snapshots", formatSectionStatus(m.snapshotsErr, fmt.Sprintf("%d", len(m.snapshots.Snapshots)))),
		)
	} else {
		lines = append(lines, "", mutedStyle.Render("Dashboard placeholder: configure a store, then refresh."))
	}
	lines = append(lines, "", subtitleStyle.Render("Quick actions"))
	items := m.dashboardItems()
	for i, item := range items {
		lines = append(lines, menuLine(i == m.selected, item))
	}
	lines = append(lines, "", helpStyle.Render("↑/↓ select • enter open • d/m/s/p quick open • esc back • r refresh • q quit"))
	return frame(lines...)
}

func (m Model) errorView() string {
	message := "unknown error"
	if m.err != nil {
		message = m.err.Error()
	}
	return frame(
		titleStyle.Render("Loki TUI"),
		"",
		errorStyle.Render("Error: "+message),
		"",
		helpStyle.Render("r retry • q quit"),
	)
}

func menuLine(selected bool, item dashboardItem) string {
	prefix := "  "
	style := menuStyle
	if selected {
		prefix = "> "
		style = selectedMenuStyle
	}
	line := fmt.Sprintf("%s[%s] %-8s %s", prefix, item.Key, item.Label, item.Description)
	return style.Render(line)
}

func formatActiveProfile(profile string, buckets []string) string {
	if strings.TrimSpace(profile) == "" {
		return "not set"
	}
	if len(buckets) == 0 {
		return profile
	}
	return fmt.Sprintf("%s (%s)", profile, strings.Join(buckets, ", "))
}

func formatCatalogSummary(catalog app.ProfileCatalogResult) string {
	profiles := profileCount(catalog)
	buckets := bucketCount(catalog)
	if profiles == 0 {
		return "none"
	}
	return fmt.Sprintf("%d profiles, %d buckets", profiles, buckets)
}
