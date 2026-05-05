package tui

import "fmt"

func (m Model) secretsView() string {
	lines := []string{
		titleStyle.Render("Secrets"),
		"",
	}
	if m.secretsErr != nil {
		lines = append(lines,
			errorStyle.Render("Secrets status unavailable: "+m.secretsErr.Error()),
			"",
			helpStyle.Render("esc back • r refresh • q quit"),
		)
		return frame(lines...)
	}
	lines = append(lines,
		labelValue("Provider", firstNonEmpty(string(m.secrets.Provider), "unknown")),
		labelValue("CLI installed", formatBool(m.secrets.CLIInstalled)),
		labelValue("Authenticated", formatBool(m.secrets.Authenticated)),
		labelValue("Ready", formatBool(m.secrets.Ready)),
		"",
		mutedStyle.Render("Secret checks are name/status only; values never render."),
	)
	if len(m.secrets.Checks) > 0 {
		lines = append(lines, "", subtitleStyle.Render("Checks"))
		for _, check := range m.secrets.Checks {
			line := fmt.Sprintf("- %s [%s]", firstNonEmpty(check.Code, "check"), firstNonEmpty(string(check.Severity), "info"))
			if check.Remediation != "" {
				line += " fix available"
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, "", helpStyle.Render("esc back • r refresh • q quit"))
	return frame(lines...)
}
