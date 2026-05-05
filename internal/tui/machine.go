package tui

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
			"",
			helpStyle.Render("esc back • r refresh • q quit"),
		)
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
	lines = append(lines, "", helpStyle.Render("esc back • r refresh • q quit"))
	return frame(lines...)
}
