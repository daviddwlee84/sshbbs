package tui

import (
	"regexp"
	"strings"
	"unicode/utf8"

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

// ansiCSIRe matches a CSI escape sequence: ESC '[' optional-params letter.
// SGR (color), cursor moves, and most other terminal control codes used by
// banners follow this shape. Other escape kinds (OSC, DCS) are left
// untouched — banners almost never carry them.
var ansiCSIRe = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

// stripCSI returns s with CSI escape sequences removed. Use it for any
// width / column calculation that involves banner content (or any other
// string that may carry inline color codes). The four helpers above
// intentionally do NOT call this — they're the article-list table path
// where input is plain text, and stripping every line would just be
// overhead.
func stripCSI(s string) string {
	return ansiCSIRe.ReplaceAllString(s, "")
}

// bannerVisualWidth returns the display width of s with CSI escapes
// excluded from the count.
func bannerVisualWidth(s string) int {
	return runewidth.StringWidth(stripCSI(s))
}

// bannerTruncate clips s to display-width w, preserving inline CSI runs
// (color codes are zero-width). On overflow the result ends with "…"
// (which counts toward the budget). If the input contained any CSI, the
// result is suffixed with `\x1b[0m` so a forgotten reset doesn't bleed
// into subsequent terminal output.
func bannerTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if bannerVisualWidth(s) <= w {
		return ensureANSIReset(s)
	}
	budget := w - 1 // reserve one cell for "…"
	var out strings.Builder
	visible := 0
	cursor := 0
	for _, loc := range ansiCSIRe.FindAllStringIndex(s, -1) {
		if !writeBodyUpToBudget(&out, s[cursor:loc[0]], &visible, budget) {
			out.WriteString("…")
			return ensureANSIReset(out.String())
		}
		out.WriteString(s[loc[0]:loc[1]])
		cursor = loc[1]
	}
	if !writeBodyUpToBudget(&out, s[cursor:], &visible, budget) {
		out.WriteString("…")
		return ensureANSIReset(out.String())
	}
	out.WriteString("…")
	return ensureANSIReset(out.String())
}

func writeBodyUpToBudget(out *strings.Builder, body string, visible *int, budget int) bool {
	for i := 0; i < len(body); {
		r, size := utf8.DecodeRuneInString(body[i:])
		rw := runewidth.RuneWidth(r)
		if *visible+rw > budget {
			return false
		}
		out.WriteRune(r)
		*visible += rw
		i += size
	}
	return true
}

func ensureANSIReset(s string) string {
	if strings.Contains(s, "\x1b[") && !strings.HasSuffix(s, "\x1b[0m") {
		return s + "\x1b[0m"
	}
	return s
}
