package tui

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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
	items   []*store.WBCounterparty
	cursor  int
	width   int
	height  int
	loadErr error
}

func newWBInboxModel(deps Deps) wbInboxModel {
	items, err := deps.Store.WaterBalloons().ListCounterpartiesFor(context.Background(), deps.User.ID, 100)
	// NOTE: we deliberately do NOT mark anything as read here. Mark-read
	// now happens on entering a specific thread (see wbThreadModel).
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
		case "esc", "backspace", "left", "h":
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
		case "r", "enter", " ", "right", "l":
			if len(m.items) == 0 {
				return m, nil
			}
			it := m.items[m.cursor]
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenWBThread, CounterpartyUserID: it.UserID}
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
		b.WriteString("\n  " + StyleHelp.Render("c compose · Esc/←/h back"))
		return b.String()
	}

	const (
		dateW   = 16
		nameW   = 14
		statusW = 5
	)
	previewW := max(20, m.width-2-dateW-1-nameW-1-statusW-3)
	header := fmt.Sprintf(" %s  %s  %s  %s",
		PadRight("Last", dateW),
		PadRight("With", nameW),
		PadRight("New", statusW),
		PadRight("Preview", previewW),
	)
	b.WriteString(StyleDim.Render(header) + "\n")

	viewerID := int64(0)
	if m.deps.User != nil {
		viewerID = m.deps.User.ID
	}
	for i, it := range m.items {
		status := "·"
		if it.UnreadCount > 0 {
			status = fmt.Sprintf("%d", it.UnreadCount)
		}
		nameStr := it.UserIDStr
		preview := it.LastBody
		if it.UserID == viewerID {
			// Self-thread shows as "📝 yourself"; the preview drops the
			// redundant "you:" prefix since both sender and recipient
			// are the viewer.
			nameStr = "📝 yourself"
		} else if it.LastFromMe {
			preview = "you: " + preview
		}
		row := fmt.Sprintf(" %s  %s  %s  %s",
			PadRight(it.LastAt.Format("2006-01-02 15:04"), dateW),
			PadRight(nameStr, nameW),
			PadRight(status, statusW),
			Truncate(preview, previewW),
		)
		if i == m.cursor {
			b.WriteString(StyleHighlight.Render("▸"+row[1:]) + "\n")
		} else {
			b.WriteString(" " + row + "\n")
		}
	}

	b.WriteString("\n  " + StyleHelp.Render("↑/↓ j/k move · Enter/→/l/r open · c compose · Esc/←/h back"))
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

	wb, err := m.deps.Store.WaterBalloons().Insert(ctx, from.ID, from.UserID, target.ID, body, false)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if target.ID == from.ID {
		// Self-WB (memo). Broker.Send to the caller's own session would
		// deadlock the unbuffered bubbletea msgs channel — see
		// pitfalls/water-balloon-self-send-hangs-server.md. Skip the
		// notification (you typed it, you saw it) and mark read so the
		// row doesn't replay as a toast on next reconnect.
		_ = m.deps.Store.WaterBalloons().MarkRead(ctx, wb.ID)
	} else if m.deps.Broker != nil {
		delivered := m.deps.Broker.Send(target.ID, WBIncomingMsg{
			ID:         wb.ID,
			FromUserID: from.UserID,
			Body:       body,
		})
		if delivered {
			// The recipient saw it live, so we can mark it read immediately.
			_ = m.deps.Store.WaterBalloons().MarkRead(ctx, wb.ID)
		}
		notifyWB(m.deps, target.ID, from.UserID, body)
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

// =====================================================================
// Thread (1-to-1 DM-style conversation with one counterparty)
//
// Layout: scrollback on top, single-line textinput at the bottom. Default
// focus is the input (chat-app convention). Tab toggles focus to the
// scrollback for vim-style scrolling. Sending Enter on a non-empty input
// goes through the same Insert + Broker.Send path as wbComposeModel so
// persistence and replay stay identical.
// =====================================================================

type wbThreadModel struct {
	deps       Deps
	cpID       int64  // counterparty user.id
	cpUserID   string // counterparty current handle (from users JOIN, not from_userid snapshot)
	items      []*store.WaterBalloon
	scroll     int
	width      int
	height     int
	loadErr    error
	input      textinput.Model
	focusInput bool   // true = textinput receives keystrokes; false = scrollback nav
	err        string // last send error, cleared on next keystroke
}

func newWBThreadModel(deps Deps, counterpartyID int64) wbThreadModel {
	ctx := context.Background()
	cp, err := deps.Store.Users().GetByID(ctx, counterpartyID)
	if err != nil {
		return wbThreadModel{deps: deps, cpID: counterpartyID, loadErr: err}
	}
	items, err := deps.Store.WaterBalloons().ListConversation(ctx, deps.User.ID, counterpartyID, 200)
	if err == nil {
		// Mark only the inbound side as read on entering the thread.
		// Replaces the old "open inbox = ack everything" behaviour.
		_ = deps.Store.WaterBalloons().MarkConversationRead(ctx, deps.User.ID, counterpartyID)
		// Reflect the mark in our local copy so View doesn't show stale "NEW".
		now := time.Now()
		for _, w := range items {
			if w.ToUserID == deps.User.ID && !w.ReadAt.Valid {
				w.ReadAt = sql.NullTime{Time: now, Valid: true}
			}
		}
	}

	in := textinput.New()
	in.Placeholder = "type a message…"
	in.CharLimit = 240
	in.Width = 60
	in.Prompt = "> "
	in.Focus()

	return wbThreadModel{
		deps:       deps,
		cpID:       counterpartyID,
		cpUserID:   cp.UserID,
		items:      items,
		loadErr:    err,
		input:      in,
		focusInput: true,
		scroll:     1 << 30, // start at bottom
	}
}

func (m wbThreadModel) Init() tea.Cmd { return textinput.Blink }

// matchesCounterparty reports whether handle (case-insensitive) names this
// thread's counterparty. Used by Root.Update to suppress the toast when an
// incoming wb is already going to be appended into the thread.
func (m wbThreadModel) matchesCounterparty(handle string) bool {
	return strings.EqualFold(handle, m.cpUserID)
}

func (m wbThreadModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Reserve room for prompt and a little chrome around the input.
		m.input.Width = max(20, msg.Width-8)
		return m, nil
	case WBIncomingMsg:
		if !m.matchesCounterparty(msg.FromUserID) {
			return m, nil
		}
		ctx := context.Background()
		if wb, err := m.deps.Store.WaterBalloons().GetByID(ctx, msg.ID); err == nil {
			m.items = append(m.items, wb)
			m.scroll = 1 << 30
		}
		_ = m.deps.Store.WaterBalloons().MarkRead(ctx, msg.ID)
		return m, nil
	case tea.KeyMsg:
		// Clear stale error on any keypress.
		m.err = ""
		if m.focusInput {
			return m.updateInputFocused(msg)
		}
		return m.updateScrollFocused(msg)
	}
	return m, nil
}

