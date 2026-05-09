package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asudbring/loki-profile-manager/internal/app"
)

type switchDryRunMsg struct {
	result      app.SwitchResult
	fingerprint string
	err         error
}

type switchExecuteMsg struct {
	result app.SwitchResult
	err    error
}

func (m *Model) ensureSwitchSelection() {
	if m.switchSelectedBuckets == nil {
		m.switchSelectedBuckets = map[string]bool{}
	}
	if len(m.catalog.Profiles) == 0 {
		m.switchProfileIndex = 0
		m.switchBucketIndex = 0
		return
	}
	if !m.switchInitialized {
		m.switchInitialized = true
		for _, bucket := range m.status.ActiveBuckets {
			m.switchSelectedBuckets[bucket] = true
		}
		if m.status.ActiveProfile != "" {
			for i, profile := range m.catalog.Profiles {
				if profile.Name == m.status.ActiveProfile {
					m.switchProfileIndex = i
					break
				}
			}
		}
	}
	if m.switchProfileIndex < 0 || m.switchProfileIndex >= len(m.catalog.Profiles) {
		m.switchProfileIndex = 0
	}
	buckets := m.currentSwitchProfileBuckets()
	if len(buckets) == 0 {
		m.switchBucketIndex = 0
		return
	}
	if m.switchBucketIndex < 0 || m.switchBucketIndex >= len(buckets) {
		m.switchBucketIndex = 0
	}
}

func (m Model) currentSwitchProfile() (app.ProfileSummary, bool) {
	if len(m.catalog.Profiles) == 0 || m.switchProfileIndex < 0 || m.switchProfileIndex >= len(m.catalog.Profiles) {
		return app.ProfileSummary{}, false
	}
	return m.catalog.Profiles[m.switchProfileIndex], true
}

func (m Model) currentSwitchProfileBuckets() []app.BucketSummary {
	profile, ok := m.currentSwitchProfile()
	if !ok {
		return nil
	}
	return profile.Buckets
}

func (m Model) selectedSwitchBuckets() []string {
	profile, ok := m.currentSwitchProfile()
	if !ok || len(m.switchSelectedBuckets) == 0 {
		return []string{}
	}
	out := []string{}
	for _, bucket := range profile.Buckets {
		if m.switchSelectedBuckets[bucket.Name] {
			out = append(out, bucket.Name)
		}
	}
	return out
}

func (m Model) switchRequest(dryRun bool) (app.SwitchRequest, bool) {
	profile, ok := m.currentSwitchProfile()
	if !ok {
		return app.SwitchRequest{}, false
	}
	return app.SwitchRequest{ParentProfile: profile.Name, Buckets: m.selectedSwitchBuckets(), DryRun: dryRun, Yes: !dryRun}, true
}

func (m Model) updateSwitchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.switchBusy {
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
		m.moveSwitchProfile(-1)
		return m, nil
	case "down", "j":
		m.moveSwitchProfile(1)
		return m, nil
	case "left", "h":
		m.moveSwitchBucket(-1)
		return m, nil
	case "right", "l":
		m.moveSwitchBucket(1)
		return m, nil
	case " ":
		m.toggleSwitchBucket()
		return m, nil
	case "d":
		return m.startSwitchDryRun()
	case "x":
		if !m.canExecuteSwitch() {
			m.switchExecErr = fmt.Errorf("switch: successful dry-run required before execute")
			return m, nil
		}
		m.confirmInput = ""
		m.confirmErr = ""
		m.screen = ScreenConfirm
		return m, nil
	}
	return m, nil
}

func (m *Model) moveSwitchProfile(delta int) {
	if len(m.catalog.Profiles) == 0 {
		return
	}
	m.switchProfileIndex = (m.switchProfileIndex + delta + len(m.catalog.Profiles)) % len(m.catalog.Profiles)
	m.switchBucketIndex = 0
	m.switchSelectedBuckets = map[string]bool{}
	m.clearSwitchDryRun()
}

func (m *Model) moveSwitchBucket(delta int) {
	buckets := m.currentSwitchProfileBuckets()
	if len(buckets) == 0 {
		m.switchBucketIndex = 0
		return
	}
	m.switchBucketIndex = (m.switchBucketIndex + delta + len(buckets)) % len(buckets)
}

