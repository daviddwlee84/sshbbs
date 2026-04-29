package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

// =====================================================================
// Inbox
// =====================================================================

type wbInboxModel struct {
	deps    Deps
	items   []*store.WaterBalloon
	cursor  int
	width   int
	height  int
	loadErr error
}

func newWBInboxModel(deps Deps) wbInboxModel {
	items, err := deps.Store.WaterBalloons().ListInboxFor(context.Background(), deps.User.ID, 100)
	// Mark all unread as read once shown — matches PTT behaviour where
	// reading the inbox clears the "new mail" indicator.
	if err == nil {
		_ = deps.Store.WaterBalloons().MarkAllReadFor(context.Background(), deps.User.ID)
	}
	return wbInboxModel{deps: deps, items: items, loadErr: err}
}

func (m wbInboxModel) Init() tea.Cmd { return nil }

func (m wbInboxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "c":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenWBCompose} }
		case "r":
			if len(m.items) == 0 {
				return m, nil
			}
			it := m.items[m.cursor]
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenWBCompose, Recipient: it.FromUserIDStr}
			}
		}
	}
	return m, nil
}

func (m wbInboxModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(" 水球 Water Balloons "))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		return b.String()
	}
	if len(m.items) == 0 {
		b.WriteString("  " + StyleDim.Render("(no messages — press c to compose)") + "\n")
		b.WriteString("\n  " + StyleHelp.Render("c compose · Esc back"))
		return b.String()
	}

	const (
		dateW   = 16
		fromW   = 14
		statusW = 5
	)
	bodyW := max(20, m.width-2-dateW-1-fromW-1-statusW-3)
	header := fmt.Sprintf(" %s  %s  %s  %s",
		PadRight("Date", dateW),
		PadRight("From", fromW),
		PadRight("•", statusW),
		PadRight("Body", bodyW),
	)
	b.WriteString(StyleDim.Render(header) + "\n")

	for i, it := range m.items {
		status := "·"
		if !it.ReadAt.Valid {
			status = "NEW"
		}
		row := fmt.Sprintf(" %s  %s  %s  %s",
			PadRight(it.CreatedAt.Format("2006-01-02 15:04"), dateW),
			PadRight(it.FromUserIDStr, fromW),
			PadRight(status, statusW),
			Truncate(it.Body, bodyW),
		)
		if i == m.cursor {
			b.WriteString(StyleHighlight.Render("▸"+row[1:]) + "\n")
		} else {
			b.WriteString(" " + row + "\n")
		}
	}

	b.WriteString("\n  " + StyleHelp.Render("↑/↓ or j/k · c compose · r reply · Esc back"))
	b.WriteString("\n")
	return b.String()
}

// =====================================================================
// Compose
// =====================================================================

const (
	wbFocusTo = iota
	wbFocusBody
)

type wbComposeModel struct {
	deps   Deps
	to     textinput.Model
	body   textarea.Model
	focus  int
	width  int
	height int
	err    string
	sent   string // success: recipient userid
}

func newWBComposeModel(deps Deps, recipient string) wbComposeModel {
	to := textinput.New()
	to.Placeholder = "alice"
	to.CharLimit = 12
	to.Width = 30
	to.SetValue(recipient)

	body := textarea.New()
	body.Placeholder = "your one-line message…"
	body.CharLimit = 240
	body.SetWidth(60)
	body.SetHeight(4)

	m := wbComposeModel{deps: deps, to: to, body: body}
	if recipient == "" {
		m.focus = wbFocusTo
		m.to.Focus()
	} else {
		m.focus = wbFocusBody
		m.body.Focus()
	}
	return m
}

func (m wbComposeModel) Init() tea.Cmd { return textinput.Blink }

func (m wbComposeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.body.SetWidth(max(40, msg.Width-12))
		return m, nil

	case tea.KeyMsg:
		if m.sent != "" {
			return m, func() tea.Msg { return NavigateMsg{To: ScreenWBInbox} }
		}
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenWBInbox} }
		case "tab":
			m.toggleFocus()
			return m, nil
		case "ctrl+s":
			return m.submit()
		}

		if m.focus == wbFocusTo {
			if k := msg.String(); k == "enter" {
				m.toggleFocus()
				return m, nil
			}
			var cmd tea.Cmd
			m.to, cmd = m.to.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *wbComposeModel) toggleFocus() {
	if m.focus == wbFocusTo {
		m.focus = wbFocusBody
		m.to.Blur()
		m.body.Focus()
	} else {
		m.focus = wbFocusTo
		m.body.Blur()
		m.to.Focus()
	}
}

func (m wbComposeModel) submit() (tea.Model, tea.Cmd) {
	to := strings.TrimSpace(m.to.Value())
	body := strings.TrimSpace(m.body.Value())
	if to == "" {
		m.err = "recipient required"
		return m, nil
	}
	if body == "" {
		m.err = "body required"
		return m, nil
	}

	ctx := context.Background()
	target, err := m.deps.Store.Users().GetByUserID(ctx, to)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			m.err = "no such user: " + to
		} else {
			m.err = err.Error()
		}
		return m, nil
	}
	from := m.deps.User

	delivered := false
	if m.deps.Broker != nil {
		// Persist first, then notify with the canonical row's ID.
	}
	wb, err := m.deps.Store.WaterBalloons().Insert(ctx, from.ID, from.UserID, target.ID, body, false)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if m.deps.Broker != nil {
		delivered = m.deps.Broker.Send(target.ID, WBIncomingMsg{
			ID:         wb.ID,
			FromUserID: from.UserID,
			Body:       body,
		})
		if delivered {
			// The recipient saw it live, so we can mark it read immediately.
			_ = m.deps.Store.WaterBalloons().MarkRead(ctx, wb.ID)
		}
	}
	m.sent = target.UserID
	return m, nil
}

func (m wbComposeModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(" 丟水球 Send Water Balloon "))
	b.WriteString("\n\n")

	if m.sent != "" {
		b.WriteString("  " + StyleSuccess.Render("✓ sent to "+m.sent) + "\n\n")
		b.WriteString("  " + StyleHelp.Render("any key returns to inbox"))
		return b.String()
	}

	b.WriteString("  " + StyleDim.Render("To (userid):") + "\n")
	b.WriteString("  " + m.to.View() + "\n\n")
	b.WriteString("  " + StyleDim.Render("Body:") + "\n")
	b.WriteString("  " + indent(m.body.View(), "  ") + "\n")

	if m.err != "" {
		b.WriteString("\n  " + StyleError.Render("⚠ "+m.err) + "\n")
	}
	b.WriteString("\n  " + StyleHelp.Render("Tab switch field · Enter (in To) → body · Ctrl+S send · Esc cancel"))
	return b.String()
}
