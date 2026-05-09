package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asudbring/loki-profile-manager/internal/activation"
	"github.com/asudbring/loki-profile-manager/internal/app"
)

type snapshotShowMsg struct {
	result app.SnapshotShowResult
	err    error
}

type snapshotRestoreDryRunMsg struct {
	result app.SnapshotRestoreDryRunResult
	target string
	err    error
}

func (m Model) updateSnapshotsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.snapshotBusy {
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
		return m, nil
	case "r":
		m.loading = true
		m.err = nil
		m.screen = ScreenLoading
		return m, tea.Batch(m.spinner.Tick, loadDashboardCmd(m.ctx, m.client))
	case "up", "k":
		m.moveSnapshot(-1)
		return m, nil
	case "down", "j":
		m.moveSnapshot(1)
		return m, nil
	case "left", "h":
		m.moveSnapshotTarget(-1)
		return m, nil
	case "right", "l":
		m.moveSnapshotTarget(1)
		return m, nil
	case "enter":
		return m.startSnapshotShow()
	case "d":
		return m.startSnapshotRestoreDryRun("")
	case "t":
		target, ok := m.currentSnapshotTarget()
		if !ok {
			m.snapshotRestoreDryRunErr = fmt.Errorf("snapshots: load a snapshot and select a target before targeted dry-run")
			return m, nil
		}
		return m.startSnapshotRestoreDryRun(target.TargetPath)
	}
	return m, nil
}

func (m *Model) ensureSnapshotSelection() {
	if len(m.snapshots.Snapshots) == 0 {
		m.snapshotIndex = 0
		return
	}
	if m.snapshotIndex < 0 || m.snapshotIndex >= len(m.snapshots.Snapshots) {
		m.snapshotIndex = 0
	}
}

func (m *Model) ensureSnapshotTargetSelection() {
	if len(m.snapshotShow.Snapshot.Targets) == 0 {
		m.snapshotTargetIndex = 0
		return
	}
	if m.snapshotTargetIndex < 0 || m.snapshotTargetIndex >= len(m.snapshotShow.Snapshot.Targets) {
		m.snapshotTargetIndex = 0
	}
}

func (m *Model) moveSnapshot(delta int) {
	if len(m.snapshots.Snapshots) == 0 {
		return
	}
	m.snapshotIndex = (m.snapshotIndex + delta + len(m.snapshots.Snapshots)) % len(m.snapshots.Snapshots)
	m.snapshotTargetIndex = 0
	m.snapshotShow = app.SnapshotShowResult{}
	m.snapshotShowErr = nil
	m.snapshotRestoreDryRun = app.SnapshotRestoreDryRunResult{}
	m.snapshotRestoreDryRunErr = nil
	m.snapshotRestoreDryRunTarget = ""
}

func (m *Model) moveSnapshotTarget(delta int) {
	if !m.hasShownSnapshot() || len(m.snapshotShow.Snapshot.Targets) == 0 {
		return
	}
	m.snapshotTargetIndex = (m.snapshotTargetIndex + delta + len(m.snapshotShow.Snapshot.Targets)) % len(m.snapshotShow.Snapshot.Targets)
}

func (m Model) selectedSnapshot() (activation.SnapshotSummary, bool) {
	if len(m.snapshots.Snapshots) == 0 || m.snapshotIndex < 0 || m.snapshotIndex >= len(m.snapshots.Snapshots) {
		return activation.SnapshotSummary{}, false
	}
	return m.snapshots.Snapshots[m.snapshotIndex], true
}

func (m Model) selectedSnapshotID() (string, bool) {
	snapshot, ok := m.selectedSnapshot()
	if !ok || strings.TrimSpace(snapshot.SnapshotID) == "" {
		return "", false
	}
	return snapshot.SnapshotID, true
}

func (m Model) hasShownSnapshot() bool {
	id, ok := m.selectedSnapshotID()
	return ok && m.snapshotShow.Snapshot.SnapshotID == id
}

func (m Model) currentSnapshotTarget() (activation.SnapshotEntry, bool) {
	if !m.hasShownSnapshot() || len(m.snapshotShow.Snapshot.Targets) == 0 || m.snapshotTargetIndex < 0 || m.snapshotTargetIndex >= len(m.snapshotShow.Snapshot.Targets) {
		return activation.SnapshotEntry{}, false
	}
	return m.snapshotShow.Snapshot.Targets[m.snapshotTargetIndex], true
}

