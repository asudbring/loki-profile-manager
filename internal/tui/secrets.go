package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asudbring/loki-profile-manager/internal/app"
)

const (
	secretsConfigureProjectIDIndex = iota
	secretsConfigureEnvironmentIndex
	secretsConfigureClientIDIndex
	secretsConfigureClientSecretIndex
	secretsConfigureHostURLIndex
)

var secretsConfigureFieldLabels = []string{
	"Project ID",
	"Environment",
	"Client ID",
	"Client secret/key",
	"Host/API URL",
}

type secretsConfigureMsg struct {
	result app.SecretsConfigureInfisicalResult
	err    error
}

func (m Model) updateSecretsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.secretsConfigureBusy {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	if m.secretsConfigure {
		return m.updateSecretsConfigureKey(msg)
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
	case "c":
		m.initSecretsConfigure()
		return m, nil
	}
	return m, nil
}

func (m Model) updateSecretsConfigureKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.secretsConfigure = false
		m.secretsConfigureErr = nil
		m.scrubSecretsConfigureInputs()
		return m, nil
	case "up":
		m.moveSecretsConfigureField(-1)
		return m, nil
	case "down":
		m.moveSecretsConfigureField(1)
		return m, nil
	case "enter":
		if m.secretsConfigureField == secretsConfigureHostURLIndex {
			m.secretsConfigureBusy = true
			m.secretsConfigureErr = nil
			m.secretsConfigureMessage = ""
			return m, secretsConfigureCmd(m.ctx, m.client, m.secretsConfigureRequest())
		}
		m.moveSecretsConfigureField(1)
		return m, nil
	case "ctrl+s":
		m.secretsConfigureBusy = true
		m.secretsConfigureErr = nil
		m.secretsConfigureMessage = ""
		return m, secretsConfigureCmd(m.ctx, m.client, m.secretsConfigureRequest())
	case "backspace", "ctrl+h":
		m.ensureSecretsConfigureInputs()
		value := m.secretsConfigureInputs[m.secretsConfigureField]
		if len(value) > 0 {
			m.secretsConfigureInputs[m.secretsConfigureField] = value[:len(value)-1]
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.ensureSecretsConfigureInputs()
		m.secretsConfigureInputs[m.secretsConfigureField] += string(msg.Runes)
	}
	return m, nil
}

func (m *Model) initSecretsConfigure() {
	m.secretsConfigure = true
	m.secretsConfigureField = 0
	m.secretsConfigureInputs = make([]string, len(secretsConfigureFieldLabels))
	m.secretsConfigureInputs[secretsConfigureEnvironmentIndex] = "dev"
	m.secretsConfigureErr = nil
	m.secretsConfigureMessage = ""
}

func (m *Model) ensureSecretsConfigureInputs() {
	if len(m.secretsConfigureInputs) == len(secretsConfigureFieldLabels) {
		return
	}
	inputs := make([]string, len(secretsConfigureFieldLabels))
	copy(inputs, m.secretsConfigureInputs)
	if strings.TrimSpace(inputs[secretsConfigureEnvironmentIndex]) == "" {
		inputs[secretsConfigureEnvironmentIndex] = "dev"
	}
	m.secretsConfigureInputs = inputs
	if m.secretsConfigureField < 0 || m.secretsConfigureField >= len(m.secretsConfigureInputs) {
		m.secretsConfigureField = 0
	}
}

func (m *Model) moveSecretsConfigureField(delta int) {
	m.ensureSecretsConfigureInputs()
	m.secretsConfigureField = (m.secretsConfigureField + delta + len(m.secretsConfigureInputs)) % len(m.secretsConfigureInputs)
}

func (m *Model) scrubSecretsConfigureInputs() {
	for i := range m.secretsConfigureInputs {
		m.secretsConfigureInputs[i] = ""
	}
	m.secretsConfigureInputs = nil
	m.secretsConfigureField = 0
}

func (m Model) secretsConfigureRequest() app.SecretsConfigureInfisicalRequest {
	m.ensureSecretsConfigureInputs()
	return app.SecretsConfigureInfisicalRequest{
		ProjectID:         strings.TrimSpace(m.secretsConfigureInputs[secretsConfigureProjectIDIndex]),
		Environment:       strings.TrimSpace(m.secretsConfigureInputs[secretsConfigureEnvironmentIndex]),
		ClientID:          strings.TrimSpace(m.secretsConfigureInputs[secretsConfigureClientIDIndex]),
		ClientSecret:      strings.TrimSpace(m.secretsConfigureInputs[secretsConfigureClientSecretIndex]),
		HostURL:           strings.TrimSpace(m.secretsConfigureInputs[secretsConfigureHostURLIndex]),
		OverwriteExisting: true,
		SkipVerify:        true,
	}
}

func secretsConfigureCmd(ctx context.Context, client Client, req app.SecretsConfigureInfisicalRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := client.SecretsConfigureInfisical(ctx, req)
		return secretsConfigureMsg{result: result, err: err}
	}
}

func (m Model) secretsView() string {
	if m.secretsConfigure {
		return m.secretsConfigureView()
	}
	lines := []string{
		titleStyle.Render("Secrets"),
		"",
	}
	if m.secretsErr != nil {
		lines = append(lines,
			errorStyle.Render("Secrets status unavailable: "+m.secretsErr.Error()),
		)
		if m.secretsConfigureMessage != "" {
			lines = append(lines, "", m.secretsConfigureMessage)
		}
		lines = append(lines, "", helpStyle.Render("c configure Infisical • esc back • r refresh • q quit"))
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
	if m.secretsConfigureMessage != "" {
		lines = append(lines, "", m.secretsConfigureMessage)
	}
	if m.secretsConfigureErr != nil {
		lines = append(lines, "", errorStyle.Render("Error: "+m.secretsConfigureErr.Error()))
	}
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
	lines = append(lines, "", helpStyle.Render("c configure Infisical • esc back • r refresh • q quit"))
	return frame(lines...)
}

func (m Model) secretsConfigureView() string {
	m.ensureSecretsConfigureInputs()
	lines := []string{
		titleStyle.Render("Secrets"),
		"",
		subtitleStyle.Render("Configure Infisical"),
		mutedStyle.Render("Validates Universal Auth, then writes the local Infisical env file. Secret field is masked."),
		"",
	}
	for i, label := range secretsConfigureFieldLabels {
		cursor := "  "
		if i == m.secretsConfigureField {
			cursor = "> "
		}
		value := m.secretsConfigureInputs[i]
		if i == secretsConfigureClientSecretIndex {
			value = strings.Repeat("•", len([]rune(value)))
		}
		if value == "" && i == secretsConfigureHostURLIndex {
			value = "optional"
		}
		lines = append(lines, fmt.Sprintf("%s%s: %s", cursor, label, value))
	}
	if m.secretsConfigureErr != nil {
		lines = append(lines, "", errorStyle.Render("Error: "+m.secretsConfigureErr.Error()))
	}
	if m.secretsConfigureBusy {
		lines = append(lines, "", mutedStyle.Render("Configuring Infisical..."), "", helpStyle.Render("q quit"))
		return frame(lines...)
	}
	lines = append(lines, "", helpStyle.Render("↑/↓ field • type edit • enter save on last field • ctrl+s save • esc cancel • q quit"))
	return frame(lines...)
}