// updateInputFocused handles keys while the textinput has focus. The
// keymap is intentionally narrow — only Esc / Tab / scroll keys / Enter
// are intercepted; everything else (including h, j, k, l, q, Q, c, r and
// punctuation) is forwarded to the textinput so the user can type freely.
func (m wbThreadModel) updateInputFocused(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return NavigateMsg{To: ScreenWBInbox} }
	case "tab":
		m.focusInput = false
		m.input.Blur()
		return m, nil
	case "up", "pgup":
		if m.scroll > 0 {
			m.scroll--
		}
		return m, nil
	case "down", "pgdown":
		m.scroll++ // clamped at render time
		return m, nil
	case "enter":
		return m.submit()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateScrollFocused handles keys while the scrollback has focus.
// Familiar list-screen vim keys; Tab returns focus to the input.
func (m wbThreadModel) updateScrollFocused(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace", "left", "h":
		return m, func() tea.Msg { return NavigateMsg{To: ScreenWBInbox} }
	case "Q":
		return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
	case "tab":
		m.focusInput = true
		m.input.Focus()
		return m, textinput.Blink
	case "up", "k", "pgup":
		if m.scroll > 0 {
			m.scroll--
		}
	case "down", "j", "pgdown":
		m.scroll++
	case "g", "home":
		m.scroll = 0
	case "G", "end":
		m.scroll = 1 << 30
	}
	return m, nil
}