func (m *Model) toggleSwitchBucket() {
	buckets := m.currentSwitchProfileBuckets()
	if len(buckets) == 0 {
		return
	}
	if m.switchSelectedBuckets == nil {
		m.switchSelectedBuckets = map[string]bool{}
	}
	name := buckets[m.switchBucketIndex].Name
	m.switchSelectedBuckets[name] = !m.switchSelectedBuckets[name]
	m.clearSwitchDryRun()
}

func (m *Model) clearSwitchDryRun() {
	m.switchDryRun = app.SwitchResult{}
	m.switchDryRunErr = nil
	m.switchDryRunFingerprint = ""
	m.switchExecResult = app.SwitchResult{}
	m.switchExecErr = nil
}

func (m Model) startSwitchDryRun() (tea.Model, tea.Cmd) {
	req, ok := m.switchRequest(true)
	if !ok {
		m.switchDryRunErr = fmt.Errorf("switch: no profile selected")
		return m, nil
	}
	m.switchBusy = true
	m.switchDryRunErr = nil
	m.switchExecErr = nil
	return m, switchDryRunCmd(m.ctx, m.client, req)
}

func (m Model) canExecuteSwitch() bool {
	return !m.switchBusy && m.switchDryRunFingerprint != "" && m.switchDryRunErr == nil && m.switchDryRun.Plan.Profile != ""
}

