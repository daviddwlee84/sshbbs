package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// PadRight pads s with spaces on the right so its display width equals w.
// If s is wider than w, it's truncated. Critical for table-like rendering
// with mixed ASCII + CJK strings — lipgloss's automatic padding miscounts
// double-width glyphs in some terminals.
func PadRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	cw := runewidth.StringWidth(s)
	if cw == w {
		return s
	}
	if cw > w {
		return runewidth.Truncate(s, w, "")
	}
	return s + strings.Repeat(" ", w-cw)
}

// PadLeft is PadRight's mirror, padding before the string.
func PadLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	cw := runewidth.StringWidth(s)
	if cw == w {
		return s
	}
	if cw > w {
		return runewidth.Truncate(s, w, "")
	}
	return strings.Repeat(" ", w-cw) + s
}

// Truncate cuts s to display-width w, appending "…" if truncation occurred.
func Truncate(s string, w int) string {
	if runewidth.StringWidth(s) <= w {
		return s
	}
	if w <= 1 {
		return runewidth.Truncate(s, w, "")
	}
	return runewidth.Truncate(s, w, "…")
}

// Width returns the display width of s.
func Width(s string) int { return runewidth.StringWidth(s) }
