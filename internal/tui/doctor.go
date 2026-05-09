package tui

import (
	"fmt"
	"sort"
	"strings"

	diagnostics "github.com/asudbring/loki-profile-manager/internal/doctor"
)

func (m Model) doctorView() string {
	lines := []string{
		titleStyle.Render("Doctor"),
		"",
	}
	if m.doctorErr != nil {
		lines = append(lines,
			errorStyle.Render("Doctor unavailable: "+m.doctorErr.Error()),
			"",
			helpStyle.Render("esc back • r refresh • q quit"),
		)
		return frame(lines...)
	}
	lines = append(lines,
		labelValue("Healthy", formatBool(m.doctor.Healthy)),
		labelValue("Summary", formatDoctorSummary(m.doctor)),
		labelValue("Runtime", runtimeSummary(m.doctor.Runtime.GOOS, m.doctor.Runtime.GOARCH)),
		labelValue("Store", firstNonEmpty(m.doctor.StorePath, "not configured")),
	)
	lines = appendDoctorChecks(lines, "Blocking", m.doctor.Checks, diagnostics.SeverityBlocking)
	lines = appendDoctorChecks(lines, "Warning", m.doctor.Checks, diagnostics.SeverityWarning)
	lines = appendDoctorChecks(lines, "Info", m.doctor.Checks, diagnostics.SeverityInfo)
	lines = append(lines, "", helpStyle.Render("esc back • r refresh • q quit"))
	return frame(lines...)
}

func appendDoctorChecks(lines []string, heading string, checks []diagnostics.Check, severity diagnostics.Severity) []string {
	filtered := []diagnostics.Check{}
	for _, check := range checks {
		if check.Severity == severity {
			filtered = append(filtered, check)
		}
	}
	if len(filtered) == 0 {
		return lines
	}
	lines = append(lines, "", subtitleStyle.Render(heading))
	for _, check := range filtered {
		line := fmt.Sprintf("- %s: %s", check.Code, check.Message)
		if check.Path != "" {
			line += " path=" + check.Path
		}
		if len(check.Details) > 0 {
			line += " details=" + formatDetails(check.Details)
		}
		if check.Remediation != "" {
			line += " fix=" + check.Remediation
		}
		lines = append(lines, line)
	}
	return lines
}

func runtimeSummary(goos, goarch string) string {
	if goos == "" && goarch == "" {
		return "unknown"
	}
	if goos == "" || goarch == "" {
		return firstNonEmpty(goos, goarch)
	}
	return goos + "/" + goarch
}

func formatDetails(details map[string]string) string {
	parts := make([]string, 0, len(details))
	for key, value := range details {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
