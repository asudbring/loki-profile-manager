package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allensu/loki-profile-manager/internal/app"
)

const (
	storeActionUse   = "use"
	storeActionInit  = "init"
	storeActionUnset = "unset"
)

type storeDiscoverMsg struct {
	result app.DiscoverStoresResult
	err    error
}

type storeActionMsg struct {
	action     string
	path       string
	message    string
	status     app.StoreStatusResult
	candidates app.DiscoverStoresResult
	err        error
}

func (m *Model) ensureStoreSelection() {
	if len(m.storeCandidates.Candidates) == 0 {
		m.storeCandidateIndex = 0
		return
	}
	if m.storeCandidateIndex < 0 || m.storeCandidateIndex >= len(m.storeCandidates.Candidates) {
		m.storeCandidateIndex = 0
	}
}

func (m Model) updateStoreKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.storeBusy {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	if m.storeConfirmAction != "" {
		return m.updateStoreConfirmKey(msg)
	}
	if m.storeManualMode {
		return m.updateStoreManualKey(msg)
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
		m.moveStoreCandidate(-1)
		return m, nil
	case "down", "j":
		m.moveStoreCandidate(1)
		return m, nil
	case "d":
		m.storeBusy = true
		m.storeDiscoverErr = nil
		return m, storeDiscoverCmd(m.ctx, m.client, app.DiscoverStoresRequest{})
	case "m":
		m.storeManualMode = true
		m.storeManualInput = ""
		m.storeActionErr = nil
		m.storeMessage = ""
		return m, nil
	case "u":
		if strings.TrimSpace(m.storeStatus.PersistedStorePath) == "" {
			m.storeActionErr = fmt.Errorf("store unset: no persisted store is configured")
			return m, nil
		}
		m.startStoreConfirm(storeActionUnset, m.storeStatus.PersistedStorePath)
		return m, nil
	case "enter":
		candidate, ok := m.selectedStoreCandidate()
		if !ok {
			m.storeActionErr = fmt.Errorf("store: no candidate selected")
			return m, nil
		}
		action, err := candidateStoreAction(candidate)
		if err != nil {
			m.storeActionErr = err
			return m, nil
		}
		m.startStoreConfirm(action, candidate.StorePath)
		return m, nil
	}
	return m, nil
}

func (m Model) updateStoreManualKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.storeManualMode = false
		m.storeManualInput = ""
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.storeManualInput)
		if path == "" {
			m.storeActionErr = fmt.Errorf("manual store path is required")
			return m, nil
		}
		m.storeBusy = true
		m.storeDiscoverErr = nil
		return m, storeDiscoverCmd(m.ctx, m.client, app.DiscoverStoresRequest{ManualPath: path})
	case "backspace", "ctrl+h":
		if len(m.storeManualInput) > 0 {
			m.storeManualInput = m.storeManualInput[:len(m.storeManualInput)-1]
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.storeManualInput += string(msg.Runes)
	}
	return m, nil
}

func (m Model) updateStoreConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.storeConfirmAction = ""
		m.storeConfirmPath = ""
		m.storeConfirmInput = ""
		m.storeConfirmErr = ""
		return m, nil
	case "enter":
		phrase := storeConfirmPhrase(m.storeConfirmAction)
		if m.storeConfirmInput != phrase {
			m.storeConfirmErr = fmt.Sprintf("confirmation mismatch; type %q", phrase)
			return m, nil
		}
		m.storeBusy = true
		m.storeConfirmErr = ""
		return m, storeActionCmd(m.ctx, m.client, m.storeConfirmAction, m.storeConfirmPath)
	case "backspace", "ctrl+h":
		if len(m.storeConfirmInput) > 0 {
			m.storeConfirmInput = m.storeConfirmInput[:len(m.storeConfirmInput)-1]
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.storeConfirmInput += string(msg.Runes)
	}
	return m, nil
}

func (m *Model) moveStoreCandidate(delta int) {
	if len(m.storeCandidates.Candidates) == 0 {
		m.storeCandidateIndex = 0
		return
	}
	m.storeCandidateIndex = (m.storeCandidateIndex + delta + len(m.storeCandidates.Candidates)) % len(m.storeCandidates.Candidates)
}

func (m Model) selectedStoreCandidate() (app.StoreCandidate, bool) {
	if len(m.storeCandidates.Candidates) == 0 || m.storeCandidateIndex < 0 || m.storeCandidateIndex >= len(m.storeCandidates.Candidates) {
		return app.StoreCandidate{}, false
	}
	return m.storeCandidates.Candidates[m.storeCandidateIndex], true
}

func (m *Model) startStoreConfirm(action, path string) {
	m.storeConfirmAction = action
	m.storeConfirmPath = path
	m.storeConfirmInput = ""
	m.storeConfirmErr = ""
	m.storeActionErr = nil
	m.storeMessage = ""
}

func candidateStoreAction(candidate app.StoreCandidate) (string, error) {
	if candidate.StoreValid {
		return storeActionUse, nil
	}
	if !candidate.StoreExists || candidate.StoreEmpty {
		return storeActionInit, nil
	}
	return "", fmt.Errorf("store candidate is invalid and non-empty; choose another path or fix it manually")
}

func storeConfirmPhrase(action string) string {
	switch action {
	case storeActionUse:
		return "USE STORE"
	case storeActionInit:
		return "INIT STORE"
	case storeActionUnset:
		return "UNSET STORE"
	default:
		return "CONFIRM"
	}
}

