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
	case ArticleAddedMsg:
		if m.board != nil && msg.BoardID == m.board.ID {
			ctx := context.Background()
			if arts, err := m.deps.Store.Articles().ListByBoard(ctx, m.board.ID, 100); err == nil {
				m.articles = arts
			}
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace", "left", "h":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardList} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k", "[":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j", "]":
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
		case "b":
			if m.board == nil || m.board.Banner == "" {
				return m, nil
			}
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenBoardSplash, BoardID: m.board.ID}
			}
		case "B":
			if m.board == nil || !m.canEditBanner() {
				return m, nil
			}
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenBoardBannerEdit, BoardID: m.board.ID}
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
	b.WriteString(m.renderBanner())

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		return b.String()
	}
	if len(m.articles) == 0 {
		hint := "(no articles yet — press p to write one)"
		help := "p post · Esc/←/h back · Ctrl+C disconnect"
		if m.isGuest() {
			hint = "(no articles yet)"
			help = "Esc/←/h back · Ctrl+C disconnect"
		}
		help = m.appendBannerHelp(help)
		b.WriteString("  " + StyleDim.Render(hint) + "\n")
		b.WriteString("\n  " + StyleHelp.Render(help))
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

	help := "↑/↓ j/k move · Enter/→/l open · p post · Esc/←/h back · Ctrl+C disconnect"
	if m.isGuest() {
		help = "↑/↓ j/k move · Enter/→/l open · Esc/←/h back · Ctrl+C disconnect"
	}
	help = m.appendBannerHelp(help)
	b.WriteString("\n  " + StyleHelp.Render(help))
	b.WriteString("\n")
	return b.String()
}

func (m boardViewModel) isGuest() bool {
	return m.deps.User != nil && m.deps.User.Role == store.RoleGuest
}

func (m boardViewModel) canEditBanner() bool {
	return m.deps.User != nil && m.deps.User.Role.AtLeast(store.RoleMod)
}

// appendBannerHelp tacks "b banner" / "B edit" hints onto a help-line
// only when relevant: `b` requires a non-empty banner; `B` requires mod+.
// Order matches the existing pattern (action keys before navigation).
func (m boardViewModel) appendBannerHelp(help string) string {
	if m.board != nil && m.board.Banner != "" {
		help += " · b banner"
	}
	if m.canEditBanner() {
		help += " · B edit"
	}
	return help
}

const maxInlineBannerLines = 8

// renderBanner produces the inline banner block shown above the article
// list. Empty string when the board has no banner. Each line is
// ANSI-aware-truncated to the viewport width with a 2-space left margin;
// the block is capped at maxInlineBannerLines with a "(press b)" hint
// when the source has more.
func (m boardViewModel) renderBanner() string {
	if m.board == nil || m.board.Banner == "" {
		return ""
	}
	width := m.width - 2
	if width < 20 {
		// Before the first WindowSizeMsg lands m.width is 0; fall back to a
		// generous default so the banner doesn't render as a single column.
		width = 78
	}
	lines := strings.Split(strings.TrimRight(m.board.Banner, "\n"), "\n")
	limit := maxInlineBannerLines
	truncated := false
	if len(lines) > limit {
		lines = lines[:limit]
		truncated = true
	}
	var out strings.Builder
	for _, line := range lines {
		out.WriteString("  ")
		out.WriteString(bannerTruncate(line, width))
		out.WriteString("\n")
	}
	if truncated {
		out.WriteString("  " + StyleDim.Render("…(banner truncated — press b for full view)") + "\n")
	}
	out.WriteString("\n")
	return out.String()
}