func (m Model) updateConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.switchBusy {
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
		m.screen = ScreenSwitch
		m.confirmInput = ""
		m.confirmErr = ""
		return m, nil
	case "enter":
		phrase := m.confirmPhrase()
		if m.confirmInput != phrase {
			m.confirmErr = fmt.Sprintf("confirmation mismatch; type %q", phrase)
			return m, nil
		}
		req, ok := m.switchRequest(false)
		if !ok {
			m.confirmErr = "switch: no profile selected"
			return m, nil
		}
		m.switchBusy = true
		m.confirmErr = ""
		return m, switchExecuteCmd(m.ctx, m.client, req, m.switchDryRunFingerprint)
	case "backspace", "ctrl+h":
		if len(m.confirmInput) > 0 {
			m.confirmInput = m.confirmInput[:len(m.confirmInput)-1]
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.confirmInput += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

func (m Model) confirmPhrase() string {
	profile, ok := m.currentSwitchProfile()
	if !ok {
		return "SWITCH"
	}
	parts := append([]string{"SWITCH", profile.Name}, m.selectedSwitchBuckets()...)
	return strings.Join(parts, " ")
}

func switchDryRunCmd(ctx context.Context, client Client, req app.SwitchRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := client.Switch(ctx, req)
		fingerprint := ""
		if err == nil {
			fingerprint = fingerprintSwitchResult(result)
		}
		return switchDryRunMsg{result: result, fingerprint: fingerprint, err: err}
	}
}

func switchExecuteCmd(ctx context.Context, client Client, req app.SwitchRequest, expectedFingerprint string) tea.Cmd {
	return func() tea.Msg {
		dryReq := req
		dryReq.DryRun = true
		dryReq.Yes = false
		dryRun, err := client.Switch(ctx, dryReq)
		if err != nil {
			return switchExecuteMsg{result: dryRun, err: fmt.Errorf("dry-run recheck failed: %w", err)}
		}
		fingerprint := fingerprintSwitchResult(dryRun)
		if fingerprint != expectedFingerprint {
			return switchExecuteMsg{result: dryRun, err: fmt.Errorf("dry-run fingerprint changed; rerun dry-run before executing")}
		}
		result, err := client.Switch(ctx, req)
		return switchExecuteMsg{result: result, err: err}
	}
}

func (m Model) switchView() string {
	m.ensureSwitchSelection()
	lines := []string{
		titleStyle.Render("Switch profile"),
		"",
	}
	if !m.status.Configured {
		return frame(append(lines, mutedStyle.Render("Store not configured; switch unavailable."), "", helpStyle.Render("esc back • r refresh • q quit"))...)
	}
	if m.catalogErr != nil {
		return frame(append(lines, errorStyle.Render("Profile catalog unavailable: "+m.catalogErr.Error()), "", helpStyle.Render("esc back • r refresh • q quit"))...)
	}
	profile, ok := m.currentSwitchProfile()
	if !ok {
		return frame(append(lines, mutedStyle.Render("No profiles found."), "", helpStyle.Render("esc back • r refresh • q quit"))...)
	}
	lines = append(lines,
		labelValue("Selected profile", profile.Name),
		labelValue("Selected buckets", formatList(m.selectedSwitchBuckets())),
		"",
		subtitleStyle.Render("Profiles"),
	)
	for i, candidate := range m.catalog.Profiles {
		prefix := "  "
		if i == m.switchProfileIndex {
			prefix = "> "
		}
		lines = append(lines, prefix+candidate.Name)
	}
	lines = append(lines, "", subtitleStyle.Render("Buckets"))
	buckets := m.currentSwitchProfileBuckets()
	if len(buckets) == 0 {
		lines = append(lines, mutedStyle.Render("No buckets for selected profile."))
	} else {
		for i, bucket := range buckets {
			cursor := "  "
			if i == m.switchBucketIndex {
				cursor = "> "
			}
			checked := "[ ]"
			if m.switchSelectedBuckets[bucket.Name] {
				checked = "[x]"
			}
			lines = append(lines, fmt.Sprintf("%s%s %s", cursor, checked, bucket.Name))
		}
	}
	lines = append(lines, "")
	lines = appendSwitchPlan(lines, m)
	lines = append(lines, "", helpStyle.Render("↑/↓ profile • ←/→ bucket • space toggle • d dry-run • x execute • esc back • q quit"))
	return frame(lines...)
}

func appendSwitchPlan(lines []string, m Model) []string {
	if m.switchBusy {
		return append(lines, mutedStyle.Render("Switch operation running..."))
	}
	if m.switchDryRun.Plan.Profile == "" && m.switchDryRunErr == nil && m.switchExecResult.Plan.Profile == "" && m.switchExecErr == nil {
		return append(lines, mutedStyle.Render("Run dry-run before executing."))
	}
	if m.switchDryRun.Plan.Profile != "" || m.switchDryRunErr != nil {
		lines = append(lines, subtitleStyle.Render("Dry-run"))
		lines = append(lines, switchResultSummary(m.switchDryRun)...)
		if m.switchDryRunErr != nil {
			lines = append(lines, errorStyle.Render("Blocker: "+m.switchDryRunErr.Error()))
		} else {
			lines = append(lines, "Ready to execute after confirmation.")
		}
	}
	if m.switchExecResult.Plan.Profile != "" || m.switchExecErr != nil {
		lines = append(lines, "", subtitleStyle.Render("Execution"))
		lines = append(lines, switchResultSummary(m.switchExecResult)...)
		if m.switchExecErr != nil {
			lines = append(lines, errorStyle.Render("Error: "+m.switchExecErr.Error()))
		} else {
			lines = append(lines, "Switch complete.")
		}
	}
	return lines
}

func switchResultSummary(result app.SwitchResult) []string {
	if result.Plan.Profile == "" {
		return nil
	}
	lines := []string{
		labelValue("Profile", result.Plan.Profile),
		labelValue("Buckets", formatList(result.Plan.Buckets)),
		labelValue("Operations", fmt.Sprintf("%d", len(result.Plan.Operations))),
		labelValue("Changed", fmt.Sprintf("%d", result.Changed)),
	}
	if result.SnapshotID != "" {
		lines = append(lines, labelValue("Snapshot", result.SnapshotID))
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	for i, op := range result.Plan.Operations {
		if i >= 8 {
			lines = append(lines, fmt.Sprintf("... %d more operation(s)", len(result.Plan.Operations)-i))
			break
		}
		line := fmt.Sprintf("- %s %s", op.Type, op.TargetPath)
		if op.Safety.Class != "" {
			line += fmt.Sprintf(" [%s]", op.Safety.Class)
		}
		if op.Safety.Message != "" {
			line += " " + op.Safety.Message
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) confirmView() string {
	phrase := m.confirmPhrase()
	lines := []string{
		titleStyle.Render("Confirm switch"),
		"",
		"Dry-run fingerprint will be rechecked before execution.",
		labelValue("Required phrase", phrase),
		labelValue("Input", m.confirmInput),
	}
	if m.switchBusy {
		lines = append(lines, mutedStyle.Render("Switch execution running..."))
	}
	if m.confirmErr != "" {
		lines = append(lines, errorStyle.Render(m.confirmErr))
	}
	lines = append(lines, "", helpStyle.Render("enter execute • esc cancel • q quit"))
	return frame(lines...)
}

func switchTargetDescription(profile string, buckets []string) string {
	if len(buckets) == 0 {
		return profile
	}
	return profile + " (" + strings.Join(buckets, ", ") + ")"
}
