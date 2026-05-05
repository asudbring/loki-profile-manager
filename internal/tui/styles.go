package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	subtitleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	labelStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	menuStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedMenuStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
)
