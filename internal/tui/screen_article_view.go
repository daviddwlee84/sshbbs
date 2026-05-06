package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

type articleViewModel struct {
	deps    Deps
	article *store.Article
	pushes  []*store.Push
	width   int
	height  int
	scroll  int
	loadErr error

	// rendered is the article body after running through glamour. We cache
	// because (a) glamour's first call loads chroma's syntax highlighter
	// (~tens of ms) and (b) View() is called on every keystroke. Re-render
	// only when the article changes or the window width changes.
	rendered      string
	renderedWidth int

	// push input state
	pushing  bool
	pushKind store.PushKind
	pushIn   textinput.Model
	err      string

	// delete state. pendingDelete: showing the y/N confirm overlay.
	// pushCursor: -1 = no push selected (D targets article); 0..len-1 = D targets that push.
	pendingDelete bool
	pushCursor    int

	// pickingCommentsMode: showing the 1/2/3 picker overlay for the
	// mod-only M shortcut (open / arrows-only / locked).
	pickingCommentsMode bool
}

// renderBody runs the article body through glamour with word-wrap set
// to the current viewport.
func (m *articleViewModel) renderBody() {
	if m.article == nil {
		m.rendered = ""
		m.renderedWidth = 0
		return
	}
	width := m.width - 4
	if width < 40 {
		width = glamourFallbackWidth
	}
	m.rendered = renderMarkdown(m.article.Body, width)
	m.renderedWidth = width
}

func newArticleViewModel(deps Deps, articleID int64) articleViewModel {
	ctx := context.Background()
	a, err := deps.Store.Articles().GetByID(ctx, articleID)
	if err != nil {
		return articleViewModel{deps: deps, loadErr: err}
	}
	pushes, err := deps.Store.Pushes().ListByArticle(ctx, articleID)

	ti := textinput.New()
	ti.Placeholder = "comment…"
	ti.CharLimit = 80
	ti.Width = 60

	m := articleViewModel{deps: deps, article: a, pushes: pushes, loadErr: err, pushIn: ti, pushCursor: -1}
	// Render with the fallback width so the body is readable even before
	// the first WindowSizeMsg arrives — Update re-renders on resize.
	m.renderBody()
	return m
}

// canDeleteArticle returns true if the current user may delete the loaded
// article (author, or mod-or-above). Used both as a permission gate at
// the key handler and to decide whether to render the help-line hint.
func (m articleViewModel) canDeleteArticle() bool {
	if m.deps.User == nil || m.article == nil {
		return false
	}
	if m.deps.User.ID == m.article.AuthorID {
		return true
	}
	return m.deps.User.Role.AtLeast(store.RoleMod)
}

// canDeletePush mirrors canDeleteArticle for the push at index idx.
func (m articleViewModel) canDeletePush(idx int) bool {
	if m.deps.User == nil || idx < 0 || idx >= len(m.pushes) {
		return false
	}
	if m.deps.User.ID == m.pushes[idx].UserID {
		return true
	}
	return m.deps.User.Role.AtLeast(store.RoleMod)
}

// canPush returns true if the current user is allowed to add new pushes.
// Guests can read but not write.
func (m articleViewModel) canPush() bool {
	return m.deps.User != nil && m.deps.User.Role != store.RoleGuest
}

// canEditArticle mirrors canDeleteArticle: the author may edit their own
// post, mods and admins may edit anyone's.
func (m articleViewModel) canEditArticle() bool {
	if m.deps.User == nil || m.article == nil {
		return false
	}
	if m.deps.User.ID == m.article.AuthorID {
		return true
	}
	return m.deps.User.Role.AtLeast(store.RoleMod)
}

// canSetCommentsMode gates the M shortcut. Mirrors board-view canPin —
// pinning and locking comments are both moderation actions that do NOT
// admit the article author. Mod+ only.
func (m articleViewModel) canSetCommentsMode() bool {
	return m.deps.User != nil && m.deps.User.Role.AtLeast(store.RoleMod)
}

func (m articleViewModel) Init() tea.Cmd { return nil }

