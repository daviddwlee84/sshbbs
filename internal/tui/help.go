package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store"
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
// screenHelpFor returns the help-overlay sections for the given screen,
// resolved at the given locale. Most rows are universal English (key
// names + short verbs) and don't depend on locale; the few that mention
// 水球 / 信箱 / 推文量 / 推/噓/→ swap to their en counterparts via
// i18n.PushGlyph in en mode so the overlay reads consistently with the
// rest of the UI. Returns the empty slice (caller treats as "no entry")
// for unknown screens.
func screenHelpFor(loc i18n.Locale, s Screen) []helpSection {
	pushG := i18n.PushGlyph(loc, store.PushKindPush)
	booG := i18n.PushGlyph(loc, store.PushKindBoo)
	arrowG := i18n.PushGlyph(loc, store.PushKindArrow)

	wb := "水球"
	mailbox := "信箱"
	mailThread := "信件"
	chat := "對話"
	pushAmount := "推文量"
	whisper := "丟水球"
	if loc == i18n.LocaleEN {
		wb = "Water Balloon"
		mailbox = "Mail"
		mailThread = "Mail"
		chat = "chat"
		pushAmount = "score"
		whisper = "whisper"
	}

	switch s {
	case ScreenMainMenu:
		return []helpSection{{
			Heading: "Main menu",
			Entries: []HelpEntry{
				{"↑/↓ j/k", "move cursor"},
				{"Enter/→/l", "select"},
				{"1-9", "jump to numbered slot"},
				{"q", "quit"},
			},
		}}
	case ScreenBoardList:
		return []helpSection{{
			Heading: "Board list",
			Entries: []HelpEntry{
				{"↑/↓ j/k [/]", "move cursor"},
				{"Enter/→/l", "open board"},
				{"/", "search boards (Enter applies, Esc cancels)"},
				{"Esc (filtered)", "clear active filter"},
				{"Esc/←/h", "back"},
			},
		}}
	case ScreenBoardView:
		return []helpSection{{
			Heading: "Board view (article list)",
			Entries: []HelpEntry{
				{"↑/↓ j/k [/]", "move cursor"},
				{"Enter/→/l", "open article"},
				{"/", "search articles by title"},
				{"s", "toggle sort (Newest ↔ " + pushAmount + ")"},
				{"Esc (filtered)", "clear active filter"},
				{"p", "post new article (non-guest)"},
				{"b", "board banner splash (if banner set)"},
				{"B", "edit banner (mod+)"},
				{"M", "pin / unpin (mod+)"},
				{"Esc/←/h", "back to board list"},
				{"Q", "back to main menu"},
			},
		}}
	case ScreenArticleView:
		return []helpSection{{
			Heading: "Article view",
			Entries: []HelpEntry{
				{"j/k", "scroll"},
				{"g/G", "top / bottom"},
				{"PgUp/PgDn", "page scroll (b/space aliases)"},
				{"+ / - / =", pushG + " / " + booG + " / " + arrowG + " (non-guest)"},
				{"r", "reply (Re:) — opens compose with parent quoted"},
				{"p / P", "select push for delete"},
				{"D", "delete cursored article or push (owner / mod+)"},
				{"E", "edit article (author / mod+)"},
				{"y", "export article markdown"},
				{"[ / ]", "previous / next article on this board"},
				{"Esc/←/h", "back to board"},
				{"Q", "back to main menu"},
			},
		}}
	case ScreenWBInbox:
		return []helpSection{{
			Heading: wb + " inbox (per-counterparty roll-up)",
			Entries: []HelpEntry{
				{"↑/↓ j/k", "move cursor"},
				{"Enter/→/l/r", "open thread with cursored user"},
				{"c", "compose new water balloon"},
				{"Esc/←/h", "back to main menu"},
			},
		}}
	case ScreenWBThread:
		return []helpSection{
			{
				Heading: wb + " " + chat + " — input focused (default)",
				Entries: []HelpEntry{
					{"Enter", "send the typed message"},
					{"Tab", "switch focus to scrollback"},
					{"↑/↓ PgUp/PgDn", "scroll history while typing"},
					{"Esc", "back to inbox"},
					{"(any other key)", "typed into the input"},
				},
			},
			{
				Heading: "Scrollback focused (after Tab)",
				Entries: []HelpEntry{
					{"↑/↓ j/k PgUp/PgDn", "scroll"},
					{"g / G / Home / End", "top / end"},
					{"Tab", "back to input"},
					{"Esc/←/h", "back to inbox"},
					{"Q", "back to main menu"},
				},
			},
		}
	case ScreenOnline:
		return []helpSection{{
			Heading: "Online users",
			Entries: []HelpEntry{
				{"↑/↓ j/k [/]", "move cursor"},
				{"Enter/→/l", whisper + " (compose)"},
				{"t", chat + " thread (chat-style)"},
				{"g / G / Home / End", "top / bottom"},
				{"Esc/←/h", "back to main menu"},
				{"Q", "back to main menu"},
			},
		}}
	case ScreenMailInbox:
		return []helpSection{{
			Heading: mailbox + " (mail inbox)",
			Entries: []HelpEntry{
				{"↑/↓ j/k [/]", "move cursor"},
				{"Enter/→/l", "open thread"},
				{"r", "reply to cursored mail"},
				{"c", "compose new mail"},
				{"Esc/←/h", "back to main menu"},
				{"Q", "back to main menu"},
			},
		}}
	case ScreenMailThread:
		return []helpSection{{
			Heading: mailThread + " thread",
			Entries: []HelpEntry{
				{"↑/↓ j/k", "scroll"},
				{"g", "top"},
				{"r", "reply"},
				{"Esc/←/h", "back to inbox"},
				{"Q", "back to main menu"},
			},
		}}
	case ScreenUserSettings:
		return []helpSection{{
			Heading: "User settings",
			Entries: []HelpEntry{
				{"↑/↓ j/k", "move cursor"},
				{"Enter/→/l", "open sub-screen"},
				{"1-5", "jump to numbered slot"},
				{"Esc/←/h q", "back to main menu"},
			},
		}}
	case ScreenLocaleSettings:
		return []helpSection{{
			Heading: "Locale settings",
			Entries: []HelpEntry{
				{"↑/↓ j/k", "move cursor"},
				{"Enter/Space/→/l", "select highlighted locale"},
				{"1-2", "jump to numbered locale"},
				{"Ctrl+S", "save"},
				{"Esc/←/h", "back to user settings (discard unsaved)"},
				{"Q", "back to main menu"},
			},
		}}
	case ScreenNotifySettings:
		return []helpSection{{
			Heading: "Notification settings",
			Entries: []HelpEntry{
				{"↑/↓ j/k", "move cursor (toggles + targets + add)"},
				{"Space", "flip pref / toggle target enabled"},
				{"Ctrl+S", "save event toggles"},
				{"a", "add new webhook target"},
				{"e", "edit cursored target"},
				{"t", "toggle cursored target enabled"},
				{"T", "send test notification to cursored target"},
				{"d", "delete cursored target"},
				{"Esc/←/h", "back to user settings"},
				{"Q", "back to main menu"},
			},
		}}
	case ScreenAdminUsers:
		return []helpSection{{
			Heading: "Admin: users",
			Entries: []HelpEntry{
				{"↑/↓ j/k", "move cursor"},
				{"[ / ]", "prev / next page"},
				{"a / m / u / g", "promote to admin / mod / user / guest"},
				{"R", "reset password (forces change on next login)"},
				{"Esc/←/h", "back to main menu"},
			},
		}}
	case ScreenArticleExport:
		return []helpSection{{
			Heading: "Export article",
			Entries: []HelpEntry{
				{"1", "plain body"},
				{"2", "body + pushes"},
				{"3", "save to data/exports/<userid>/"},
				{"c", "OSC52 copy to clipboard"},
				{"Esc/←/h/q", "back"},
			},
		}}
	case ScreenBoardSplash:
		return []helpSection{{
			Heading: "Board banner splash",
			Entries: []HelpEntry{
				{"any key", "return to board view"},
			},
		}}
	}
	return nil
}

// helpGlobals are the cross-screen keys shown on every help page so users
// don't need to discover them per screen. The "open WB inbox" hint is
// the only locale-aware row — its label uses 水球 in zh-TW; en mode
// falls back to "Water Balloon" via i18n. Computed lazily by globalHelp
// so it picks up the active locale at render time.
func globalHelp(loc i18n.Locale) []HelpEntry {
	return []HelpEntry{
		{"?", "show this help (any key dismisses)"},
		{"Ctrl+U", i18n.T(loc, i18n.HelpGlobalWBInbox)},
		{"Ctrl+C", "disconnect"},
	}
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
		ScreenPasswordChange,
		ScreenBioEdit:
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
// loc localises the overlay title and the globals row that mentions
// 水球 (en mode renders "Water Balloon" instead).
func renderHelp(loc i18n.Locale, s Screen) string {
	var b strings.Builder
	b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.HelpOverlayTitle)) + "\n\n")

	sections := screenHelpFor(loc, s)
	if len(sections) == 0 {
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
	for _, e := range globalHelp(loc) {
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
