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
	case ArticlePinChangedMsg:
		if m.board != nil && msg.BoardID == m.board.ID {
			// Pin re-orders the list. Re-anchor the cursor by article ID so
			// the same article stays highlighted across the reflow.
			anchorID := int64(0)
			if m.cursor < len(m.articles) {
				anchorID = m.articles[m.cursor].ID
			}
			ctx := context.Background()
			if arts, err := m.deps.Store.Articles().ListByBoard(ctx, m.board.ID, 100); err == nil {
				m.articles = arts
				m.cursor = findCursorByID(arts, anchorID)
			}
		}
		return m, nil
	case ArticleCommentsModeChangedMsg:
		if m.board != nil && msg.BoardID == m.board.ID {
			// Mode flip doesn't reorder, but we still re-anchor the cursor
			// for parity with the pin handler — if a future change adds
			// list ordering by comments_mode, we won't lose the highlight.
			anchorID := int64(0)
			if m.cursor < len(m.articles) {
				anchorID = m.articles[m.cursor].ID
			}
			ctx := context.Background()
			if arts, err := m.deps.Store.Articles().ListByBoard(ctx, m.board.ID, 100); err == nil {
				m.articles = arts
				m.cursor = findCursorByID(arts, anchorID)
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
		case "M":
			// Toggle pin on the cursor article. Mod+ only — silent no-op
			// otherwise so non-mods don't get a confusing error toast for
			// trying a key the help line doesn't even mention to them.
			if m.board == nil || !m.canPin() || len(m.articles) == 0 {
				return m, nil
			}
			a := m.articles[m.cursor]
			pinned := !a.PinnedAt.Valid // toggle
			ctx := context.Background()
			u := m.deps.User
			if err := m.deps.Store.Articles().SetPinned(ctx, a.ID, u.ID, u.Role, pinned); err != nil {
				return m, nil
			}
			if arts, err := m.deps.Store.Articles().ListByBoard(ctx, m.board.ID, 100); err == nil {
				m.articles = arts
				m.cursor = findCursorByID(arts, a.ID)
			}
			if m.deps.Broker != nil {
				m.deps.Broker.SendToAll(u.ID, ArticlePinChangedMsg{
					BoardID: m.board.ID, ArticleID: a.ID, Pinned: pinned,
				})
			}
			return m, nil
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
		// Pinned (板規 / 置頂) articles surface a leading "[M] " marker —
		// the same glyph PTT uses for M-marked posts. Locked / arrows-only
		// articles stack additional [鎖] / [箭] badges so users see the
		// constraint before opening the article. Long titles still get
		// truncated by Truncate.
		title := a.Title
		var prefix string
		if a.PinnedAt.Valid {
			prefix += "[M]"
		}
		switch a.CommentsMode {
		case store.CommentsModeLocked:
			prefix += "[鎖]"
		case store.CommentsModeArrowsOnly:
			prefix += "[箭]"
		}
		if prefix != "" {
			title = prefix + " " + a.Title
		}
		row := fmt.Sprintf(" %s  %s  %s  %s  %s",
			PadRight(fmt.Sprintf("%d", i+1), idxW),
			PadRight(a.CreatedAt.Format("01/02 15:04"), dateW),
			PadRight(score, scoreW),
			PadRight(a.AuthorUserID, authorW),
			Truncate(title, titleW),
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

// canPin gates the M shortcut. Mirrors canEditBanner — pinning is a
// moderation action and intentionally does NOT admit the article author
// (an author shouldn't bubble their own post to the top).
func (m boardViewModel) canPin() bool {
	return m.deps.User != nil && m.deps.User.Role.AtLeast(store.RoleMod)
}

// findCursorByID returns the index of the article with the given ID in
// the slice, or 0 if not found. Used to re-anchor the cursor after a
// pin/unpin reorders the list.
func findCursorByID(arts []*store.Article, id int64) int {
	for i, a := range arts {
		if a.ID == id {
			return i
		}
	}
	return 0
}

// appendBannerHelp tacks "b banner" / "B edit" / "M pin" hints onto a
// help-line only when relevant: `b` requires a non-empty banner; `B` and
// `M` require mod+. Order matches the existing pattern (action keys
// before navigation). Name kept for git-blame stability — it now covers
// all mod-affordances on the board view, not just banner ones.
func (m boardViewModel) appendBannerHelp(help string) string {
	if m.board != nil && m.board.Banner != "" {
		help += " · b banner"
	}
	if m.canEditBanner() {
		help += " · B edit"
	}
	if m.canPin() {
		help += " · M pin/unpin"
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
