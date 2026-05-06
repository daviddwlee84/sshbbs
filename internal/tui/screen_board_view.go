package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

type boardViewModel struct {
	deps         Deps
	board        *store.Board
	articles     []*store.Article
	cursor       int
	width        int
	height       int
	loadErr      error
	search       textinput.Model   // input for the active search term
	searchActive bool              // true while focused & accepting keys
	filter       string            // confirmed title substring; "" = no filter
	sort         store.ArticleSort // SortNewestFirst (default) or SortByScoreDesc
}

func newBoardViewModel(deps Deps, boardID int64) boardViewModel {
	ctx := context.Background()
	board, err := deps.Store.Boards().GetByID(ctx, boardID)
	if err != nil {
		return boardViewModel{deps: deps, loadErr: err}
	}
	ti := textinput.New()
	ti.Placeholder = i18n.T(localeOf(deps), i18n.ScreenBoardViewSearchPlaceholder)
	ti.CharLimit = 64
	ti.Width = 50
	m := boardViewModel{deps: deps, board: board, search: ti}
	return m.reload(0)
}

func (m boardViewModel) Init() tea.Cmd { return nil }

// reload re-fetches articles using the current m.filter and m.sort and
// returns the updated model. anchorID > 0 keeps the cursor on the same
// article ID across the reflow; anchorID == 0 leaves the cursor at its
// numeric index (clamped to the new length). DB errors after the initial
// load are silent so a transient broadcast-driven refresh doesn't blank
// the screen — matches the pre-refactor behaviour of the inline reloads.
func (m boardViewModel) reload(anchorID int64) boardViewModel {
	if m.board == nil {
		return m
	}
	ctx := context.Background()
	arts, err := m.deps.Store.Articles().ListByBoardOpts(ctx, m.board.ID,
		store.ListArticlesOpts{Limit: 100, TitleSearch: m.filter, Sort: m.sort})
	if err != nil {
		if m.loadErr == nil {
			m.loadErr = err
		}
		return m
	}
	m.articles = arts
	if anchorID > 0 {
		m.cursor = findCursorByID(arts, anchorID)
	}
	if m.cursor >= len(arts) {
		m.cursor = max(0, len(arts)-1)
	}
	return m
}

// currentAnchorID returns the cursored article's ID, or 0 if the list is
// empty. Used to preserve the highlight across reloads triggered by sort
// toggles or pin/mode broadcasts.
func (m boardViewModel) currentAnchorID() int64 {
	if m.cursor < len(m.articles) {
		return m.articles[m.cursor].ID
	}
	return 0
}