func (m Model) startSnapshotShow() (tea.Model, tea.Cmd) {
	id, ok := m.selectedSnapshotID()
	if !ok {
		m.snapshotShowErr = fmt.Errorf("snapshots: no snapshot selected")
		return m, nil
	}
	m.snapshotBusy = true
	m.snapshotShowErr = nil
	m.snapshotRestoreDryRun = app.SnapshotRestoreDryRunResult{}
	m.snapshotRestoreDryRunErr = nil
	m.snapshotRestoreDryRunTarget = ""
	return m, snapshotShowCmd(m.ctx, m.client, id)
}

func (m Model) startSnapshotRestoreDryRun(target string) (tea.Model, tea.Cmd) {
	id, ok := m.selectedSnapshotID()
	if !ok {
		m.snapshotRestoreDryRunErr = fmt.Errorf("snapshots: no snapshot selected")
		return m, nil
	}
	m.snapshotBusy = true
	m.snapshotRestoreDryRunErr = nil
	m.snapshotRestoreDryRunTarget = target
	return m, snapshotRestoreDryRunCmd(m.ctx, m.client, id, target)
}

func snapshotShowCmd(ctx context.Context, client Client, snapshotID string) tea.Cmd {
	return func() tea.Msg {
		result, err := client.ShowSnapshot(ctx, app.SnapshotShowRequest{SnapshotID: snapshotID})
		return snapshotShowMsg{result: result, err: err}
	}
}

func snapshotRestoreDryRunCmd(ctx context.Context, client Client, snapshotID, target string) tea.Cmd {
	return func() tea.Msg {
		result, err := client.RestoreSnapshotDryRun(ctx, app.SnapshotRestoreDryRunRequest{SnapshotID: snapshotID, DryRun: true, Target: target})
		return snapshotRestoreDryRunMsg{result: result, target: target, err: err}
	}
}

func (m Model) snapshotsView() string {
	m.ensureSnapshotSelection()
	lines := []string{
		titleStyle.Render("Snapshots"),
		"",
		labelValue("Local snapshot dir", firstNonEmpty(m.snapshots.SnapshotDir, "unknown")),
		labelValue("Snapshots", fmt.Sprintf("%d", len(m.snapshots.Snapshots))),
	}
	if m.snapshotsErr != nil {
		return frame(append(lines, "", errorStyle.Render("Snapshots unavailable: "+m.snapshotsErr.Error()), "", helpStyle.Render("esc back • r refresh • q quit"))...)
	}
	if len(m.snapshots.Snapshots) == 0 {
		lines = append(lines, "", mutedStyle.Render("No retained snapshots."), "", helpStyle.Render("esc back • r refresh • q quit"))
		return frame(lines...)
	}
	lines = append(lines, "", subtitleStyle.Render("Retained snapshots"))
	for i, snapshot := range m.snapshots.Snapshots {
		lines = append(lines, snapshotSummaryLine(i == m.snapshotIndex, snapshot))
		if snapshot.MetadataError != "" {
			lines = append(lines, "  metadata warning: "+snapshot.MetadataError)
		}
	}
	if m.snapshotBusy {
		lines = append(lines, "", mutedStyle.Render("Snapshot operation running..."))
	}
	if m.snapshotShowErr != nil {
		lines = append(lines, "", errorStyle.Render("Error: "+m.snapshotShowErr.Error()))
	}
	if m.hasShownSnapshot() {
		lines = appendSnapshotDetail(lines, m)
	}
	if m.snapshotRestoreDryRunErr != nil || m.snapshotRestoreDryRun.SnapshotID != "" {
		lines = appendSnapshotRestoreDryRun(lines, m.snapshotRestoreDryRun, m.snapshotRestoreDryRunErr)
	}
	lines = append(lines, "", helpStyle.Render("↑/↓ snapshot • enter show • ←/→ target • d full dry-run • t target dry-run • esc back • r refresh • q quit"))
	return frame(lines...)
}

