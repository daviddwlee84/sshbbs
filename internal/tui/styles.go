package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Force lipgloss's package-level renderer to a profile that supports
// background colors. The default renderer auto-detects from os.Stdout,
// which is wrong for a server: stdout might be redirected to a log file
// (e.g. `./sshbbs >server.log 2>&1 &`), making termenv pick `Ascii` and
// silently strip every Background() escape we render. The actual SSH
// session renders into the client's terminal (capable of TrueColor in
// every modern client), so committing to TrueColor here is safe and
// matches what wish's per-session renderer would pick anyway.
func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
}

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
