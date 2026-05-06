package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpEntry is one row in a screen's keymap. Keys is the displayed shortcut
// (e.g. "Enter/→/l"), Desc the human-readable action.
type HelpEntry struct {
	Keys string
	Desc string
}

// helpHeader is the optional title row shown above the keymap. Empty for
// most screens; some screens use it to clarify mode-specific keys.
type helpSection struct {
	Heading string
	Entries []HelpEntry
}

// screenHelp maps a Screen constant to the keys we want the help overlay
// to surface. Mirrors the inline help-line strings rendered at the bottom
// of each screen, with extra notes on globals (Ctrl+U, Ctrl+C, ?) so the
// overlay is self-contained.
//
// Form screens (compose, edit, password change, board-banner edit) are
// intentionally absent: they need to pass `?` through to textinput so the
// user can type it as a literal character. Their bottom help-line already
// surfaces every key they bind.
var screenHelp = map[Screen][]helpSection{
	ScreenMainMenu: {{
		Heading: "Main menu",
		Entries: []HelpEntry{
			{"↑/↓ j/k", "move cursor"},
			{"Enter/→/l", "select"},
			{"1-9", "jump to numbered slot"},
			{"q", "quit"},
		},
	}},
	ScreenBoardList: {{
		Heading: "Board list",
		Entries: []HelpEntry{
			{"↑/↓ j/k [/]", "move cursor"},
			{"Enter/→/l", "open board"},
			{"Esc/←/h", "back"},
		},
	}},
	ScreenBoardView: {{
		Heading: "Board view (article list)",
		Entries: []HelpEntry{
			{"↑/↓ j/k [/]", "move cursor"},
			{"Enter/→/l", "open article"},
			{"p", "post new article (non-guest)"},
			{"b", "board banner splash (if banner set)"},
			{"B", "edit banner (mod+)"},
			{"M", "pin / unpin (mod+)"},
			{"Esc/←/h", "back to board list"},
			{"Q", "back to main menu"},
		},
	}},
	ScreenArticleView: {{
		Heading: "Article view",
		Entries: []HelpEntry{
			{"j/k", "scroll"},
			{"g/G", "top / bottom"},
			{"PgUp/PgDn", "page scroll (b/space aliases)"},
			{"+ / - / =", "推 / 噓 / → (non-guest)"},
			{"p / P", "select push for delete"},
			{"D", "delete cursored article or push (owner / mod+)"},
			{"E", "edit article (author / mod+)"},
			{"y", "export article markdown"},
			{"[ / ]", "previous / next article on this board"},
			{"Esc/←/h", "back to board"},
			{"Q", "back to main menu"},
		},
	}},
	ScreenWBInbox: {{
		Heading: "水球 inbox (per-counterparty roll-up)",
		Entries: []HelpEntry{
			{"↑/↓ j/k", "move cursor"},
			{"Enter/→/l/r", "open thread with cursored user"},
			{"c", "compose new water balloon"},
			{"Esc/←/h", "back to main menu"},
		},
	}},
	ScreenWBThread: {{
		Heading: "水球 對話 (1-to-1 history)",
		Entries: []HelpEntry{
			{"↑/↓ j/k", "scroll"},
			{"g / G", "top / end"},
			{"c / r", "compose to this user"},
			{"Esc/←/h", "back to inbox"},
			{"Q", "back to main menu"},
		},
	}},
	ScreenOnline: {{
		Heading: "Online users",
		Entries: []HelpEntry{
			{"↑/↓ j/k [/]", "move cursor"},
			{"Enter/→/l", "丟水球 (compose)"},
			{"t", "對話 thread (chat-style)"},
			{"g / G / Home / End", "top / bottom"},
			{"Esc/←/h", "back to main menu"},
			{"Q", "back to main menu"},
		},
	}},
	ScreenMailInbox: {{
		Heading: "信箱 (mail inbox)",
		Entries: []HelpEntry{
			{"↑/↓ j/k [/]", "move cursor"},
			{"Enter/→/l", "open thread"},
			{"r", "reply to cursored mail"},
			{"c", "compose new mail"},
			{"Esc/←/h", "back to main menu"},
			{"Q", "back to main menu"},
		},
	}},
	ScreenMailThread: {{
		Heading: "信件 thread",
		Entries: []HelpEntry{
			{"↑/↓ j/k", "scroll"},
			{"g", "top"},
			{"r", "reply"},
			{"Esc/←/h", "back to inbox"},
			{"Q", "back to main menu"},
		},
	}},
	ScreenAdminUsers: {{
		Heading: "Admin: users",
		Entries: []HelpEntry{
			{"↑/↓ j/k", "move cursor"},
			{"[ / ]", "prev / next page"},
			{"a / m / u / g", "promote to admin / mod / user / guest"},
			{"R", "reset password (forces change on next login)"},
			{"Esc/←/h", "back to main menu"},
		},
	}},
	ScreenArticleExport: {{
		Heading: "Export article",
		Entries: []HelpEntry{
			{"1", "plain body"},
			{"2", "body + pushes"},
			{"3", "save to data/exports/<userid>/"},
			{"c", "OSC52 copy to clipboard"},
			{"Esc/←/h/q", "back"},
		},
	}},
	ScreenBoardSplash: {{
		Heading: "Board banner splash",
		Entries: []HelpEntry{
			{"any key", "return to board view"},
		},
	}},
}