func snapshotSummaryLine(selected bool, snapshot activation.SnapshotSummary) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	previousProfile := firstNonEmpty(snapshot.PreviousActiveProfile, "not set")
	line := fmt.Sprintf("%s%s", prefix, firstNonEmpty(snapshot.SnapshotID, "unknown"))
	if snapshot.CreatedAt != "" {
		line += " created=" + snapshot.CreatedAt
	}
	line += fmt.Sprintf(" previous=%s buckets=%s targets=%d", previousProfile, formatSnapshotList(snapshot.PreviousActiveBuckets), snapshot.TargetCount)
	if len(snapshot.TargetKinds) > 0 {
		line += " kinds=" + strings.Join(snapshot.TargetKinds, ",")
	}
	if !snapshot.Exists {
		line += " exists=false"
	}
	return line
}

func appendSnapshotDetail(lines []string, m Model) []string {
	snapshot := m.snapshotShow.Snapshot
	lines = append(lines,
		"",
		subtitleStyle.Render("Snapshot metadata"),
		labelValue("ID", firstNonEmpty(snapshot.SnapshotID, "unknown")),
		labelValue("Created", firstNonEmpty(snapshot.CreatedAt, "unknown")),
		labelValue("Machine ID", firstNonEmpty(snapshot.MachineID, "unknown")),
		labelValue("Reason", firstNonEmpty(snapshot.Reason, "not recorded")),
		labelValue("Source snapshot", firstNonEmpty(snapshot.SourceSnapshotID, "none")),
		labelValue("Previous active profile", firstNonEmpty(snapshot.PreviousActiveProfile, "not set")),
		labelValue("Previous active buckets", formatSnapshotList(snapshot.PreviousActiveBuckets)),
		labelValue("Targets", fmt.Sprintf("%d", len(snapshot.Targets))),
		labelValue("Managed target rows", fmt.Sprintf("%d", len(snapshot.ManagedTargets))),
	)
	if len(snapshot.Targets) > 0 {
		lines = append(lines, "", subtitleStyle.Render("Targets"))
		for i, target := range snapshot.Targets {
			lines = append(lines, snapshotTargetLine(i == m.snapshotTargetIndex, target))
			if activation.PathLooksSensitive(target.TargetPath) || activation.PathLooksSensitive(target.LinkTarget) {
				lines = append(lines, "  warning: sensitive-looking target path redacted")
			}
		}
	}
	return lines
}

func snapshotTargetLine(selected bool, target activation.SnapshotEntry) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	kind := firstNonEmpty(target.Kind, "unknown")
	line := fmt.Sprintf("%s- %s %s", prefix, kind, formatSnapshotPath(target.TargetPath))
	if value := shortSnapshotHash(target.Hash); value != "" {
		line += " hash=" + value
	}
	if value := shortSnapshotHash(target.ExpectedHash); value != "" {
		line += " expected_hash=" + value
	}
	if target.ExpectedMode != "" {
		line += " expected_mode=" + target.ExpectedMode
	}
	if target.LinkTarget != "" {
		line += " link_target=" + formatSnapshotPath(target.LinkTarget)
	}
	return line
}

