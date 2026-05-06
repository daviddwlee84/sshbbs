package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/auth"
)

// Form-style screen reachable in two modes:
//
//   - Forced rotation: routed from Root when deps.MustChangePassword is set.
//     Esc disconnects (re-login lands on the same screen).
//   - Voluntary change: navigated from the user-settings menu. Esc returns
//     to the menu instead of quitting.
//
// Like screen_register, it deliberately does NOT bind h/l so those keys
// remain available for text editing inside the password fields.

const (
	pwFieldCurrent = iota
	pwFieldNew
	pwFieldConfirm
	pwFieldCount
)

type passwordChangeModel struct {
	deps    Deps
	inputs  []textinput.Model
	focus   int
	err     string
	success bool
}

func newPasswordChangeModel(deps Deps) passwordChangeModel {
	inputs := make([]textinput.Model, pwFieldCount)
	for i := range inputs {
		ti := textinput.New()
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
		ti.CharLimit = 128
		ti.Width = 32
		inputs[i] = ti
	}
	inputs[pwFieldCurrent].Placeholder = "目前密碼"
	inputs[pwFieldNew].Placeholder = "新密碼 (≥ 6)"
	inputs[pwFieldConfirm].Placeholder = "再次輸入新密碼"
	inputs[pwFieldCurrent].Focus()
	return passwordChangeModel{deps: deps, inputs: inputs}
}

func (m passwordChangeModel) Init() tea.Cmd { return textinput.Blink }

func (m passwordChangeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil

	case tea.KeyMsg:
		if m.success {
			// Voluntary change → back to the settings menu so the user
			// can carry on tweaking other things. Forced rotation → main
			// menu (the entry point post-rotation).
			to := ScreenMainMenu
			if !m.deps.MustChangePassword {
				to = ScreenUserSettings
			}
			return m, func() tea.Msg { return NavigateMsg{To: to} }
		}
		switch msg.String() {
		case "esc":
			// Voluntary mode (entered via 個人設定): Esc returns to the
			// settings menu. Forced-rotation mode: Esc disconnects — the
			// must_change flag is still set, so reconnecting just lands
			// back on this screen. Cleaner than letting the user limbo on
			// a screen they refused.
			if !m.deps.MustChangePassword {
				return m, func() tea.Msg { return NavigateMsg{To: ScreenUserSettings} }
			}
			return m, tea.Quit
		case "tab", "down":
			m.advance(+1)
			return m, nil
		case "shift+tab", "up":
			m.advance(-1)
			return m, nil
		case "enter":
			if m.focus < len(m.inputs)-1 {
				m.advance(+1)
				return m, nil
			}
			return m.submit()
		}
	}

	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m *passwordChangeModel) advance(d int) {
	n := len(m.inputs)
	m.focus = (m.focus + d + n) % n
	for i := range m.inputs {
		if i == m.focus {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m passwordChangeModel) submit() (tea.Model, tea.Cmd) {
	current := m.inputs[pwFieldCurrent].Value()
	newPw := m.inputs[pwFieldNew].Value()
	confirm := m.inputs[pwFieldConfirm].Value()

	if m.deps.User == nil {
		m.err = "internal error: no user"
		return m, nil
	}
	if err := auth.VerifyPasswordHash(m.deps.User.PasswordHash, current); err != nil {
		m.err = "目前密碼錯誤"
		return m, nil
	}
	if err := auth.ValidatePassword(newPw); err != nil {
		m.err = err.Error()
		return m, nil
	}
	if newPw != confirm {
		m.err = "新密碼與確認不一致"
		return m, nil
	}
	if newPw == current {
		m.err = "新密碼不可與目前密碼相同"
		return m, nil
	}

	hash, err := auth.HashPassword(newPw)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	ctx := context.Background()
	if err := m.deps.Store.Users().SetPassword(ctx, m.deps.User.ID, hash); err != nil {
		m.err = err.Error()
		return m, nil
	}
	// Refresh User in place so subsequent screens see the cleared flag.
	if fresh, err := m.deps.Store.Users().GetByID(ctx, m.deps.User.ID); err == nil {
		*m.deps.User = *fresh
	}
	m.success = true
	to := ScreenMainMenu
	if !m.deps.MustChangePassword {
		to = ScreenUserSettings
	}
	return m, func() tea.Msg { return NavigateMsg{To: to} }
}

func (m passwordChangeModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.deps.MustChangePassword {
		b.WriteString(StyleHeader.Render("  === 修改密碼 (首次登入必須修改) ==="))
	} else {
		b.WriteString(StyleHeader.Render("  === 修改密碼 Change password ==="))
	}
	b.WriteString("\n\n")

	labels := []string{"目前密碼 current", "新密碼 new (≥ 6)", "再次輸入 confirm"}
	for i, label := range labels {
		b.WriteString("  " + StyleDim.Render(label))
		b.WriteString("\n  ")
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n\n")
	}

	if m.err != "" {
		b.WriteString("  " + StyleError.Render("⚠ "+m.err) + "\n\n")
	}
	if m.success {
		if m.deps.MustChangePassword {
			b.WriteString("  " + StyleSuccess.Render("✓ 已更新，將進入主選單…") + "\n\n")
		} else {
			b.WriteString("  " + StyleSuccess.Render("✓ 已更新") + "\n\n")
		}
	}
	escHint := "Esc disconnect"
	if !m.deps.MustChangePassword {
		escHint = "Esc back"
	}
	b.WriteString("  " + StyleHelp.Render("Tab/↓ next · Shift+Tab/↑ prev · Enter submit · "+escHint))
	b.WriteString("\n")
	return b.String()
}
