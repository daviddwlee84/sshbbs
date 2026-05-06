package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// glamourFallbackWidth is the wrap width used before a tea.WindowSizeMsg
// has set the real terminal width. 80 is the historical terminal default;
// any screen that caches a glamour render should re-render once the
// actual width arrives.
const glamourFallbackWidth = 80

// renderMarkdown runs body through glamour with word-wrap set to width.
// Returns the raw body unchanged on any glamour failure so a corrupt-
// markdown payload is still readable. Trims the leading/trailing blank
// lines glamour likes to add for breathing room — they push content off
// small viewports.
func renderMarkdown(body string, width int) string {
	if width < 40 {
		width = glamourFallbackWidth
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil {
		return body
	}
	return strings.Trim(out, "\n")
}