func (m articleViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Only re-render when the width target actually changes — height
		// changes don't affect glamour wrap.
		newWidth := max(msg.Width-4, 40)
		if newWidth != m.renderedWidth {
			m.renderBody()
		}
		return m, nil

	case PushAddedMsg:
		if m.article != nil && msg.ArticleID == m.article.ID {
			ctx := context.Background()
			// Re-fetch from DB so timestamps and recommend_score reflect
			// the canonical state, not the just-synthesized broker payload.
			if pushes, err := m.deps.Store.Pushes().ListByArticle(ctx, m.article.ID); err == nil {
				m.pushes = pushes
			}
			if a, err := m.deps.Store.Articles().GetByID(ctx, m.article.ID); err == nil {
				m.article = a
			}
		}

	case ArticleUpdatedMsg:
		if m.article != nil && msg.ArticleID == m.article.ID {
			// Edit happened in another session — refetch the canonical
			// title/body so we render the new text.
			if a, err := m.deps.Store.Articles().GetByID(context.Background(), m.article.ID); err == nil {
				m.article = a
				m.renderBody()
				// Reset scroll if the new body is shorter than where we
				// were parked.
				if m.scroll > m.bodyLineCount() {
					m.scroll = 0
				}
			}
		}
		return m, nil

	case ArticleCommentsModeChangedMsg:
		if m.article != nil && msg.ArticleID == m.article.ID {
			// Mode flip happened in another session — refetch so the badge
			// and the +/-/= gates pick up the new value. No body re-render
			// needed (only the header changes).
			if a, err := m.deps.Store.Articles().GetByID(context.Background(), m.article.ID); err == nil {
				m.article = a
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.pushing {
			return m.updatePushInput(msg)
		}
		if m.pendingDelete {
			return m.updateDeleteConfirm(msg)
		}
		if m.pickingCommentsMode {
			return m.updateCommentsModePicker(msg)
		}
		switch msg.String() {
		case "esc", "backspace", "left", "h":
			boardID := int64(0)
			if m.article != nil {
				boardID = m.article.BoardID
			}
			return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardView, BoardID: boardID} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		case "pgup", "b":
			m.scroll = max(0, m.scroll-10)
		case "pgdown", " ":
			m.scroll += 10
		case "g":
			m.scroll = 0
		case "G":
			m.scroll = max(0, m.bodyLineCount()-m.viewportLines())
		case "[":
			if m.article == nil {
				return m, nil
			}
			prev, _, err := m.deps.Store.Articles().NeighboursOf(context.Background(), m.article.BoardID, m.article.ID)
			if err != nil || prev == 0 {
				return m, nil
			}
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenArticleView, ArticleID: prev, BoardID: m.article.BoardID}
			}
		case "]":
			if m.article == nil {
				return m, nil
			}
			_, next, err := m.deps.Store.Articles().NeighboursOf(context.Background(), m.article.BoardID, m.article.ID)
			if err != nil || next == 0 {
				return m, nil
			}
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenArticleView, ArticleID: next, BoardID: m.article.BoardID}
			}
		case "+":
			if !m.canPush() {
				return m, nil
			}
			return m.openPush(store.PushKindPush), nil
		case "-":
			if !m.canPush() {
				return m, nil
			}
			return m.openPush(store.PushKindBoo), nil
		case "=":
			if !m.canPush() {
				return m, nil
			}
			return m.openPush(store.PushKindArrow), nil
		case "p":
			// Advance the push cursor. -1 → 0 → ... → len-1 → -1 (cycles
			// back to "no selection, D targets the article").
			if len(m.pushes) == 0 {
				return m, nil
			}
			m.pushCursor++
			if m.pushCursor >= len(m.pushes) {
				m.pushCursor = -1
			}
			m.err = ""
			return m, nil
		case "P":
			if len(m.pushes) == 0 {
				return m, nil
			}
			if m.pushCursor < 0 {
				m.pushCursor = len(m.pushes) - 1
			} else {
				m.pushCursor--
			}
			m.err = ""
			return m, nil
		case "D":
			if m.pushCursor >= 0 {
				if !m.canDeletePush(m.pushCursor) {
					m.err = "權限不足 (only the push author or a mod can delete it)"
					return m, nil
				}
			} else {
				if !m.canDeleteArticle() {
					m.err = "權限不足 (only the author or a mod can delete)"
					return m, nil
				}
			}
			m.pendingDelete = true
			m.err = ""
			return m, nil
		case "E":
			if m.article == nil {
				return m, nil
			}
			if !m.canEditArticle() {
				m.err = "權限不足 (only the author or a mod can edit)"
				return m, nil
			}
			id := m.article.ID
			boardID := m.article.BoardID
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenArticleEdit, ArticleID: id, BoardID: boardID}
			}
		case "y":
			if m.article == nil {
				return m, nil
			}
			id := m.article.ID
			boardID := m.article.BoardID
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenArticleExport, ArticleID: id, BoardID: boardID}
			}
		case "M":
			// Open the comments-mode picker. Mod+ only — silent no-op
			// otherwise so non-mods don't get a confusing toast for a key
			// the help line doesn't even surface to them (mirrors M on
			// the board view).
			if m.article == nil || !m.canSetCommentsMode() {
				return m, nil
			}
			m.pickingCommentsMode = true
			m.err = ""
			return m, nil
		}
	}
	return m, nil
}

