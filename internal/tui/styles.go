package tui

import "github.com/charmbracelet/lipgloss"

var (
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("63")).
			Padding(0, 1)

	StyleFooter = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	StyleHelp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	StyleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	StyleSuccess = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Bold(true)

	StyleHighlight = lipgloss.NewStyle().
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("60"))

	StyleDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	StylePushKind = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")).
			Bold(true)

	StyleBooKind = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	StyleArrowKind = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	StyleToast = lipgloss.NewStyle().
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("33")).
			Padding(0, 1).
			Bold(true)
)
