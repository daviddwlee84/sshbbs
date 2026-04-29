package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

type boardViewModel struct {
	deps     Deps
	board    *store.Board
	articles []*store.Article
	cursor   int
	width    int
	height   int
	loadErr  error
}

func newBoardViewModel(deps Deps, boardID int64) boardViewModel {
	ctx := context.Background()
	board, err := deps.Store.Boards().GetByID(ctx, boardID)
	if err != nil {
		return boardViewModel{deps: deps, loadErr: err}
	}
	articles, err := deps.Store.Articles().ListByBoard(ctx, boardID, 100)
	return boardViewModel{deps: deps, board: board, articles: articles, loadErr: err}
}

func (m boardViewModel) Init() tea.Cmd { return nil }

func (m boardViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace", "left", "h":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardList} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.articles)-1 {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			if len(m.articles) > 0 {
				m.cursor = len(m.articles) - 1
			}
		case "enter", " ", "right", "l":
			if len(m.articles) == 0 {
				return m, nil
			}
			a := m.articles[m.cursor]
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenArticleView, ArticleID: a.ID, BoardID: a.BoardID}
			}
		case "p":
			if m.board == nil {
				return m, nil
			}
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenPostCompose, BoardID: m.board.ID}
			}
		}
	}
	return m, nil
}

func (m boardViewModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.board != nil {
		b.WriteString(StyleHeader.Render(fmt.Sprintf(" 看板 %s · %s ", m.board.Name, m.board.Title)))
	} else {
		b.WriteString(StyleHeader.Render(" 看板 "))
	}
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		return b.String()
	}
	if len(m.articles) == 0 {
		b.WriteString("  " + StyleDim.Render("(no articles yet — press p to write one)") + "\n")
		b.WriteString("\n  " + StyleHelp.Render("p post · Esc/←/h back · Ctrl+C disconnect"))
		return b.String()
	}

	const (
		idxW    = 4
		dateW   = 11
		scoreW  = 4
		authorW = 14
	)
	titleW := max(20, m.width-2-idxW-1-dateW-1-scoreW-1-authorW-2)
	header := fmt.Sprintf(" %s  %s  %s  %s  %s",
		PadRight("#", idxW),
		PadRight("Date", dateW),
		PadRight("Sc.", scoreW),
		PadRight("Author", authorW),
		PadRight("Title", titleW),
	)
	b.WriteString(StyleDim.Render(header) + "\n")

	for i, a := range m.articles {
		score := ""
		switch {
		case a.RecommendScore >= 100:
			score = "爆"
		case a.RecommendScore > 0:
			score = fmt.Sprintf("%d", a.RecommendScore)
		case a.RecommendScore < 0:
			score = "X" + fmt.Sprintf("%d", -a.RecommendScore)
		}
		row := fmt.Sprintf(" %s  %s  %s  %s  %s",
			PadRight(fmt.Sprintf("%d", i+1), idxW),
			PadRight(a.CreatedAt.Format("01/02 15:04"), dateW),
			PadRight(score, scoreW),
			PadRight(a.AuthorUserID, authorW),
			Truncate(a.Title, titleW),
		)
		if i == m.cursor {
			b.WriteString(StyleHighlight.Render("▸"+row[1:]) + "\n")
		} else {
			b.WriteString(" " + row + "\n")
		}
	}

	b.WriteString("\n  " + StyleHelp.Render("↑/↓ j/k move · Enter/→/l open · p post · Esc/←/h back · Ctrl+C disconnect"))
	b.WriteString("\n")
	return b.String()
}