// updateDeleteConfirm handles y/n while the confirm overlay is up.
// y: dispatch the delete; if targeting a push, refresh the push list
// and the article (for the score). If targeting the article, navigate
// back to the board view.
// n / esc: cancel.
func (m articleViewModel) updateDeleteConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.article == nil || m.deps.User == nil {
			m.pendingDelete = false
			return m, nil
		}
		ctx := context.Background()
		if m.pushCursor >= 0 && m.pushCursor < len(m.pushes) {
			pushID := m.pushes[m.pushCursor].ID
			if err := m.deps.Store.Pushes().Delete(ctx, pushID, m.deps.User.ID, m.deps.User.Role); err != nil {
				m.err = err.Error()
				m.pendingDelete = false
				return m, nil
			}
			// Refresh the push list and re-fetch the article so its
			// recommend_score reflects the reverted delta.
			if pushes, err := m.deps.Store.Pushes().ListByArticle(ctx, m.article.ID); err == nil {
				m.pushes = pushes
			}
			if a, err := m.deps.Store.Articles().GetByID(ctx, m.article.ID); err == nil {
				m.article = a
			}
			// Clamp the cursor so the next D / p continues from a valid index.
			if m.pushCursor >= len(m.pushes) {
				m.pushCursor = -1
			}
			m.pendingDelete = false
			return m, nil
		}
		if err := m.deps.Store.Articles().Delete(ctx, m.article.ID, m.deps.User.ID, m.deps.User.Role); err != nil {
			m.err = err.Error()
			m.pendingDelete = false
			return m, nil
		}
		boardID := m.article.BoardID
		return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardView, BoardID: boardID} }
	case "n", "N", "esc":
		m.pendingDelete = false
		return m, nil
	}
	return m, nil
}

// bodyLineCount returns the number of lines View will render for the
// loaded article. Uses the glamour-rendered body so g/G/PgDn agree
// with what's actually on screen; falls back to raw if rendering hasn't
// run yet (shouldn't happen post-construction but defensive).
func (m articleViewModel) bodyLineCount() int {
	if m.rendered != "" {
		return len(strings.Split(m.rendered, "\n"))
	}
	if m.article == nil {
		return 0
	}
	return len(strings.Split(m.article.Body, "\n"))
}

// viewportLines mirrors the maxLines computation in View so g/G/PgDn
// agree on how many lines fit on screen.
func (m articleViewModel) viewportLines() int {
	return max(m.height-16, 5)
}

func (m articleViewModel) openPush(k store.PushKind) tea.Model {
	// Early gate based on the cached article state. The store-side check
	// in PushRepo.Create is the authoritative gate (handles races between
	// sessions), but rejecting here gives a friendlier error than the
	// raw sentinel and avoids opening the input box at all.
	if m.article != nil {
		switch m.article.CommentsMode {
		case store.CommentsModeLocked:
			m.err = "本文已鎖定留言"
			return m
		case store.CommentsModeArrowsOnly:
			if k != store.PushKindArrow {
				m.err = "本文僅開放箭頭留言"
				return m
			}
		}
	}
	m.pushing = true
	m.pushKind = k
	m.pushIn.SetValue("")
	m.pushIn.Focus()
	m.err = ""
	return m
}

