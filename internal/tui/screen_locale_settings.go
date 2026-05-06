package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
)

// localeSettingsModel is a two-row chooser for the user's UI locale.
// Reached from the "個人設定 / User settings" sub-menu. Mirrors
// notifySettingsModel's commit discipline: cursor moves are free, the
// chosen locale is staged in `selected` and only committed to the DB
// on Ctrl+S. Esc discards.
//
// Not a textinput form, so h/l navigation can be bound (left = back,
// right = pick the cursored option). Not added to isFormScreen so '?'
// still pops the help overlay on this screen.
type localeSettingsModel struct {
	deps     Deps
	options  []i18n.Locale
	cursor   int           // index into options, currently highlighted row
	current  i18n.Locale   // what's persisted in deps.User.Locale right now
	selected i18n.Locale   // what Ctrl+S would persist; equal to current until the user picks something else
	flash    string
	err      string
}

func newLocaleSettingsModel(deps Deps) localeSettingsModel {
	current := localeOf(deps)
	options := []i18n.Locale{i18n.LocaleZHTW, i18n.LocaleEN}
	cursor := 0
	for i, o := range options {
		if o == current {
			cursor = i
			break
		}
	}
	return localeSettingsModel{
		deps:     deps,
		options:  options,
		cursor:   cursor,
		current:  current,
		selected: current,
	}
}

func (m localeSettingsModel) Init() tea.Cmd { return nil }

func (m localeSettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "q", "left", "h", "backspace":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenUserSettings} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
			return m, nil
		case "enter", " ", "right", "l":
			m.selected = m.options[m.cursor]
			m.flash = ""
			m.err = ""
			return m, nil
		case "ctrl+s":
			return m.save()
		case "1", "2":
			idx := int(k.String()[0] - '1')
			if idx < len(m.options) {
				m.cursor = idx
				m.selected = m.options[idx]
				m.flash = ""
				m.err = ""
			}
			return m, nil
		}
	}
	return m, nil
}

func (m localeSettingsModel) save() (tea.Model, tea.Cmd) {
	if m.deps.User == nil {
		m.err = "internal error: no user"
		return m, nil
	}
	if !i18n.Valid(string(m.selected)) {
		m.err = fmt.Sprintf("invalid locale %q", m.selected)
		return m, nil
	}
	if m.selected == m.current {
		// Nothing to do; surface a quiet flash so the keystroke still has
		// visible effect.
		loc := localeOf(m.deps)
		m.flash = i18n.T(loc, i18n.ScreenLocaleSettingsFlashSaved)
		return m, nil
	}
	ctx := context.Background()
	if err := m.deps.Store.Users().SetLocale(ctx, m.deps.User.ID, string(m.selected)); err != nil {
		m.err = err.Error()
		return m, nil
	}
	// Refresh User in place so every other screen sharing this Deps
	// pointer renders in the new locale on the next navigate. Mirrors
	// screen_bio_edit.submit().
	if fresh, err := m.deps.Store.Users().GetByID(ctx, m.deps.User.ID); err == nil {
		*m.deps.User = *fresh
	}
	m.current = m.selected
	loc := localeOf(m.deps)
	m.flash = i18n.T(loc, i18n.ScreenLocaleSettingsFlashSaved)
	m.err = ""
	return m, nil
}

func (m localeSettingsModel) View() string {
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenLocaleSettingsTitle)))
	b.WriteString("\n\n")

	b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenLocaleSettingsIntro)))
	b.WriteString("\n\n")

	for i, o := range m.options {
		marker := "  "
		dot := "[ ]"
		if o == m.selected {
			dot = "[•]"
		}
		var label string
		switch o {
		case i18n.LocaleZHTW:
			label = i18n.T(loc, i18n.ScreenLocaleSettingsOptionZH)
		case i18n.LocaleEN:
			label = i18n.T(loc, i18n.ScreenLocaleSettingsOptionEN)
		default:
			label = string(o)
		}
		row := fmt.Sprintf("  %d. %s %s", i+1, dot, label)
		if i == m.cursor {
			marker = "▸ "
			row = StyleHighlight.Render(fmt.Sprintf(" %d. %s %-32s ", i+1, dot, label))
		}
		b.WriteString(marker + row + "\n")
	}

	if m.selected != m.current {
		b.WriteString("\n  " + StyleDim.Render(i18n.T(loc, i18n.ScreenLocaleSettingsDirty)))
	}
	b.WriteString("\n\n  " + StyleDim.Render(i18n.T(loc, i18n.ScreenLocaleSettingsNoteGlyphs)))

	if m.err != "" {
		b.WriteString("\n\n  " + StyleError.Render("⚠ "+m.err))
	}
	if m.flash != "" {
		b.WriteString("\n\n  " + StyleSuccess.Render(m.flash))
	}

	b.WriteString("\n\n  " + StyleHelp.Render(i18n.T(loc, i18n.ScreenLocaleSettingsHelpLine)))
	b.WriteString("\n")
	return b.String()
}
