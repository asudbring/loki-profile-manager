package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allensu/loki-profile-manager/internal/app"
	"github.com/allensu/loki-profile-manager/internal/storesync"
)

type syncDryRunMsg struct {
	result      app.SyncResult
	fingerprint string
	err         error
}

type syncExecuteMsg struct {
	result      app.SyncResult
	fingerprint string
	err         error
}

func (m Model) updateSyncKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.syncBusy {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = ScreenDashboard
		m.syncConfirmInput = ""
		m.syncConfirmErr = ""
		return m, nil
	case "r":
		m.loading = true
		m.err = nil
		m.screen = ScreenLoading
		return m, tea.Batch(m.spinner.Tick, loadDashboardCmd(m.ctx, m.client))
	case "d":
		return m.startSyncDryRun()
	case "x":
		if !m.canExecuteSync() {
			m.syncExecErr = fmt.Errorf("sync: successful dry-run required before execute")
			return m, nil
		}
		m.syncConfirmInput = ""
		m.syncConfirmErr = ""
		return m, nil
	case "enter":
		if !m.canExecuteSync() {
			m.syncExecErr = fmt.Errorf("sync: successful dry-run required before execute")
			return m, nil
		}
		phrase := m.syncConfirmPhrase()
		if m.syncConfirmInput != phrase {
			m.syncConfirmErr = fmt.Sprintf("confirmation mismatch; type %q", phrase)
			return m, nil
		}
		m.syncBusy = true
		m.syncConfirmErr = ""
		m.syncExecErr = nil
		return m, syncExecuteCmd(m.ctx, m.client, m.syncDryRunFingerprint)
	case "backspace", "ctrl+h":
		if len(m.syncConfirmInput) > 0 {
			m.syncConfirmInput = m.syncConfirmInput[:len(m.syncConfirmInput)-1]
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes && m.canExecuteSync() {
		m.syncConfirmInput += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

func (m Model) startSyncDryRun() (tea.Model, tea.Cmd) {
	m.syncBusy = true
	m.syncDryRunErr = nil
	m.syncExecErr = nil
	m.syncConfirmInput = ""
	m.syncConfirmErr = ""
	return m, syncDryRunCmd(m.ctx, m.client)
}

func (m Model) canExecuteSync() bool {
	return !m.syncBusy && m.syncDryRunErr == nil && m.syncDryRunFingerprint != "" && m.syncDryRun.WouldDeleteCount > 0
}

func (m Model) syncConfirmPhrase() string {
	return fmt.Sprintf("DELETE %d CONFLICTS", m.syncDryRun.WouldDeleteCount)
}

func syncDryRunCmd(ctx context.Context, client Client) tea.Cmd {
	return func() tea.Msg {
		result, err := client.Sync(ctx, app.SyncRequest{DryRun: true})
		return syncDryRunMsg{result: result, fingerprint: result.ConflictFingerprint, err: err}
	}
}

func syncExecuteCmd(ctx context.Context, client Client, expectedFingerprint string) tea.Cmd {
	return func() tea.Msg {
		dryRun, err := client.Sync(ctx, app.SyncRequest{DryRun: true})
		if err != nil {
			return syncExecuteMsg{result: dryRun, fingerprint: dryRun.ConflictFingerprint, err: fmt.Errorf("dry-run recheck failed: %w", err)}
		}
		if dryRun.ConflictFingerprint != expectedFingerprint {
			return syncExecuteMsg{result: dryRun, fingerprint: dryRun.ConflictFingerprint, err: fmt.Errorf("sync: conflict list changed; rerun dry-run before deleting")}
		}
		result, err := client.Sync(ctx, app.SyncRequest{Yes: true, ExpectedConflictFingerprint: expectedFingerprint})
		return syncExecuteMsg{result: result, fingerprint: result.ConflictFingerprint, err: err}
	}
}

func (m Model) syncView() string {
	lines := []string{
		titleStyle.Render("Sync conflicts"),
		"",
		labelValue("Store", firstNonEmpty(m.status.StorePath, "not configured")),
	}
	if !m.status.Configured {
		return frame(append(lines, "", mutedStyle.Render("Store not configured; sync unavailable."), "", helpStyle.Render("esc back • r refresh • q quit"))...)
	}
	if m.syncBusy {
		lines = append(lines, "", mutedStyle.Render("Sync operation running..."))
	}
	if !m.hasSyncDryRun() && !m.hasSyncExecution() && !m.syncBusy {
		lines = append(lines, "", mutedStyle.Render("Run dry-run to scan provider conflict-copy files."))
	}
	if m.hasSyncDryRun() {
		lines = appendSyncDryRun(lines, m)
	}
	if m.hasSyncExecution() {
		lines = appendSyncExecution(lines, m)
	}
	if m.canExecuteSync() {
		lines = append(lines,
			"",
			subtitleStyle.Render("Confirmation"),
			labelValue("Required phrase", m.syncConfirmPhrase()),
			labelValue("Input", m.syncConfirmInput),
			"Dry-run will be rechecked before deletion.",
		)
		if m.syncConfirmErr != "" {
			lines = append(lines, errorStyle.Render(m.syncConfirmErr))
		}
	} else if m.syncDryRunErr == nil && m.syncDryRunFingerprint != "" && m.syncDryRun.WouldDeleteCount == 0 {
		lines = append(lines, "", mutedStyle.Render("No deletable conflict copies found."))
	}
	lines = append(lines, "", helpStyle.Render("d dry-run • x clear confirmation • enter execute • esc back • r refresh • q quit"))
	return frame(lines...)
}

func (m Model) hasSyncDryRun() bool {
	return m.syncDryRunErr != nil || m.syncDryRunFingerprint != "" || len(m.syncDryRun.Conflicts) > 0
}

func (m Model) hasSyncExecution() bool {
	return m.syncExecErr != nil || m.syncExecResult.ConflictFingerprint != "" || m.syncExecResult.DeletedCount > 0 || m.syncExecResult.WouldDeleteCount > 0
}

func appendSyncDryRun(lines []string, m Model) []string {
	result := m.syncDryRun
	lines = append(lines,
		"",
		subtitleStyle.Render("Dry-run"),
		labelValue("Conflict copies", fmt.Sprintf("%d", len(result.Conflicts))),
		labelValue("Would delete", fmt.Sprintf("%d", result.WouldDeleteCount)),
		labelValue("Skipped", fmt.Sprintf("%d", result.SkippedCount)),
		labelValue("Scan truncated", formatBool(result.Truncated)),
	)
	if result.Truncated {
		lines = append(lines, "Warning: conflict scan truncated")
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	if m.syncDryRunErr != nil {
		lines = append(lines, errorStyle.Render(formatSyncError(m.syncDryRunErr)))
	}
	lines = appendSyncConflicts(lines, "Planned deletions", result.Conflicts, storesync.ConflictActionDelete)
	lines = appendSyncConflicts(lines, "Skipped", result.Conflicts, storesync.ConflictActionSkip)
	return lines
}

func appendSyncExecution(lines []string, m Model) []string {
	result := m.syncExecResult
	lines = append(lines,
		"",
		subtitleStyle.Render("Execution"),
		labelValue("Deleted", fmt.Sprintf("%d", result.DeletedCount)),
		labelValue("Skipped", fmt.Sprintf("%d", result.SkippedCount)),
		labelValue("Heartbeat", formatBool(result.HeartbeatUpdated)),
	)
	if m.syncExecErr != nil {
		lines = append(lines, errorStyle.Render(formatSyncError(m.syncExecErr)))
	} else if result.DeletedCount > 0 || result.HeartbeatUpdated {
		lines = append(lines, "Sync cleanup complete.")
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	return lines
}

func appendSyncConflicts(lines []string, title string, conflicts []storesync.ConflictCopy, action string) []string {
	start := len(lines)
	lines = append(lines, "", subtitleStyle.Render(title))
	for _, conflict := range conflicts {
		if conflict.Action != action {
			continue
		}
		lines = append(lines, syncConflictLine(conflict))
	}
	if len(lines) == start+2 {
		return lines[:start]
	}
	return lines
}

func syncConflictLine(conflict storesync.ConflictCopy) string {
	path := firstNonEmpty(conflict.RelativePath, conflict.Path, conflict.Name, "unknown")
	kind := firstNonEmpty(conflict.Kind, "unknown")
	line := fmt.Sprintf("- %s [%s]", path, kind)
	if conflict.Reason != "" {
		line += " reason=" + conflict.Reason
	}
	return line
}

func formatSyncError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if strings.Contains(message, "operation lock") {
		return "Lock/error: " + message
	}
	return "Error: " + message
}