// helpGlobals are the cross-screen keys shown on every help page so users
// don't need to discover them per screen.
var helpGlobals = []HelpEntry{
	{"?", "show this help (any key dismisses)"},
	{"Ctrl+U", "open 水球 inbox (logged-in only)"},
	{"Ctrl+C", "disconnect"},
}

// isFormScreen reports whether a screen is a text-entry form whose
// textinput / textarea must receive '?' as a literal character. The help
// overlay is suppressed on these screens; the bottom help-line each form
// already renders is the source of truth there.
func isFormScreen(s Screen) bool {
	switch s {
	case ScreenPostCompose,
		ScreenWBCompose,
		ScreenMailCompose,
		ScreenArticleEdit,
		ScreenBoardBannerEdit,
		ScreenPasswordChange:
		return true
	}
	return false
}

// helpAvailable reports whether the help overlay should respond to '?' on
// this screen. False for form screens and during register/MustChangePassword
// (those flows start before any Screen const is meaningful).
func helpAvailable(deps Deps, s Screen) bool {
	if deps.IsRegister || deps.MustChangePassword {
		return false
	}
	return !isFormScreen(s)
}

// helpBox is the lipgloss frame around the help body. Width is set per-call
// so the box hugs the rendered content rather than spanning the terminal.
var helpBox = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("63")).
	Padding(1, 2)

// renderHelp builds the full-screen help overlay for the given screen.
// Includes per-screen entries (or a fallback message) plus the globals.
func renderHelp(s Screen) string {
	var b strings.Builder
	b.WriteString(StyleHeader.Render(" 鍵盤捷徑說明 Help ") + "\n\n")

	sections, ok := screenHelp[s]
	if !ok {
		b.WriteString("  " + StyleDim.Render("(no per-screen keymap registered for this screen)") + "\n\n")
	} else {
		for i, sec := range sections {
			if i > 0 {
				b.WriteString("\n")
			}
			if sec.Heading != "" {
				b.WriteString(StyleDim.Render("  "+sec.Heading) + "\n")
			}
			for _, e := range sec.Entries {
				b.WriteString(formatHelpRow(e) + "\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(StyleDim.Render("  Global") + "\n")
	for _, e := range helpGlobals {
		b.WriteString(formatHelpRow(e) + "\n")
	}

	b.WriteString("\n  " + StyleHelp.Render("press any key to dismiss"))

	return helpBox.Render(b.String())
}

// formatHelpRow pads keys to a stable column width so the descriptions
// align — uses the runewidth-aware PadRight because keys often contain
// arrows / CJK glyphs.
func formatHelpRow(e HelpEntry) string {
	const keysW = 22
	return fmt.Sprintf("    %s  %s", PadRight(e.Keys, keysW), e.Desc)
}
