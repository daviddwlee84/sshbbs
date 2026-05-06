package tui

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

const (
	editFocusTitle = iota
	editFocusBody
)

// articleEditModel mirrors postComposeModel but pre-fills from an existing
// article and routes to ArticleRepo.Update on submit. This is a form
// screen — h/l are intentionally NOT bound (they belong to the textarea).
type articleEditModel struct {
	deps      Deps
	articleID int64
	boardID   int64
	title     textinput.Model
	body      textarea.Model
	focus     int
	width     int
	height    int
	err       string
	loadErr   error
}

func newArticleEditModel(deps Deps, articleID int64) articleEditModel {
	loc := localeOf(deps)
	ti := textinput.New()
	ti.Placeholder = i18n.T(loc, i18n.ScreenArticleEditTitlePh)
	ti.CharLimit = 64
	ti.Width = 60

	ta := textarea.New()
	ta.Placeholder = i18n.T(loc, i18n.ScreenArticleEditBodyPh)
	ta.CharLimit = 8000
	ta.SetWidth(72)
	ta.SetHeight(12)

	m := articleEditModel{deps: deps, articleID: articleID, title: ti, body: ta, focus: editFocusTitle}

	if deps.User == nil {
		m.loadErr = errors.New("not logged in")
		return m
	}
	a, err := deps.Store.Articles().GetByID(context.Background(), articleID)
	if err != nil {
		m.loadErr = err
		return m
	}
	// Permission check at construction time — deny early so the screen
	// doesn't even mount for a user who can't edit. Mirrors
	// canDeleteArticle: author OR mod+.
	if a.AuthorID != deps.User.ID && !deps.User.Role.AtLeast(store.RoleMod) {
		m.loadErr = store.ErrPermissionDenied
		return m
	}
	m.boardID = a.BoardID
	m.title.SetValue(a.Title)
	m.body.SetValue(a.Body)
	m.title.Focus()
	return m
}

func (m articleEditModel) Init() tea.Cmd { return textinput.Blink }

func (m articleEditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.loadErr != nil {
			// Any key bounces back to article view (or board view if we
			// don't even know the article).
			return m, m.cancelCmd()
		}
		switch msg.String() {
		case "esc":
			return m, m.cancelCmd()
		case "tab":
			m.toggleFocus()
			return m, nil
		case "ctrl+s":
			return m.submit()
		}
	}

	if m.loadErr != nil {
		return m, nil
	}

	var cmd tea.Cmd
	if m.focus == editFocusTitle {
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

func (m *articleEditModel) toggleFocus() {
	if m.focus == editFocusTitle {
		m.focus = editFocusBody
		m.title.Blur()
		m.body.Focus()
	} else {
		m.focus = editFocusTitle
		m.body.Blur()
		m.title.Focus()
	}
}

func (m articleEditModel) cancelCmd() tea.Cmd {
	id := m.articleID
	boardID := m.boardID
	return func() tea.Msg {
		// If we never loaded a board id (loadErr early), fall back to the
		// board list — better than landing on an invalid article view.
		if boardID == 0 {
			return NavigateMsg{To: ScreenBoardList}
		}
		return NavigateMsg{To: ScreenArticleView, ArticleID: id, BoardID: boardID}
	}
}

func (m articleEditModel) submit() (tea.Model, tea.Cmd) {
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
	if err := m.deps.Store.Articles().Update(ctx, m.articleID, u.ID, u.Role, title, body); err != nil {
		m.err = err.Error()
		return m, nil
	}
	if m.deps.Broker != nil {
		// Exclude the editor's own session — they already mutated their
		// local article state by reloading on the way back to article view.
		m.deps.Broker.SendToAll(u.ID, ArticleUpdatedMsg{ArticleID: m.articleID})
	}
	id := m.articleID
	boardID := m.boardID
	return m, func() tea.Msg {
		return NavigateMsg{To: ScreenArticleView, ArticleID: id, BoardID: boardID}
	}
}

func (m articleEditModel) View() string {
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenArticleEditTitle)))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		b.WriteString("\n  " + StyleHelp.Render("press any key to go back"))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString("  " + StyleDim.Render("Title:") + "\n")
	b.WriteString("  " + m.title.View() + "\n\n")

	b.WriteString("  " + StyleDim.Render("Body:") + "\n")
	b.WriteString("  " + indent(m.body.View(), "  ") + "\n")

	if m.err != "" {
		b.WriteString("\n  " + StyleError.Render("⚠ "+m.err) + "\n")
	}
	b.WriteString("\n  " + StyleHelp.Render("Tab switch field · Enter (in title) → body · Ctrl+S save · Esc cancel"))
	b.WriteString("\n")
	return b.String()
}