func (m boardViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = max(20, msg.Width-12)
		return m, nil
	case ArticleAddedMsg:
		if m.board != nil && msg.BoardID == m.board.ID {
			// No anchor — preserve cursor index. Matches pre-refactor
			// behaviour: a freshly added article shifts the cursor to
			// what was previously one row below it (in newest-first sort).
			m = m.reload(0)
		}
		return m, nil
	case ArticlePinChangedMsg:
		if m.board != nil && msg.BoardID == m.board.ID {
			// Pin re-orders the list — re-anchor the cursor by article ID
			// so the same article stays highlighted across the reflow.
			m = m.reload(m.currentAnchorID())
		}
		return m, nil
	case ArticleCommentsModeChangedMsg:
		if m.board != nil && msg.BoardID == m.board.ID {
			m = m.reload(m.currentAnchorID())
		}
		return m, nil
	case tea.KeyMsg:
		// While the search input is focused every printable key types into
		// it; only enter (confirm), esc (cancel), and ctrl+c (handled at
		// Root) escape that. Action keys (s/M/p/b/B/Q) and vim navigation
		// (h/j/k/l/g/G/[/]) all become text characters during search.
		if m.searchActive {
			switch msg.String() {
			case "enter":
				m.filter = strings.TrimSpace(m.search.Value())
				m.searchActive = false
				m.search.Blur()
				m.cursor = 0
				m = m.reload(0)
				return m, nil
			case "esc":
				m.search.SetValue("")
				m.filter = ""
				m.searchActive = false
				m.search.Blur()
				m.cursor = 0
				m = m.reload(0)
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "/":
			m.searchActive = true
			m.search.SetValue(m.filter)
			m.search.CursorEnd()
			m.search.Focus()
			return m, textinput.Blink
		case "esc":
			if m.filter != "" {
				m.filter = ""
				m.search.SetValue("")
				m.cursor = 0
				m = m.reload(0)
				return m, nil
			}
			return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardList} }
		case "backspace", "left", "h":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardList} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "s":
			// Cycle sort. Anchor on the cursored article so the highlight
			// follows the row across the reorder. NOTE(score-vs-pushcount):
			// the user-facing label says 推文量 (push amount); under the hood
			// we sort by recommend_score (signed +1/-1/0 sum). Acceptable
			// proxy for now; if the product distinction matters later, add
			// a maintained push_count column.
			m.sort = (m.sort + 1) % 2
			m = m.reload(m.currentAnchorID())
			return m, nil
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
			pinned := !a.PinnedAt.Valid
			ctx := context.Background()
			u := m.deps.User
			if err := m.deps.Store.Articles().SetPinned(ctx, a.ID, u.ID, u.Role, pinned); err != nil {
				return m, nil
			}
			m = m.reload(a.ID)
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
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	if m.board != nil {
		b.WriteString(StyleHeader.Render(i18n.Tf(loc, i18n.ScreenBoardViewTitleNamed, m.board.Name, m.board.Title)))
	} else {
		b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenBoardViewTitleBare)))
	}
	b.WriteString("\n\n")
	b.WriteString(m.renderBanner())

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		return b.String()
	}

	// Search row + sort indicator. Search runs above the article table so the
	// table's row layout stays predictable across show/hide.
	switch {
	case m.searchActive:
		b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenBoardViewSearchPrompt)) + m.search.View() + "\n")
		b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenBoardViewSearchInProgress)) + "\n\n")
	case m.filter != "":
		b.WriteString("  " + StyleDim.Render(i18n.Tf(loc, i18n.ScreenBoardViewSearchActive, m.filter, len(m.articles))) + "\n\n")
	}

	if len(m.articles) == 0 {
		hint := "(no articles yet — press p to write one)"
		help := "p post · / search · ? help · Esc/←/h back · Ctrl+C disconnect"
		if m.filter != "" {
			hint = i18n.T(loc, i18n.ScreenBoardViewNoArticles)
		}
		if m.isGuest() {
			hint = "(no articles yet)"
			help = "/ search · ? help · Esc/←/h back · Ctrl+C disconnect"
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
	b.WriteString(StyleDim.Render(header))
	if m.sort == store.SortByScoreDesc {
		b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenBoardViewSortByScore)))
	}
	b.WriteString("\n")

	for i, a := range m.articles {
		score := ""
		switch {
		case a.RecommendScore >= 100:
			score = i18n.ScoreExploded(loc)
		case a.RecommendScore > 0:
			score = fmt.Sprintf("%d", a.RecommendScore)
		case a.RecommendScore < 0:
			score = "X" + fmt.Sprintf("%d", -a.RecommendScore)
		}
		// Pinned (板規 / 置頂) articles surface a leading "[M] " marker —
		// the same glyph PTT uses for M-marked posts. Locked / arrows-only
		// articles stack additional [鎖] / [箭] (zh-TW) or [L] / [A] (en)
		// badges via i18n.CommentsModeBadge so users see the constraint
		// before opening the article. Long titles still get truncated.
		title := a.Title
		var prefix string
		if a.PinnedAt.Valid {
			prefix += "[M]"
		}
		prefix += i18n.CommentsModeBadge(loc, a.CommentsMode)
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

	help := i18n.T(loc, i18n.ScreenBoardViewHelpLine)
	if m.isGuest() {
		// Guest variant strips the "p post" affordance — they can't write.
		help = "↑/↓ j/k move · Enter/→/l open · / search · s sort · ? help · Esc/←/h back · Ctrl+C disconnect"
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