// updateCommentsModePicker handles 1/2/3/Esc while the comments-mode
// picker overlay is up. On a successful flip it refetches the article
// locally and broadcasts ArticleCommentsModeChangedMsg so other live
// sessions update their badges and gates.
func (m articleViewModel) updateCommentsModePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.article == nil || m.deps.User == nil {
		m.pickingCommentsMode = false
		return m, nil
	}
	var mode store.CommentsMode
	switch msg.String() {
	case "1":
		mode = store.CommentsModeOpen
	case "2":
		mode = store.CommentsModeArrowsOnly
	case "3":
		mode = store.CommentsModeLocked
	case "esc", "n", "N":
		m.pickingCommentsMode = false
		return m, nil
	default:
		return m, nil
	}
	ctx := context.Background()
	u := m.deps.User
	if err := m.deps.Store.Articles().SetCommentsMode(ctx, m.article.ID, u.ID, u.Role, mode); err != nil {
		m.err = err.Error()
		m.pickingCommentsMode = false
		return m, nil
	}
	if a, err := m.deps.Store.Articles().GetByID(ctx, m.article.ID); err == nil {
		m.article = a
	}
	m.pickingCommentsMode = false
	if m.deps.Broker != nil {
		m.deps.Broker.SendToAll(u.ID, ArticleCommentsModeChangedMsg{
			BoardID:   m.article.BoardID,
			ArticleID: m.article.ID,
			Mode:      string(mode),
		})
	}
	return m, nil
}

func (m articleViewModel) updatePushInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.pushing = false
		m.pushIn.Blur()
		return m, nil
	case "enter":
		body := strings.TrimSpace(m.pushIn.Value())
		if body == "" && m.pushKind != store.PushKindArrow {
			m.err = "comment required for 推/噓"
			return m, nil
		}
		// Late gate: re-check comments_mode in case the cached article
		// is stale. Server-side will also reject (PushRepo.Create), but
		// catching here surfaces a friendlier message than the sentinel.
		if m.article != nil {
			switch m.article.CommentsMode {
			case store.CommentsModeLocked:
				m.err = "本文已鎖定留言"
				m.pushing = false
				m.pushIn.Blur()
				return m, nil
			case store.CommentsModeArrowsOnly:
				if m.pushKind != store.PushKindArrow {
					m.err = "本文僅開放箭頭留言"
					m.pushing = false
					m.pushIn.Blur()
					return m, nil
				}
			}
		}
		ctx := context.Background()
		u := m.deps.User
		p, err := m.deps.Store.Pushes().Create(ctx, m.article.ID, u.ID, u.UserID, m.pushKind, body)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		// Append locally so the originator sees it immediately.
		m.pushes = append(m.pushes, p)
		if a, err := m.deps.Store.Articles().GetByID(ctx, m.article.ID); err == nil {
			m.article = a
		}
		m.pushing = false
		m.pushIn.Blur()
		// Broadcast to other live sessions; they filter by ArticleID.
		if m.deps.Broker != nil {
			m.deps.Broker.SendToAll(u.ID, PushAddedMsg{
				ArticleID:  m.article.ID,
				UserUserID: u.UserID,
				Kind:       string(m.pushKind),
				Body:       body,
			})
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.pushIn, cmd = m.pushIn.Update(msg)
	return m, cmd
}

