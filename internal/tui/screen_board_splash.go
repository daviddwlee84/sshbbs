package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

// boardSplashModel renders a board's banner full-screen. It's reached by
// pressing `b` from the board view; any key returns. No edit / scroll —
// banners that overflow the viewport rely on terminal scrollback (we
// don't try to paginate because banners are typically short, and adding
// scroll machinery clashes with "any key returns").
type boardSplashModel struct {
	deps    Deps
	board   *store.Board
	boardID int64
	width   int
	height  int
	loadErr error
}

func newBoardSplashModel(deps Deps, boardID int64) boardSplashModel {
	board, err := deps.Store.Boards().GetByID(context.Background(), boardID)
	if err != nil {
		return boardSplashModel{deps: deps, boardID: boardID, loadErr: err}
	}
	return boardSplashModel{deps: deps, board: board, boardID: boardID}
}

func (m boardSplashModel) Init() tea.Cmd { return nil }

func (m boardSplashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		_ = msg
		// Any key returns to the board view. The board id is preserved
		// even if loadErr — better than dumping the user back to the
		// board list when the only thing that failed was a banner re-fetch.
		return m, func() tea.Msg {
			return NavigateMsg{To: ScreenBoardView, BoardID: m.boardID}
		}
	}
	return m, nil
}

func (m boardSplashModel) View() string {
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	if m.board != nil {
		b.WriteString(StyleHeader.Render(i18n.Tf(loc, i18n.ScreenBoardSplashTitleNamed, m.board.Name)))
	} else {
		b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenBoardSplashTitleBare)))
	}
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		b.WriteString("\n  " + StyleHelp.Render("press any key to return"))
		return b.String()
	}
	if m.board == nil || m.board.Banner == "" {
		b.WriteString("  " + StyleDim.Render("(this board has no banner yet)") + "\n")
		b.WriteString("\n  " + StyleHelp.Render("press any key to return"))
		return b.String()
	}

	width := m.width - 2
	if width < 20 {
		width = 78
	}
	for _, line := range strings.Split(strings.TrimRight(m.board.Banner, "\n"), "\n") {
		b.WriteString("  ")
		b.WriteString(bannerTruncate(line, width))
		b.WriteString("\n")
	}

	b.WriteString("\n  " + StyleHelp.Render("press any key to return · Esc/Enter/q also work"))
	b.WriteString("\n")
	return b.String()
}
