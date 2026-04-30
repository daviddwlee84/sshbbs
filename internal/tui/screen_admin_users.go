package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

const adminUsersPageSize = 20

type adminUsersModel struct {
	deps    Deps
	users   []*store.User
	cursor  int
	page    int
	loadErr error
	toast   string
	width   int
	height  int
}

func newAdminUsersModel(deps Deps) adminUsersModel {
	m := adminUsersModel{deps: deps}
	m.reload()
	return m
}

func (m *adminUsersModel) reload() {
	users, err := m.deps.Store.Users().ListAll(context.Background(), adminUsersPageSize, m.page*adminUsersPageSize)
	m.users = users
	m.loadErr = err
	if m.cursor >= len(users) {
		m.cursor = max(0, len(users)-1)
	}
}

func (m adminUsersModel) Init() tea.Cmd { return nil }

func (m adminUsersModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace", "left", "h":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.users)-1 {
				m.cursor++
			}
		case "[":
			if m.page > 0 {
				m.page--
				m.cursor = 0
				m.reload()
			}
		case "]":
			// Try to advance; if reload comes back empty, snap back.
			m.page++
			m.cursor = 0
			m.reload()
			if len(m.users) == 0 && m.page > 0 {
				m.page--
				m.reload()
			}
		case "g":
			return m.applyRole(store.RoleGuest)
		case "u":
			return m.applyRole(store.RoleUser)
		case "M":
			return m.applyRole(store.RoleMod)
		case "a":
			return m.applyRole(store.RoleAdmin)
		}
	}
	return m, nil
}

func (m adminUsersModel) applyRole(target store.Role) (tea.Model, tea.Cmd) {
	if len(m.users) == 0 || m.cursor < 0 || m.cursor >= len(m.users) {
		return m, nil
	}
	u := m.users[m.cursor]
	if u.Role == target {
		m.toast = fmt.Sprintf("%s 已是 %s", u.UserID, target)
		return m, nil
	}
	// Last-admin guard: refuse to demote the only remaining admin.
	if u.Role == store.RoleAdmin && target != store.RoleAdmin {
		count, err := m.deps.Store.Users().CountByRole(context.Background(), store.RoleAdmin)
		if err != nil {
			m.toast = err.Error()
			return m, nil
		}
		if count <= 1 {
			m.toast = "無法解除最後一名管理員 (cannot demote the last admin)"
			return m, nil
		}
	}
	if err := m.deps.Store.Users().SetRole(context.Background(), u.ID, target); err != nil {
		if errors.Is(err, store.ErrInvalidRole) {
			m.toast = "invalid role"
		} else {
			m.toast = err.Error()
		}
		return m, nil
	}
	m.toast = fmt.Sprintf("%s → %s", u.UserID, target)
	m.reload()
	return m, nil
}

func (m adminUsersModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(" 使用者管理 Admin Users "))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		return b.String()
	}
	if len(m.users) == 0 {
		b.WriteString("  " + StyleDim.Render("(no users on this page)") + "\n")
		b.WriteString("\n  " + StyleHelp.Render("[ prev page · Esc back"))
		return b.String()
	}

	const (
		idW       = 5
		userIDW   = 14
		roleW     = 6
		flagW     = 12
		nicknameW = 16
	)
	header := fmt.Sprintf(" %s  %s  %s  %s  %s",
		PadRight("id", idW),
		PadRight("user_id", userIDW),
		PadRight("role", roleW),
		PadRight("must_chg_pw", flagW),
		PadRight("nickname", nicknameW),
	)
	b.WriteString(StyleDim.Render(header) + "\n")

	for i, u := range m.users {
		flag := ""
		if u.MustChangePassword {
			flag = "yes"
		}
		row := fmt.Sprintf(" %s  %s  %s  %s  %s",
			PadRight(fmt.Sprintf("%d", u.ID), idW),
			PadRight(u.UserID, userIDW),
			PadRight(string(u.Role), roleW),
			PadRight(flag, flagW),
			PadRight(Truncate(u.Nickname, nicknameW), nicknameW),
		)
		if i == m.cursor {
			b.WriteString(StyleHighlight.Render("▸"+row[1:]) + "\n")
		} else {
			b.WriteString(" " + row + "\n")
		}
	}

	b.WriteString("\n  " + StyleDim.Render(fmt.Sprintf("page %d", m.page+1)) + "\n")
	if m.toast != "" {
		b.WriteString("  " + StyleHighlight.Render(m.toast) + "\n")
	}
	b.WriteString("\n  " + StyleHelp.Render(
		"j/k move · g/u/M/a set role (guest/user/Mod/admin) · [/] page · Q/Esc back",
	))
	b.WriteString("\n")
	return b.String()
}