func (m articleViewModel) View() string {
	if m.loadErr != nil {
		return "\n  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n"
	}
	if m.article == nil {
		return "\n  " + StyleDim.Render("(no article)") + "\n"
	}
	a := m.article

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(fmt.Sprintf(" 文章 #%d · %s ", a.ID, a.AuthorUserID)))
	b.WriteString("\n\n")

	b.WriteString("  " + StyleDim.Render("Title: ") + a.Title + "\n")
	b.WriteString("  " + StyleDim.Render("Date:  ") + a.CreatedAt.Format("2006-01-02 15:04:05") + "\n")
	b.WriteString("  " + StyleDim.Render("Score: ") + fmt.Sprintf("%d", a.RecommendScore) + "\n")
	switch a.CommentsMode {
	case store.CommentsModeArrowsOnly:
		b.WriteString("  " + StyleDim.Render("留言:  ") + StyleError.Render("[箭] 僅開放箭頭") + "\n")
	case store.CommentsModeLocked:
		b.WriteString("  " + StyleDim.Render("留言:  ") + StyleError.Render("[鎖] 已關閉留言") + "\n")
	}
	b.WriteString("\n")

	body := m.rendered
	if body == "" {
		body = a.Body
	}
	bodyLines := strings.Split(body, "\n")
	start := min(m.scroll, len(bodyLines))
	maxLines := max(m.height-16, 5)
	end := min(start+maxLines, len(bodyLines))
	for _, line := range bodyLines[start:end] {
		b.WriteString(line + "\n")
	}
	if start > 0 || end < len(bodyLines) {
		b.WriteString("  " + StyleDim.Render(fmt.Sprintf("[lines %d-%d / %d]", start+1, end, len(bodyLines))))
		b.WriteString("\n")
	}

	if len(m.pushes) > 0 {
		b.WriteString("\n  " + StyleDim.Render(fmt.Sprintf("── 推文 (%d) ──", len(m.pushes))) + "\n")
		for i, p := range m.pushes {
			ts := p.CreatedAt.Format("01/02 15:04")
			gutter := "  "
			if i == m.pushCursor {
				gutter = "▸ "
			}
			line := fmt.Sprintf("%s%s %s %s  %s",
				gutter,
				renderPushKind(p.Kind),
				PadRight(p.UserUserID, 14),
				p.Body,
				StyleDim.Render(ts),
			)
			if i == m.pushCursor {
				line = StyleHighlight.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	if m.pushing {
		b.WriteString("\n  ")
		b.WriteString(renderPushKind(m.pushKind) + " ")
		b.WriteString(m.pushIn.View())
		b.WriteString("\n")
		if m.err != "" {
			b.WriteString("  " + StyleError.Render("⚠ "+m.err) + "\n")
		}
		b.WriteString("  " + StyleHelp.Render("Enter send · Esc cancel"))
	} else if m.pendingDelete {
		prompt := "確定刪除這篇文章? (y/N)"
		if m.pushCursor >= 0 && m.pushCursor < len(m.pushes) {
			prompt = fmt.Sprintf("確定刪除推文 #%d? (y/N)", m.pushCursor+1)
		}
		b.WriteString("\n  " + StyleError.Render("⚠ "+prompt))
	} else if m.pickingCommentsMode {
		b.WriteString("\n  " + StyleHeader.Render(" 留言模式 "))
		b.WriteString("\n  " + StyleHelp.Render("1 開放  2 僅箭頭  3 鎖文  Esc 取消"))
	} else {
		help := "j/k scroll · y 匯出"
		if m.canPush() {
			help += " · + 推 · - 噓 · = →"
		}
		if len(m.pushes) > 0 {
			help += " · p/P 選推文"
		}
		if m.canEditArticle() {
			help += " · E 編輯"
		}
		if m.canSetCommentsMode() {
			help += " · M 留言模式"
		}
		if m.pushCursor >= 0 && m.canDeletePush(m.pushCursor) {
			help += " · D 刪除推文"
		} else if m.canDeleteArticle() {
			help += " · D 刪除文章"
		}
		help += " · Esc/← back · Ctrl+C disconnect"
		if m.err != "" {
			b.WriteString("\n  " + StyleError.Render("⚠ "+m.err))
		}
		b.WriteString("\n  " + StyleHelp.Render(help))
	}
	b.WriteString("\n")
	return b.String()
}

func renderPushKind(k store.PushKind) string {
	switch k {
	case store.PushKindPush:
		return StylePushKind.Render("推")
	case store.PushKindBoo:
		return StyleBooKind.Render("噓")
	case store.PushKindArrow:
		return StyleArrowKind.Render("→")
	}
	return "?"
}
