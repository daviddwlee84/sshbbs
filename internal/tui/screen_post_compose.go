package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/markdown"
	"github.com/daviddwlee84/sshbbs/internal/notify"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

const (
	composeFocusTitle = iota
	composeFocusBody
)

type postComposeModel struct {
	deps     Deps
	boardID  int64
	parentID int64 // 0 = new post; non-zero = Re: reply to that article
	parent   *store.Article
	title    textinput.Model
	body     textarea.Model
	focus    int
	width    int
	height   int
	err      string
}

func newPostComposeModel(deps Deps, boardID, parentID int64) postComposeModel {
	loc := localeOf(deps)
	ti := textinput.New()
	ti.Placeholder = i18n.T(loc, i18n.ScreenPostComposeTitlePh)
	ti.CharLimit = 64
	ti.Width = 60
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = i18n.T(loc, i18n.ScreenPostComposeBodyPh)
	ta.CharLimit = 8000
	ta.SetWidth(72)
	ta.SetHeight(12)

	m := postComposeModel{
		deps:     deps,
		boardID:  boardID,
		parentID: parentID,
		title:    ti,
		body:     ta,
		focus:    composeFocusTitle,
	}

	// Reply mode: pull the parent article and pre-fill the title with "Re: "
	// (collapsing duplicates) and the body with a markdown blockquote of the
	// parent — same shape as mail reply (see quoteForReply in screen_mail.go).
	// Fall through to a blank compose if the parent has gone missing.
	if parentID != 0 && deps.Store != nil {
		if parent, err := deps.Store.Articles().GetByID(context.Background(), parentID); err == nil {
			m.parent = parent
			// boardID may have been passed as 0 by the caller (article-view
			// only knows the parent ID); fill it from the parent record so
			// the reply lands on the same board as the original.
			if m.boardID == 0 {
				m.boardID = parent.BoardID
			}
			m.title.SetValue(rePrefix(parent.Title))
			m.body.SetValue(quoteArticleForReply(parent, loc))
			// Land focus on the body so the user can start typing the reply
			// straight away — the prefilled title is usually fine as-is.
			m.title.Blur()
			m.body.Focus()
			m.focus = composeFocusBody
		}
	}
	return m
}

// rePrefix prepends "Re: " to the parent title, collapsing existing "Re: "
// runs so a long thread doesn't grow "Re: Re: Re: Hi".
func rePrefix(title string) string {
	t := strings.TrimSpace(title)
	for {
		if strings.HasPrefix(strings.ToLower(t), "re:") {
			t = strings.TrimSpace(t[3:])
			continue
		}
		break
	}
	return "Re: " + t
}