func storeDiscoverCmd(ctx context.Context, client Client, req app.DiscoverStoresRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := client.DiscoverStores(ctx, req)
		return storeDiscoverMsg{result: result, err: err}
	}
}

func storeActionCmd(ctx context.Context, client Client, action, path string) tea.Cmd {
	return func() tea.Msg {
		message := ""
		switch action {
		case storeActionUse:
			result, err := client.UseStore(ctx, app.UseStoreRequest{StorePath: path})
			if err != nil {
				return storeActionMsg{action: action, path: path, err: err}
			}
			if !result.Valid {
				return storeActionMsg{action: action, path: path, err: fmt.Errorf("store use: invalid store layout: missing %v", result.Missing)}
			}
			message = "Store configured: " + result.StorePath
		case storeActionInit:
			result, err := client.EnsureStore(ctx, app.EnsureStoreRequest{StorePath: path})
			if err != nil {
				return storeActionMsg{action: action, path: path, err: err}
			}
			if !result.Valid {
				return storeActionMsg{action: action, path: path, err: fmt.Errorf("store init: invalid store layout: missing %v", result.Missing)}
			}
			message = "Store initialized: " + result.StorePath
		case storeActionUnset:
			if _, err := client.ForgetStore(ctx, app.ForgetStoreRequest{}); err != nil {
				return storeActionMsg{action: action, path: path, err: err}
			}
			message = "Store configuration cleared."
		default:
			return storeActionMsg{action: action, path: path, err: fmt.Errorf("unknown store action %q", action)}
		}
		status, statusErr := client.StoreStatus(ctx)
		if statusErr != nil {
			return storeActionMsg{action: action, path: path, message: message, err: statusErr}
		}
		candidates, discoverErr := client.DiscoverStores(ctx, app.DiscoverStoresRequest{})
		if discoverErr != nil {
			return storeActionMsg{action: action, path: path, message: message, status: status, err: discoverErr}
		}
		return storeActionMsg{action: action, path: path, message: message, status: status, candidates: candidates}
	}
}

func (m Model) storeView() string {
	lines := []string{
		titleStyle.Render("Store"),
		"",
	}
	if m.storeStatusErr != nil {
		lines = append(lines, errorStyle.Render("Store status unavailable: "+m.storeStatusErr.Error()))
	}
	lines = append(lines,
		labelValue("Persisted", firstNonEmpty(m.storeStatus.PersistedStorePath, "not configured")),
		labelValue("Override", firstNonEmpty(m.storeStatus.StoreOverride, "none")),
		labelValue("Effective", firstNonEmpty(m.storeStatus.EffectiveStorePath, "not configured")),
		labelValue("Source", firstNonEmpty(m.storeStatus.EffectiveSource, "none")),
		labelValue("Valid", formatBool(m.storeStatus.Valid)),
	)
	if m.storeStatus.Message != "" {
		lines = append(lines, labelValue("Message", m.storeStatus.Message))
	}
	if m.storeMessage != "" {
		lines = append(lines, "", m.storeMessage)
	}
	if m.storeActionErr != nil {
		lines = append(lines, "", errorStyle.Render("Error: "+m.storeActionErr.Error()))
	}
	if m.storeBusy {
		lines = append(lines, "", mutedStyle.Render("Store operation running..."), "", helpStyle.Render("q quit"))
		return frame(lines...)
	}
	if m.storeConfirmAction != "" {
		phrase := storeConfirmPhrase(m.storeConfirmAction)
		lines = append(lines,
			"",
			subtitleStyle.Render("Confirm store action"),
			labelValue("Action", m.storeConfirmAction),
			labelValue("Path", m.storeConfirmPath),
			labelValue("Required phrase", phrase),
			labelValue("Input", m.storeConfirmInput),
		)
		if m.storeConfirmErr != "" {
			lines = append(lines, errorStyle.Render(m.storeConfirmErr))
		}
		lines = append(lines, "", helpStyle.Render("enter execute • esc cancel • q quit"))
		return frame(lines...)
	}
	if m.storeManualMode {
		lines = append(lines,
			"",
			subtitleStyle.Render("Manual store path"),
			labelValue("Path", m.storeManualInput),
			"",
			helpStyle.Render("type path • enter inspect • esc cancel • q quit"),
		)
		return frame(lines...)
	}
	lines = append(lines, "", subtitleStyle.Render("Candidates"))
	if m.storeDiscoverErr != nil {
		lines = append(lines, errorStyle.Render("Discovery failed: "+m.storeDiscoverErr.Error()))
	} else if len(m.storeCandidates.Candidates) == 0 {
		lines = append(lines, mutedStyle.Render("No OneDrive, Dropbox, or manual candidates found."))
	} else {
		for i, candidate := range m.storeCandidates.Candidates {
			cursor := "  "
			if i == m.storeCandidateIndex {
				cursor = "> "
			}
			status := "invalid"
			if candidate.StoreValid {
				status = "valid"
			} else if !candidate.StoreExists {
				status = "missing"
			} else if candidate.StoreEmpty {
				status = "empty"
			}
			lines = append(lines, fmt.Sprintf("%s%s %s [%s]", cursor, candidate.Provider, candidate.StorePath, status))
		}
	}
	lines = append(lines, "", helpStyle.Render("↑/↓ select • enter use/init • d rediscover • m manual • u unset • esc back • r refresh • q quit"))
	return frame(lines...)
}
