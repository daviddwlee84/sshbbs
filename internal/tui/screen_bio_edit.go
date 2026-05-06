package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/auth"
	"github.com/daviddwlee84/sshbbs/internal/i18n"
)

// bioEditModel is a single-textarea form screen reached from the user
// settings menu. Form-screen convention (CLAUDE.md): no h/l navigation —
// those keys must remain available for text editing.
type bioEditModel struct {
	deps    Deps
	body    textarea.Model
	width   int
	height  int
	err     string
	success bool
}

func newBioEditModel(deps Deps) bioEditModel {
	ta := textarea.New()
	ta.Placeholder = i18n.T(localeOf(deps), i18n.ScreenBioEditPlaceholder)
	ta.CharLimit = 4096 // generous client-side; auth.ValidateBio is the source of truth (rune count)
	ta.SetWidth(72)
	ta.SetHeight(10)
	if deps.User != nil && deps.User.Bio != "" {
		ta.SetValue(deps.User.Bio)
	}
	ta.Focus()
	return bioEditModel{deps: deps, body: ta}
}

func (m bioEditModel) Init() tea.Cmd { return textarea.Blink }

func (m bioEditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.body.SetWidth(max(40, msg.Width-8))
		m.body.SetHeight(max(8, msg.Height-12))
		return m, nil

	case tea.KeyMsg:
		if m.success {
			return m, func() tea.Msg { return NavigateMsg{To: ScreenUserSettings} }
		}
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenUserSettings} }
		case "ctrl+s":
			return m.submit()
		}
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

func (m bioEditModel) submit() (tea.Model, tea.Cmd) {
	bio := strings.TrimRight(m.body.Value(), "\n")
	if err := auth.ValidateBio(bio); err != nil {
		m.err = err.Error()
		return m, nil
	}
	if m.deps.User == nil {
		m.err = "internal error: no user"
		return m, nil
	}
	ctx := context.Background()
	if err := m.deps.Store.Users().SetBio(ctx, m.deps.User.ID, bio); err != nil {
		m.err = err.Error()
		return m, nil
	}
	// Refresh User in place so the settings screen header and any future
	// reads see the canonical bio without a re-login.
	if fresh, err := m.deps.Store.Users().GetByID(ctx, m.deps.User.ID); err == nil {
		*m.deps.User = *fresh
	}
	m.success = true
	return m, func() tea.Msg { return NavigateMsg{To: ScreenUserSettings} }
}

func (m bioEditModel) View() string {
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenBioEditTitle)))
	b.WriteString("\n\n")

	b.WriteString("  " + StyleDim.Render("Bio (free-form, ≤ 1024 chars; newlines OK):") + "\n")
	b.WriteString("  " + indent(m.body.View(), "  ") + "\n")

	if m.err != "" {
		b.WriteString("\n  " + StyleError.Render("⚠ "+m.err))
	}
	if m.success {
		b.WriteString("\n  " + StyleSuccess.Render("✓ saved"))
	}
	b.WriteString("\n  " + StyleHelp.Render("Ctrl+S save · Esc cancel"))
	b.WriteString("\n")
	return b.String()
}
