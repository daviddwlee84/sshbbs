package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/auth"
)

const (
	regFieldUserID = iota
	regFieldPassword
	regFieldNickname
	regFieldEmail
	regFieldCount
)

type registerModel struct {
	deps    Deps
	inputs  []textinput.Model
	focus   int
	err     string
	success bool
	done    string // success: registered userid
}

func newRegisterModel(deps Deps) registerModel {
	inputs := make([]textinput.Model, regFieldCount)

	inputs[regFieldUserID] = textinput.New()
	inputs[regFieldUserID].Placeholder = "alice"
	inputs[regFieldUserID].CharLimit = 12
	inputs[regFieldUserID].Width = 32
	inputs[regFieldUserID].Focus()

	inputs[regFieldPassword] = textinput.New()
	inputs[regFieldPassword].Placeholder = "≥ 6 characters"
	inputs[regFieldPassword].EchoMode = textinput.EchoPassword
	inputs[regFieldPassword].EchoCharacter = '•'
	inputs[regFieldPassword].CharLimit = 128
	inputs[regFieldPassword].Width = 32

	inputs[regFieldNickname] = textinput.New()
	inputs[regFieldNickname].Placeholder = "愛麗絲"
	inputs[regFieldNickname].CharLimit = 64
	inputs[regFieldNickname].Width = 40

	inputs[regFieldEmail] = textinput.New()
	inputs[regFieldEmail].Placeholder = "alice@example.com"
	inputs[regFieldEmail].CharLimit = 128
	inputs[regFieldEmail].Width = 40

	return registerModel{deps: deps, inputs: inputs}
}

func (m registerModel) Init() tea.Cmd { return textinput.Blink }

func (m registerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil

	case tea.KeyMsg:
		if m.success {
			return m, tea.Quit
		}
		switch msg.String() {
		case "esc":
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

func (m *registerModel) advance(d int) {
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

func (m registerModel) submit() (tea.Model, tea.Cmd) {
	user, err := auth.Register(context.Background(), m.deps.Store,
		m.inputs[regFieldUserID].Value(),
		m.inputs[regFieldPassword].Value(),
		m.inputs[regFieldNickname].Value(),
		m.inputs[regFieldEmail].Value(),
	)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.success = true
	m.done = user.UserID
	return m, nil
}

func (m registerModel) View() string {
	if m.success {
		return StyleSuccess.Render("\n  ✓ 註冊成功！") +
			"\n\n  請重新連線：\n\n    " +
			StyleHighlight.Render(fmt.Sprintf("ssh %s@<host> -p <port>", m.done)) +
			"\n\n  按任意鍵結束。\n"
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render("  === 註冊新帳號 ==="))
	b.WriteString("\n\n")

	labels := []string{
		"帳號 user id (3-12, 字母開頭, 英數+底線)",
		"密碼 password (≥ 6)",
		"暱稱 nickname",
		"Email (optional)",
	}
	for i, label := range labels {
		b.WriteString("  " + StyleDim.Render(label))
		b.WriteString("\n  ")
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n\n")
	}

	if m.err != "" {
		b.WriteString("  " + StyleError.Render("⚠ "+m.err) + "\n\n")
	}
	b.WriteString("  " + StyleHelp.Render("Tab/↓ next · Shift+Tab/↑ prev · Enter submit · Esc/Ctrl+C cancel") + "\n")
	return b.String()
}
