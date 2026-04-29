package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	composeFocusTitle = iota
	composeFocusBody
)

type postComposeModel struct {
	deps    Deps
	boardID int64
	title   textinput.Model
	body    textarea.Model
	focus   int
	width   int
	height  int
	err     string
}

func newPostComposeModel(deps Deps, boardID int64) postComposeModel {
	ti := textinput.New()
	ti.Placeholder = "標題 title"
	ti.CharLimit = 64
	ti.Width = 60
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = "內容 body…"
	ta.CharLimit = 8000
	ta.SetWidth(72)
	ta.SetHeight(12)

	return postComposeModel{
		deps:    deps,
		boardID: boardID,
		title:   ti,
		body:    ta,
		focus:   composeFocusTitle,
	}
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
	_, err := m.deps.Store.Articles().Create(ctx, m.boardID, u.ID, u.UserID, title, body)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if err := m.deps.Store.Users().IncrementPosts(ctx, u.ID); err != nil {
		m.err = err.Error()
		return m, nil
	}
	return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardView, BoardID: m.boardID} }
}

func (m postComposeModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(" 發表新文章 New Post "))
	b.WriteString("\n\n")

	b.WriteString("  " + StyleDim.Render("Title:") + "\n")
	b.WriteString("  " + m.title.View() + "\n\n")

	b.WriteString("  " + StyleDim.Render("Body:") + "\n")
	b.WriteString("  " + indent(m.body.View(), "  ") + "\n")

	if m.err != "" {
		b.WriteString("\n  " + StyleError.Render("⚠ "+m.err) + "\n")
	}
	b.WriteString("\n  " + StyleHelp.Render("Tab switch field · Enter (in title) → body · Ctrl+S send · Esc cancel"))
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
