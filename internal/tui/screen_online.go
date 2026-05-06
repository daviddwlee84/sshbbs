package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
)

type onlineModel struct {
	deps    Deps
	users   []chat.OnlineUser
	cursor  int
	width   int
	height  int
}

func newOnlineModel(deps Deps) onlineModel {
	var list []chat.OnlineUser
	if deps.Broker != nil {
		list = deps.Broker.OnlineList()
	}
	// Stable order: by user_id ascending so the same login is in the same
	// row each time the screen is opened. The broker's map iteration is
	// otherwise non-deterministic.
	sort.Slice(list, func(i, j int) bool { return list[i].UserID < list[j].UserID })
	return onlineModel{deps: deps, users: list}
}

func (m onlineModel) Init() tea.Cmd { return nil }

func (m onlineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace", "left", "h", "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k", "[":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j", "]":
			if m.cursor < len(m.users)-1 {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			if len(m.users) > 0 {
				m.cursor = len(m.users) - 1
			}
		case "enter", " ", "right", "l":
			if len(m.users) == 0 {
				return m, nil
			}
			u := m.users[m.cursor]
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenWBCompose, Recipient: u.UserIDStr}
			}
		case "t":
			// Open the 1-to-1 conversation thread with the cursored user.
			// Compose stays on Enter; t is the chat-style entry point.
			if len(m.users) == 0 {
				return m, nil
			}
			u := m.users[m.cursor]
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenWBThread, CounterpartyUserID: u.UserID}
			}
		}
	}
	return m, nil
}

func (m onlineModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(fmt.Sprintf(" 線上使用者 Online (%d) ", len(m.users))))
	b.WriteString("\n\n")

	if len(m.users) == 0 {
		b.WriteString("  " + StyleDim.Render("(nobody else online)") + "\n")
		b.WriteString("\n  " + StyleHelp.Render("Esc/←/h back · Q quit to menu"))
		return b.String()
	}

	const (
		userW = 14
		sessW = 10
	)
	header := fmt.Sprintf("  %s  %s",
		PadRight("UserID", userW),
		PadRight("Sessions", sessW),
	)
	b.WriteString(StyleDim.Render(header) + "\n")

	for i, u := range m.users {
		row := fmt.Sprintf("  %s  %s",
			PadRight(u.UserIDStr, userW),
			PadRight(fmt.Sprintf("%d", u.Sessions), sessW),
		)
		if i == m.cursor {
			b.WriteString(StyleHighlight.Render("▸"+row[1:]) + "\n")
		} else {
			b.WriteString(" " + row + "\n")
		}
	}

	b.WriteString("\n  " + StyleHelp.Render("↑/↓ j/k move · Enter/→/l 丟水球 · t 對話 thread · Esc/←/h back · Q quit"))
	b.WriteString("\n")
	return b.String()
}