// submit posts the current input as a new water balloon. Mirrors
// wbComposeModel.submit so persistence and broker delivery behaviour
// stay identical — only the UI surfacing differs (inline append + clear
// instead of a "✓ sent" page transition).
func (m wbThreadModel) submit() (tea.Model, tea.Cmd) {
	body := strings.TrimSpace(m.input.Value())
	if body == "" {
		return m, nil
	}
	from := m.deps.User
	ctx := context.Background()
	wb, err := m.deps.Store.WaterBalloons().Insert(ctx, from.ID, from.UserID, m.cpID, body, false)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if m.cpID == from.ID {
		// Self-thread / memo: skip Broker.Send (would deadlock the
		// unbuffered msgs channel), mark read immediately. See
		// pitfalls/water-balloon-self-send-hangs-server.md.
		_ = m.deps.Store.WaterBalloons().MarkRead(ctx, wb.ID)
	} else if m.deps.Broker != nil {
		delivered := m.deps.Broker.Send(m.cpID, WBIncomingMsg{
			ID:         wb.ID,
			FromUserID: from.UserID,
			Body:       body,
		})
		if delivered {
			_ = m.deps.Store.WaterBalloons().MarkRead(ctx, wb.ID)
		}
		notifyWB(m.deps, m.cpID, from.UserID, body)
	}
	m.items = append(m.items, wb)
	m.input.SetValue("")
	m.scroll = 1 << 30
	return m, nil
}

func (m wbThreadModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	headerLabel := m.cpUserID
	if m.deps.User != nil && m.cpID == m.deps.User.ID {
		headerLabel = "📝 yourself (memo)"
	}
	b.WriteString(StyleHeader.Render(fmt.Sprintf(" 對話 with %s ", headerLabel)))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		return b.String()
	}

	// Scrollback occupies all the vertical space that isn't taken by the
	// input row, the optional error line, the help line, and the surrounding
	// blank rows / header.
	lines := m.renderLines()
	const reserved = 7 // header(2) + input(1) + help(2) + padding(2)
	visible := m.height - reserved
	if visible < 1 {
		visible = len(lines)
	}
	if m.scroll > max(0, len(lines)-visible) {
		m.scroll = max(0, len(lines)-visible)
	}
	if len(lines) == 0 {
		b.WriteString("  " + StyleDim.Render("(empty thread — type below to start)") + "\n")
	} else {
		end := min(m.scroll+visible, len(lines))
		for _, ln := range lines[m.scroll:end] {
			b.WriteString(ln + "\n")
		}
	}
	b.WriteString("\n")

	if m.err != "" {
		b.WriteString("  " + StyleError.Render("⚠ "+m.err) + "\n")
	}

	// Input row.
	prefix := "  "
	if m.focusInput {
		prefix = "▸ "
	}
	b.WriteString(prefix + m.input.View() + "\n")

	help := "Tab focus scrollback · Enter send · Esc back"
	if !m.focusInput {
		help = "↑/↓ j/k scroll · g/G top/end · Tab focus input · Esc/←/h back · Q quit"
	}
	b.WriteString("\n  " + StyleHelp.Render(help))
	return b.String()
}

// renderLines flattens the message list to display lines so scroll math is
// straightforward. Each message contributes a header line + body lines + a
// blank separator. Body wrapping is left to the terminal (240-char cap on
// compose keeps it short in practice).
func (m wbThreadModel) renderLines() []string {
	out := make([]string, 0, len(m.items)*3)
	viewerID := int64(0)
	if m.deps.User != nil {
		viewerID = m.deps.User.ID
	}
	for _, w := range m.items {
		var sender string
		if w.FromUserID == viewerID {
			sender = StyleDim.Render("you")
		} else {
			// Always use the current cp handle, not w.FromUserIDStr (snapshot).
			sender = StyleHighlight.Render(m.cpUserID)
		}
		out = append(out, "  "+sender+"  "+StyleDim.Render(w.CreatedAt.Format("2006-01-02 15:04")))
		out = append(out, "    "+w.Body)
		out = append(out, "")
	}
	return out
}
