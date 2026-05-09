package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asudbring/loki-profile-manager/internal/app"
	"github.com/asudbring/loki-profile-manager/internal/machine"
)

const machineConfirmPhrase = "REGISTER MACHINE"

var machineFieldLabels = []string{"Name", "Allowed profiles", "Allowed buckets", "Active profile", "Active buckets"}

type machineRegisterMsg struct {
	record machine.Record
	err    error
}

func (m Model) updateMachineKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.machineBusy {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	if m.machineConfirm {
		return m.updateMachineConfirmKey(msg)
	}
	if m.machineEdit {
		return m.updateMachineEditKey(msg)
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
	case "e":
		if !m.status.Configured {
			m.machineRegisterErr = fmt.Errorf("machine register: store is not configured")
			return m, nil
		}
		m.initMachineEdit()
		return m, nil
	}
	return m, nil
}

func (m Model) updateMachineEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.machineEdit = false
		m.machineConfirm = false
		m.machineConfirmInput = ""
		m.machineConfirmErr = ""
		return m, nil
	case "up", "k":
		m.moveMachineField(-1)
		return m, nil
	case "down", "j", "enter":
		m.moveMachineField(1)
		return m, nil
	case "x":
		if len(splitCommaList(m.machineInputs[1])) == 0 {
			m.machineRegisterErr = fmt.Errorf("machine register: at least one allowed profile is required")
			return m, nil
		}
		m.machineConfirm = true
		m.machineConfirmInput = ""
		m.machineConfirmErr = ""
		m.machineRegisterErr = nil
		return m, nil
	case "backspace", "ctrl+h":
		if len(m.machineInputs) == 0 {
			m.initMachineEdit()
		}
		value := m.machineInputs[m.machineFieldIndex]
		if len(value) > 0 {
			m.machineInputs[m.machineFieldIndex] = value[:len(value)-1]
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		if len(m.machineInputs) == 0 {
			m.initMachineEdit()
		}
		m.machineInputs[m.machineFieldIndex] += string(msg.Runes)
	}
	return m, nil
}

func (m Model) updateMachineConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.machineConfirm = false
		m.machineConfirmInput = ""
		m.machineConfirmErr = ""
		return m, nil
	case "enter":
		if m.machineConfirmInput != machineConfirmPhrase {
			m.machineConfirmErr = fmt.Sprintf("confirmation mismatch; type %q", machineConfirmPhrase)
			return m, nil
		}
		req := app.RegisterMachineRequest{
			DisplayName:           strings.TrimSpace(m.machineInputs[0]),
			AllowedParentProfiles: splitCommaList(m.machineInputs[1]),
			AllowedBuckets:        splitCommaList(m.machineInputs[2]),
			ActiveProfile:         strings.TrimSpace(m.machineInputs[3]),
			ActiveBuckets:         splitCommaList(m.machineInputs[4]),
		}
		m.machineBusy = true
		m.machineConfirmErr = ""
		return m, machineRegisterCmd(m.ctx, m.client, req)
	case "backspace", "ctrl+h":
		if len(m.machineConfirmInput) > 0 {
			m.machineConfirmInput = m.machineConfirmInput[:len(m.machineConfirmInput)-1]
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.machineConfirmInput += string(msg.Runes)
	}
	return m, nil
}

func (m *Model) initMachineEdit() {
	name := ""
	allowedProfiles := []string{}
	allowedBuckets := []string{}
	activeProfile := m.status.ActiveProfile
	activeBuckets := cloneStringSlice(m.status.ActiveBuckets)
	if m.machine.Record != nil {
		name = m.machine.Record.DisplayName
		allowedProfiles = cloneStringSlice(m.machine.Record.AllowedParentProfiles)
		allowedBuckets = cloneStringSlice(m.machine.Record.AllowedBuckets)
		if activeProfile == "" {
			activeProfile = m.machine.Record.ActiveProfile
		}
		if len(activeBuckets) == 0 {
			activeBuckets = cloneStringSlice(m.machine.Record.ActiveBuckets)
		}
	} else {
		if m.status.MachineDisplayName != "" {
			name = m.status.MachineDisplayName
		}
		allowedProfiles = cloneStringSlice(m.status.MachineAllowedParentProfiles)
		allowedBuckets = cloneStringSlice(m.status.MachineAllowedBuckets)
	}
	if len(allowedProfiles) == 0 {
		for _, profile := range m.catalog.Profiles {
			allowedProfiles = append(allowedProfiles, profile.Name)
		}
	}
	if len(allowedBuckets) == 0 {
		for _, profile := range m.catalog.Profiles {
			for _, bucket := range profile.Buckets {
				allowedBuckets = append(allowedBuckets, bucket.Name)
			}
		}
	}
	m.machineInputs = []string{
		name,
		strings.Join(allowedProfiles, ","),
		strings.Join(uniqueStrings(allowedBuckets), ","),
		activeProfile,
		strings.Join(activeBuckets, ","),
	}
	m.machineFieldIndex = 0
	m.machineEdit = true
	m.machineConfirm = false
	m.machineRegisterErr = nil
	m.machineConfirmErr = ""
}

