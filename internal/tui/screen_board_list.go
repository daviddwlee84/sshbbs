package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

type boardListModel struct {
	deps    Deps
	boards  []*store.Board
	cursor  int
	width   int
	height  int
	loadErr error
}

func newBoardListModel(deps Deps) boardListModel {
	boards, err := deps.Store.Boards().List(context.Background())
	return boardListModel{deps: deps, boards: boards, loadErr: err}
}

func (m boardListModel) Init() tea.Cmd { return nil }

func (m boardListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "up", "k", "[":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j", "]":
			if m.cursor < len(m.boards)-1 {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			if len(m.boards) > 0 {
				m.cursor = len(m.boards) - 1
			}
		case "enter", " ", "right", "l":
			if len(m.boards) == 0 {
				return m, nil
			}
			b := m.boards[m.cursor]
			return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardView, BoardID: b.ID} }
		}
	}
	return m, nil
}

func (m boardListModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(" 看板列表 Boards "))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ load failed: "+m.loadErr.Error()) + "\n")
		return b.String()
	}
	if len(m.boards) == 0 {
		b.WriteString("  " + StyleDim.Render("(no boards yet)") + "\n")
		return b.String()
	}

	const (
		nameW    = 14
		titleW   = 32
	)
	header := fmt.Sprintf("  %s  %s  %s",
		PadRight("Name", nameW),
		PadRight("Title", titleW),
		"Description",
	)
	b.WriteString(StyleDim.Render(header) + "\n")

	for i, br := range m.boards {
		row := fmt.Sprintf("  %s  %s  %s",
			PadRight(br.Name, nameW),
			PadRight(br.Title, titleW),
			Truncate(br.Description, max(20, m.width-nameW-titleW-8)),
		)
		if i == m.cursor {
			b.WriteString(StyleHighlight.Render("▸"+row[1:]) + "\n")
		} else {
			b.WriteString(" " + row + "\n")
		}
	}

	b.WriteString("\n  " + StyleHelp.Render("↑/↓ j/k move · Enter/→/l open · Esc/←/h back · Ctrl+C disconnect"))
	b.WriteString("\n")
	return b.String()
}
