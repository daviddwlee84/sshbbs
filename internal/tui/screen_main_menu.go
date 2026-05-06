package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

type menuItem struct {
	label string
	hint  string
	to    Screen
}

type mainMenuModel struct {
	deps   Deps
	items  []menuItem
	cursor int
}

func newMainMenuModel(deps Deps) mainMenuModel {
	loc := localeOf(deps)
	items := []menuItem{
		{label: i18n.T(loc, i18n.ScreenMainMenuItemBoards), hint: i18n.T(loc, i18n.ScreenMainMenuHintBoards), to: ScreenBoardList},
		{label: i18n.T(loc, i18n.ScreenMainMenuItemWB), hint: i18n.T(loc, i18n.ScreenMainMenuHintWB), to: ScreenWBInbox},
		{label: i18n.T(loc, i18n.ScreenMainMenuItemOnline), hint: i18n.T(loc, i18n.ScreenMainMenuHintOnline), to: ScreenOnline},
		{label: i18n.T(loc, i18n.ScreenMainMenuItemMail), hint: i18n.T(loc, i18n.ScreenMainMenuHintMail), to: ScreenMailInbox},
		{label: i18n.T(loc, i18n.ScreenMainMenuItemSettings), hint: i18n.T(loc, i18n.ScreenMainMenuHintSettings), to: ScreenUserSettings},
	}
	// Admin gets an extra entry; positioned before the Quit row so the
	// numeric shortcut for Quit always lands on the LAST item regardless
	// of role (the "5 quits" shortcut becomes "6 quits" for admin).
	if deps.User != nil && deps.User.Role == store.RoleAdmin {
		items = append(items, menuItem{
			label: i18n.T(loc, i18n.ScreenMainMenuItemAdmin),
			hint:  i18n.T(loc, i18n.ScreenMainMenuHintAdmin),
			to:    ScreenAdminUsers,
		})
	}
	items = append(items, menuItem{
		label: i18n.T(loc, i18n.ScreenMainMenuItemQuit),
		hint:  i18n.T(loc, i18n.ScreenMainMenuHintQuit),
		to:    -1,
	})
	return mainMenuModel{deps: deps, items: items}
}

func (m mainMenuModel) Init() tea.Cmd { return nil }

func (m mainMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			return m, nil
		case "enter", " ", "right", "l":
			it := m.items[m.cursor]
			if it.to == -1 {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return NavigateMsg{To: it.to} }
		case "1", "2", "3", "4", "5", "6", "7":
			idx := int(k.String()[0] - '1')
			if idx < len(m.items) {
				it := m.items[idx]
				if it.to == -1 {
					return m, tea.Quit
				}
				return m, func() tea.Msg { return NavigateMsg{To: it.to} }
			}
		}
	}
	return m, nil
}

func (m mainMenuModel) View() string {
	u := m.deps.User
	loc := localeOf(m.deps)
	nick := u.Nickname
	if nick == "" {
		nick = u.UserID
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(i18n.Tf(loc, i18n.ScreenMainMenuTitle, nick, u.UserID)))
	b.WriteString("\n\n")

	if u.LastLoginAt.Valid {
		b.WriteString("  " + StyleDim.Render(i18n.Tf(loc, i18n.ScreenMainMenuLastLoginLine,
			u.LastLoginAt.Time.Format("2006-01-02 15:04"),
			u.NumLogins,
			u.NumPosts,
		)))
		b.WriteString("\n\n")
	}

	b.WriteString("  ")
	b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenMainMenuMidHeader)))
	b.WriteString("\n\n")

	for i, it := range m.items {
		marker := "  "
		row := fmt.Sprintf("  %d. %s", i+1, it.label)
		if i == m.cursor {
			marker = "▸ "
			row = StyleHighlight.Render(fmt.Sprintf(" %d. %-32s ", i+1, it.label))
		}
		b.WriteString(marker + row)
		if it.hint != "" {
			b.WriteString("  " + StyleDim.Render(it.hint))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n  ")
	b.WriteString(StyleHelp.Render(i18n.Tf(loc, i18n.ScreenMainMenuHelpLine, len(m.items))))
	b.WriteString("\n")
	return b.String()
}