func (m *Model) moveMachineField(delta int) {
	if len(m.machineInputs) == 0 {
		m.initMachineEdit()
	}
	m.machineFieldIndex = (m.machineFieldIndex + delta + len(m.machineInputs)) % len(m.machineInputs)
}

func machineRegisterCmd(ctx context.Context, client Client, req app.RegisterMachineRequest) tea.Cmd {
	return func() tea.Msg {
		record, err := client.RegisterMachine(ctx, req)
		return machineRegisterMsg{record: record, err: err}
	}
}

func (m Model) machineView() string {
	lines := []string{
		titleStyle.Render("Machine"),
		"",
	}
	if m.machineErr != nil {
		lines = append(lines,
			errorStyle.Render("Machine status unavailable: "+m.machineErr.Error()),
			"",
			helpStyle.Render("esc back • r refresh • q quit"),
		)
		return frame(lines...)
	}
	if !m.status.Configured {
		lines = append(lines,
			mutedStyle.Render("Store not configured; machine registry unavailable."),
		)
		if m.machineRegisterErr != nil {
			lines = append(lines, errorStyle.Render("Error: "+m.machineRegisterErr.Error()))
		}
		lines = append(lines, "", helpStyle.Render("esc back • r refresh • q quit"))
		return frame(lines...)
	}
	lines = append(lines,
		labelValue("Store", firstNonEmpty(m.machine.StorePath, m.status.StorePath, "unknown")),
		labelValue("Machine ID path", firstNonEmpty(m.machine.MachineIDPath, "unknown")),
		labelValue("Machine ID", firstNonEmpty(m.machine.MachineID, "not created")),
		labelValue("Registered", formatBool(m.machine.Registered)),
	)
	if m.machine.Message != "" {
		lines = append(lines, labelValue("Message", m.machine.Message))
	}
	if m.machine.Warning != "" {
		lines = append(lines, labelValue("Warning", m.machine.Warning))
	}
	if m.machine.Record != nil {
		record := m.machine.Record
		lines = append(lines,
			"",
			subtitleStyle.Render("Registry record"),
			labelValue("Display name", firstNonEmpty(record.DisplayName, "unknown")),
			labelValue("OS", firstNonEmpty(record.OS, "unknown")),
			labelValue("Hostname", firstNonEmpty(record.Hostname, "unknown")),
			labelValue("Allowed profiles", formatList(record.AllowedParentProfiles)),
			labelValue("Allowed buckets", formatList(record.AllowedBuckets)),
			labelValue("Active profile", firstNonEmpty(record.ActiveProfile, "not set")),
			labelValue("Active buckets", formatList(record.ActiveBuckets)),
			labelValue("Last seen", firstNonEmpty(record.LastSeen, "unknown")),
			labelValue("Loki version", firstNonEmpty(record.LokiVersion, "unknown")),
		)
	}
	if m.machineRegisterRecord.MachineID != "" && m.machineRegisterErr == nil {
		lines = append(lines, "", "Machine registered.")
	}
	if m.machineRegisterErr != nil {
		lines = append(lines, "", errorStyle.Render("Error: "+m.machineRegisterErr.Error()))
	}
	if m.machineBusy {
		lines = append(lines, "", mutedStyle.Render("Machine registration running..."), "", helpStyle.Render("q quit"))
		return frame(lines...)
	}
	if m.machineConfirm {
		lines = append(lines,
			"",
			subtitleStyle.Render("Confirm machine registration"),
			labelValue("Required phrase", machineConfirmPhrase),
			labelValue("Input", m.machineConfirmInput),
		)
		if m.machineConfirmErr != "" {
			lines = append(lines, errorStyle.Render(m.machineConfirmErr))
		}
		lines = append(lines, "", helpStyle.Render("enter register • esc cancel • q quit"))
		return frame(lines...)
	}
	if m.machineEdit {
		lines = append(lines, "", subtitleStyle.Render("Edit registration"))
		for i, label := range machineFieldLabels {
			cursor := "  "
			if i == m.machineFieldIndex {
				cursor = "> "
			}
			value := ""
			if i < len(m.machineInputs) {
				value = m.machineInputs[i]
			}
			lines = append(lines, fmt.Sprintf("%s%s: %s", cursor, label, value))
		}
		lines = append(lines, "", helpStyle.Render("↑/↓ field • type edit • x confirm • esc cancel • q quit"))
		return frame(lines...)
	}
	lines = append(lines, "", helpStyle.Render("e edit/register • esc back • r refresh • q quit"))
	return frame(lines...)
}

func splitCommaList(value string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
