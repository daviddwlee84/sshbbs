package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// userSettingsModel is the "個人設定" sub-menu reached from the main menu.
// Three entries: change password, edit bio, edit notification settings.
// Layout and key handling mirror mainMenuModel deliberately so muscle memory
// transfers across BBS-internal menus.
type userSettingsModel struct {
	deps   Deps
	items  []menuItem
	cursor int
}

func newUserSettingsModel(deps Deps) userSettingsModel {
	items := []menuItem{
		{label: "修改密碼 Change password", hint: "current → new → confirm", to: ScreenPasswordChange},
		{label: "修改 Bio Edit bio", hint: "free-form profile blurb", to: ScreenBioEdit},
		{label: "通知設定 Notification settings", hint: "webhook targets + per-event toggles", to: ScreenNotifySettings},
		{label: "返回 Back", hint: "main menu", to: ScreenMainMenu},
	}
	return userSettingsModel{deps: deps, items: items}
}

func (m userSettingsModel) Init() tea.Cmd { return nil }

func (m userSettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "q", "left", "h", "backspace":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
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
			return m, func() tea.Msg { return NavigateMsg{To: it.to} }
		case "1", "2", "3", "4":
			idx := int(k.String()[0] - '1')
			if idx < len(m.items) {
				it := m.items[idx]
				return m, func() tea.Msg { return NavigateMsg{To: it.to} }
			}
		}
	}
	return m, nil
}

func (m userSettingsModel) View() string {
	u := m.deps.User
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(" 個人設定 User Settings "))
	b.WriteString("\n\n")

	if u != nil {
		b.WriteString("  " + StyleDim.Render(fmt.Sprintf("帳號 %s · 角色 %s", u.UserID, u.Role)))
		b.WriteString("\n")
		bio := strings.TrimSpace(u.Bio)
		if bio == "" {
			b.WriteString("  " + StyleDim.Render("(尚未填寫 bio)"))
		} else {
			// Show only the first line of bio in the header so the menu
			// itself stays compact; the bio-edit screen owns the full view.
			firstLine := bio
			if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
				firstLine = firstLine[:i]
			}
			b.WriteString("  " + StyleDim.Render("Bio: ") + Truncate(firstLine, 60))
		}
		b.WriteString("\n\n")
	}

	for i, it := range m.items {
		marker := "  "
		row := fmt.Sprintf("  %d. %s", i+1, it.label)
		if i == m.cursor {
			marker = "▸ "
			row = StyleHighlight.Render(fmt.Sprintf(" %d. %-40s ", i+1, it.label))
		}
		b.WriteString(marker + row)
		if it.hint != "" {
			b.WriteString("  " + StyleDim.Render(it.hint))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n  " + StyleHelp.Render("↑/↓ j/k move · Enter/→/l choose · 1-4 jump · Esc/←/h back"))
	b.WriteString("\n")
	return b.String()
}
