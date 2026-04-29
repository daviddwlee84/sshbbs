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

	// push input state
	pushing  bool
	pushKind store.PushKind
	pushIn   textinput.Model
	err      string
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

	return articleViewModel{deps: deps, article: a, pushes: pushes, loadErr: err, pushIn: ti}
}

func (m articleViewModel) Init() tea.Cmd { return nil }

func (m articleViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
		return m, nil

	case tea.KeyMsg:
		if m.pushing {
			return m.updatePushInput(msg)
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
			return m.openPush(store.PushKindPush), nil
		case "-":
			return m.openPush(store.PushKindBoo), nil
		case "=":
			return m.openPush(store.PushKindArrow), nil
		}
	}
	return m, nil
}

// bodyLineCount returns the number of lines View will render for the
// loaded article (matches strings.Split(body, "\n") used in View).
func (m articleViewModel) bodyLineCount() int {
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
	m.pushing = true
	m.pushKind = k
	m.pushIn.SetValue("")
	m.pushIn.Focus()
	m.err = ""
	return m
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
	b.WriteString("\n")

	bodyLines := strings.Split(a.Body, "\n")
	start := min(m.scroll, len(bodyLines))
	maxLines := max(m.height-16, 5)
	end := min(start+maxLines, len(bodyLines))
	for _, line := range bodyLines[start:end] {
		b.WriteString("  " + line + "\n")
	}
	if start > 0 || end < len(bodyLines) {
		b.WriteString("  " + StyleDim.Render(fmt.Sprintf("[lines %d-%d / %d]", start+1, end, len(bodyLines))))
		b.WriteString("\n")
	}

	if len(m.pushes) > 0 {
		b.WriteString("\n  " + StyleDim.Render(fmt.Sprintf("── 推文 (%d) ──", len(m.pushes))) + "\n")
		for _, p := range m.pushes {
			ts := p.CreatedAt.Format("01/02 15:04")
			line := fmt.Sprintf("  %s %s %s  %s",
				renderPushKind(p.Kind),
				PadRight(p.UserUserID, 14),
				p.Body,
				StyleDim.Render(ts),
			)
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
	} else {
		b.WriteString("\n  " + StyleHelp.Render("j/k scroll · + 推 · - 噓 · = → · Esc/← back · Ctrl+C disconnect"))
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