func appendSnapshotRestoreDryRun(lines []string, result app.SnapshotRestoreDryRunResult, err error) []string {
	lines = append(lines, "", subtitleStyle.Render("Restore dry-run"))
	if result.SnapshotID != "" {
		lines = append(lines,
			labelValue("ID", result.SnapshotID),
			"Mode: dry-run only; no restore writes executed by TUI.",
		)
		if result.TargetFilter != "" || result.TargetFilterRedacted {
			lines = append(lines, labelValue("Target filter", formatRestoreTargetFilter(result)))
		}
		if result.GuardRecorded {
			lines = append(lines, labelValue("Guard", "recorded, expires="+result.GuardExpiresAt))
			command := restoreCLICommand(result)
			if command == "" {
				lines = append(lines, "Command hidden because target path is redacted; rerun CLI dry-run with explicit --target path.")
			} else {
				lines = append(lines, labelValue("Run", command))
				if result.TargetFilter == "" && !result.TargetFilterRedacted {
					lines = append(lines, fmt.Sprintf("CLI will require consent phrase: RESTORE %s", result.SnapshotID))
				}
			}
		} else if len(result.Blockers) > 0 {
			lines = append(lines, labelValue("Guard", "not recorded"))
		}
		lines = appendSnapshotRestoreSummary(lines, result.Summary)
		if len(result.Targets) > 0 {
			lines = append(lines, "", subtitleStyle.Render("Restore targets"))
			for _, target := range result.Targets {
				lines = append(lines, snapshotRestoreTargetLine(target))
				for _, warning := range target.Warnings {
					lines = append(lines, "  warning: "+warning)
				}
			}
		}
	}
	if err != nil {
		lines = append(lines, errorStyle.Render("Error: "+err.Error()))
	}
	for _, blocker := range result.Blockers {
		lines = append(lines, errorStyle.Render("Blocker: "+blocker))
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	return lines
}

func appendSnapshotRestoreSummary(lines []string, summary app.SnapshotRestoreDryRunSummary) []string {
	previousProfile := firstNonEmpty(summary.PreviousActiveProfile, "not set")
	return append(lines,
		"",
		subtitleStyle.Render("Restore summary"),
		labelValue("Targets", fmt.Sprintf("%d", summary.TargetCount)),
		labelValue("Restore files", fmt.Sprintf("%d", summary.RestoreFileCount)),
		labelValue("Restore directories", fmt.Sprintf("%d", summary.RestoreDirectoryCount)),
		labelValue("Restore symlinks", fmt.Sprintf("%d", summary.RestoreSymlinkCount)),
		labelValue("Remove created", fmt.Sprintf("%d", summary.RemoveCreatedTargetCount)),
		labelValue("Skip missing", fmt.Sprintf("%d", summary.SkipMissingTargetAbsentCount)),
		labelValue("Unknown", fmt.Sprintf("%d", summary.UnknownCount)),
		labelValue("Would restore active state", formatBool(summary.WouldRestoreActiveState)),
		labelValue("Previous active profile", previousProfile),
		labelValue("Previous active buckets", formatSnapshotList(summary.PreviousActiveBuckets)),
		labelValue("Managed target rows", fmt.Sprintf("%d", summary.WouldRestoreManagedTargetRows)),
	)
}

func snapshotRestoreTargetLine(target app.SnapshotRestoreDryRunTarget) string {
	path := target.TargetPath
	if target.TargetPathRedacted || path == "" {
		path = "(redacted-sensitive-path)"
	}
	line := fmt.Sprintf("- %s %s", firstNonEmpty(target.Action, "unknown"), path)
	if target.CurrentKind != "" {
		line += " current=" + target.CurrentKind
	}
	if target.CurrentMode != "" {
		line += " mode=" + target.CurrentMode
	}
	if target.CurrentHashPrefix != "" {
		line += " hash=" + target.CurrentHashPrefix
	}
	if target.SnapshotHashPrefix != "" {
		line += " snapshot_hash=" + target.SnapshotHashPrefix
	}
	if target.ExpectedHashPrefix != "" {
		line += " expected_hash=" + target.ExpectedHashPrefix
	}
	if target.ExpectedMode != "" {
		line += " expected_mode=" + target.ExpectedMode
	}
	if target.LinkTarget != "" {
		line += " link_target=" + target.LinkTarget
	} else if target.LinkTargetRedacted {
		line += " link_target=(redacted)"
	}
	if target.TargetPathRedacted {
		line += " [redacted]"
	}
	return line
}

func formatSnapshotList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func formatSnapshotPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "unknown"
	}
	if activation.PathLooksSensitive(path) {
		return "(redacted-sensitive-path)"
	}
	return path
}

func formatRestoreTargetFilter(result app.SnapshotRestoreDryRunResult) string {
	if result.TargetFilterRedacted || result.TargetFilter == "" {
		return "(redacted-sensitive-path)"
	}
	return result.TargetFilter
}

func restoreCLICommand(result app.SnapshotRestoreDryRunResult) string {
	if !result.GuardRecorded || result.SnapshotID == "" || result.TargetFilterRedacted {
		return ""
	}
	parts := []string{"loki", "snapshots", "restore", shellQuote(result.SnapshotID)}
	if result.TargetFilter != "" {
		parts = append(parts, "--target", shellQuote(result.TargetFilter))
	}
	parts = append(parts, "--yes")
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n\"'\\$`;&|<>()*?![]{}") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func shortSnapshotHash(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