// quoteArticleForReply renders the parent article as a markdown blockquote
// suitable for pre-filling a reply body. Mirrors mail's quoteForReply.
// The reply body lands in the COMPOSER's textarea and is then sent to
// other users (broadcast + the article itself), so we use the composer's
// locale here rather than the recipient's. The "wrote" suffix is the only
// localised piece.
func quoteArticleForReply(parent *store.Article, loc i18n.Locale) string {
	var b strings.Builder
	fmt.Fprintf(&b, "> %s · %s %s:\n>\n",
		parent.AuthorUserID,
		parent.CreatedAt.Format("2006-01-02 15:04"),
		i18n.T(loc, i18n.ScreenPostComposeWroteSuffix),
	)
	for _, line := range strings.Split(parent.Body, "\n") {
		b.WriteString("> ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m postComposeModel) Init() tea.Cmd { return textinput.Blink }

func (m postComposeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		bodyW := max(40, msg.Width-8)
		bodyH := max(8, msg.Height-12)
		m.body.SetWidth(bodyW)
		m.body.SetHeight(bodyH)
		m.title.Width = max(20, msg.Width-12)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardView, BoardID: m.boardID} }
		case "tab":
			m.toggleFocus()
			return m, nil
		case "ctrl+s":
			return m.submit()
		case "ctrl+i":
			m.importFromBody()
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.focus == composeFocusTitle {
		// In title field, Enter advances to body.
		if k, ok := msg.(tea.KeyMsg); ok && k.String() == "enter" {
			m.toggleFocus()
			return m, nil
		}
		m.title, cmd = m.title.Update(msg)
	} else {
		m.body, cmd = m.body.Update(msg)
	}
	return m, cmd
}

func (m *postComposeModel) toggleFocus() {
	if m.focus == composeFocusTitle {
		m.focus = composeFocusBody
		m.title.Blur()
		m.body.Focus()
	} else {
		m.focus = composeFocusTitle
		m.body.Blur()
		m.title.Focus()
	}
}

// importFromBody parses the current body as markdown and prefills title +
// body from any frontmatter found. If the body has no frontmatter it's
// left alone (with a helpful error). Pushes in the parsed input are
// silently ignored — see plan: import via Ctrl+I always credits the
// current user as author and never restores foreign-authored pushes.
func (m *postComposeModel) importFromBody() {
	raw := m.body.Value()
	loc := localeOf(m.deps)
	if strings.TrimSpace(raw) == "" {
		m.err = i18n.T(loc, i18n.ScreenPostComposeFMHintPaste)
		return
	}
	parsed, err := markdown.Parse(raw)
	if err != nil {
		m.err = "parse: " + err.Error()
		return
	}
	if parsed.Title == "" && parsed.Body == strings.TrimRight(raw, "\n") {
		// Parser didn't find frontmatter — body is verbatim. Tell the
		// user so they don't think Ctrl+I silently no-op'd.
		m.err = i18n.T(loc, i18n.ScreenPostComposeFMHintMissing)
		return
	}
	if parsed.Title != "" {
		m.title.SetValue(parsed.Title)
	}
	m.body.SetValue(parsed.Body)
	m.err = ""
}

func (m postComposeModel) submit() (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.title.Value())
	body := m.body.Value()
	if title == "" {
		m.err = "title is required"
		return m, nil
	}
	if strings.TrimSpace(body) == "" {
		m.err = "body is required"
		return m, nil
	}
	ctx := context.Background()
	u := m.deps.User
	art, err := m.deps.Store.Articles().Create(ctx, m.boardID, u.ID, u.UserID, title, body)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if err := m.deps.Store.Users().IncrementPosts(ctx, u.ID); err != nil {
		m.err = err.Error()
		return m, nil
	}
	// Broadcast to other live sessions; recipients filter by BoardID and
	// re-fetch the canonical list (mirrors the PushAddedMsg pattern).
	if m.deps.Broker != nil {
		m.deps.Broker.SendToAll(u.ID, ArticleAddedMsg{
			BoardID:      m.boardID,
			ArticleID:    art.ID,
			AuthorUserID: u.UserID,
			Title:        art.Title,
		})
	}
	// Reply notification: when the post is a Re: reply to someone else's
	// article, fan out to that author's webhook targets so they hear about
	// it even when offline. Self-reply (replying to your own article) is
	// silenced — the user already saw their own action.
	if m.parent != nil && m.deps.Notify != nil && m.parent.AuthorID != u.ID {
		// Webhook title + body render in the RECIPIENT's locale (their
		// phone, their language) — see plan §5. recipientLocale loads
		// users.locale and falls back to Default on any error.
		recLoc := recipientLocale(m.deps, m.parent.AuthorID)
		m.deps.Notify.Dispatch(notify.Event{
			Kind:       notify.KindReply,
			ToUserID:   m.parent.AuthorID,
			FromUserID: u.UserID,
			Title:      i18n.Tf(recLoc, i18n.NotifyReplyTitle, u.UserID),
			Body:       i18n.Tf(recLoc, i18n.NotifyReplyBody, title, Truncate(m.parent.Title, 60)),
		})
	}
	return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardView, BoardID: m.boardID} }
}

func (m postComposeModel) View() string {
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	if m.parent != nil {
		b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenPostComposeTitleReply)))
		b.WriteString("\n  " + StyleDim.Render(i18n.Tf(loc, i18n.ScreenPostComposeReplyPrefix,
			m.parent.ID, m.parent.AuthorUserID, m.parent.Title)))
	} else {
		b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenPostComposeTitleNew)))
	}
	b.WriteString("\n\n")

	b.WriteString("  " + StyleDim.Render("Title:") + "\n")
	b.WriteString("  " + m.title.View() + "\n\n")

	b.WriteString("  " + StyleDim.Render("Body:") + "\n")
	b.WriteString("  " + indent(m.body.View(), "  ") + "\n")

	if m.err != "" {
		b.WriteString("\n  " + StyleError.Render("⚠ "+m.err) + "\n")
	}
	b.WriteString("\n  " + StyleHelp.Render("Tab switch field · Enter (in title) → body · Ctrl+S send · Ctrl+I import markdown · Esc cancel"))
	b.WriteString("\n")
	return b.String()
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if i == 0 {
			continue
		}
		lines[i] = prefix + ln
	}
	return strings.Join(lines, "\n")
}
