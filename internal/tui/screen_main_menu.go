package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	return mainMenuModel{
		deps: deps,
		items: []menuItem{
			{label: "看板列表 Boards", hint: "browse and read articles", to: ScreenBoardList},
			{label: "水球 Water Balloons", hint: "private messages with online users", to: ScreenWBInbox},
			{label: "離線 Quit", hint: "disconnect", to: -1},
		},
	}
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
		case "enter", " ":
			it := m.items[m.cursor]
			if it.to == -1 {
				return m, tea.Quit
			}
			return m, func() tea.Msg { return NavigateMsg{To: it.to} }
		case "1", "2", "3":
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
	nick := u.Nickname
	if nick == "" {
		nick = u.UserID
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(fmt.Sprintf(" SSH-BBS · %s (%s) ", nick, u.UserID)))
	b.WriteString("\n\n")

	if u.LastLoginAt.Valid {
		b.WriteString("  " + StyleDim.Render(fmt.Sprintf("上次登入 %s · 累計登入 %d 次 · 發文 %d 篇",
			u.LastLoginAt.Time.Format("2006-01-02 15:04"),
			u.NumLogins,
			u.NumPosts,
		)))
		b.WriteString("\n\n")
	}

	b.WriteString("  ")
	b.WriteString(StyleHeader.Render(" 主選單 Main Menu "))
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
	b.WriteString(StyleHelp.Render("↑/↓ or j/k to move · Enter to choose · 1-3 jump · q to quit"))
	b.WriteString("\n")
	return b.String()
}
