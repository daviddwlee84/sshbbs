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

type mailInboxModel struct {
	deps    Deps
	items   []*store.Mail
	cursor  int
	width   int
	height  int
	loadErr error
}

func newMailInboxModel(deps Deps) mailInboxModel {
	items, err := deps.Store.Mail().ListInboxFor(context.Background(), deps.User.ID, 100)
	return mailInboxModel{deps: deps, items: items, loadErr: err}
}

func (m mailInboxModel) Init() tea.Cmd { return nil }

func (m mailInboxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace", "left", "h":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k", "[":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j", "]":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "c":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMailCompose} }
		case "r":
			if len(m.items) == 0 {
				return m, nil
			}
			it := m.items[m.cursor]
			return m, func() tea.Msg {
				return NavigateMsg{
					To:        ScreenMailCompose,
					Recipient: it.FromUserIDStr,
					MailID:    it.ID,
				}
			}
		case "enter", " ", "right", "l":
			if len(m.items) == 0 {
				return m, nil
			}
			it := m.items[m.cursor]
			return m, func() tea.Msg {
				return NavigateMsg{To: ScreenMailThread, MailThreadID: it.ThreadID}
			}
		}
	}
	return m, nil
}

func (m mailInboxModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(" 信箱 Mail "))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		return b.String()
	}
	if len(m.items) == 0 {
		b.WriteString("  " + StyleDim.Render("(no mail — press c to compose)") + "\n")
		b.WriteString("\n  " + StyleHelp.Render("c compose · Esc/←/h back · Q quit"))
		return b.String()
	}

	const (
		dateW    = 16
		fromW    = 14
		statusW  = 5
	)
	subjectW := max(20, m.width-2-dateW-1-fromW-1-statusW-3)
	header := fmt.Sprintf(" %s  %s  %s  %s",
		PadRight("Date", dateW),
		PadRight("From", fromW),
		PadRight("•", statusW),
		PadRight("Subject", subjectW),
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
			Truncate(it.Subject, subjectW),
		)
		if i == m.cursor {
			b.WriteString(StyleHighlight.Render("▸"+row[1:]) + "\n")
		} else {
			b.WriteString(" " + row + "\n")
		}
	}

	b.WriteString("\n  " + StyleHelp.Render("↑/↓ j/k move · Enter/→/l open · r reply · c compose · Esc/←/h back · Q quit"))
	b.WriteString("\n")
	return b.String()
}

// =====================================================================
// Thread view
// =====================================================================

type mailThreadModel struct {
	deps     Deps
	threadID int64
	items    []*store.Mail
	scroll   int
	width    int
	height   int
	loadErr  error

	// rendered holds the glamour-formatted body for each item, parallel to
	// items[]. Cached because (a) glamour's first call loads chroma's
	// syntax highlighter and (b) View is called on every keystroke;
	// re-rendering N message bodies per keypress would lag visibly.
	// renderedWidth lets us re-render only when the viewport width changes.
	rendered      []string
	renderedWidth int
}

func newMailThreadModel(deps Deps, threadID int64) mailThreadModel {
	ctx := context.Background()
	items, err := deps.Store.Mail().ListThread(ctx, threadID)
	if err == nil {
		// Mark every mail in the thread that's addressed to the current user
		// as read. A thread you've opened is no longer "new".
		for _, it := range items {
			if it.ToUserID == deps.User.ID && !it.ReadAt.Valid {
				_ = deps.Store.Mail().MarkRead(ctx, it.ID)
			}
		}
	}
	m := mailThreadModel{deps: deps, threadID: threadID, items: items, loadErr: err}
	// Render at the fallback width so the body is readable even before the
	// first WindowSizeMsg lands. Update re-renders on resize.
	m.renderBodies()
	return m
}

// renderBodies runs each item's body through glamour at the current
// width. Cached on the model; cleared by passing a new width.
func (m *mailThreadModel) renderBodies() {
	width := m.width - 4
	if width < 40 {
		width = glamourFallbackWidth
	}
	out := make([]string, len(m.items))
	for i, it := range m.items {
		out[i] = renderMarkdown(it.Body, width)
	}
	m.rendered = out
	m.renderedWidth = width
}

func (m mailThreadModel) Init() tea.Cmd { return nil }

func (m mailThreadModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		newWidth := m.width - 4
		if newWidth < 40 {
			newWidth = glamourFallbackWidth
		}
		if newWidth != m.renderedWidth {
			m.renderBodies()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace", "left", "h":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMailInbox} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		case "g":
			m.scroll = 0
		case "r":
			// Reply to the LAST message in the thread (whoever sent it).
			if len(m.items) == 0 {
				return m, nil
			}
			last := m.items[len(m.items)-1]
			recipient := last.FromUserIDStr
			if last.FromUserID == m.deps.User.ID && last.ParentID.Valid {
				// We sent the last one; reply to the user we were replying to.
				if parent, err := m.deps.Store.Mail().GetByID(context.Background(), last.ParentID.Int64); err == nil {
					recipient = parent.FromUserIDStr
				}
			}
			return m, func() tea.Msg {
				return NavigateMsg{
					To:        ScreenMailCompose,
					Recipient: recipient,
					MailID:    last.ID,
				}
			}
		}
	}
	return m, nil
}

func (m mailThreadModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(fmt.Sprintf(" 信件 Thread #%d ", m.threadID)))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		return b.String()
	}
	if len(m.items) == 0 {
		b.WriteString("  " + StyleDim.Render("(empty thread)") + "\n")
		return b.String()
	}

	for i, it := range m.items {
		b.WriteString("  " + StyleDim.Render(fmt.Sprintf("── #%d  %s → %s  %s ──",
			i+1,
			it.FromUserIDStr,
			recipientName(m.deps, it.ToUserID),
			it.CreatedAt.Format("2006-01-02 15:04"),
		)) + "\n")
		b.WriteString("  " + StyleDim.Render("Subject: ") + it.Subject + "\n")
		// Use the glamour-rendered body when available; fall back to the
		// raw body if rendering hasn't run (e.g. zero-width pre-resize).
		body := it.Body
		if i < len(m.rendered) && m.rendered[i] != "" {
			body = m.rendered[i]
		}
		b.WriteString(body + "\n\n")
	}

	b.WriteString("  " + StyleHelp.Render("r reply · Esc/←/h back · Q quit"))
	b.WriteString("\n")
	return b.String()
}

// recipientName looks up the display handle for a user_id; falls back to
// the numeric id if the lookup fails (best-effort, View() must not error).
func recipientName(deps Deps, userID int64) string {
	if deps.Store == nil {
		return fmt.Sprintf("uid=%d", userID)
	}
	u, err := deps.Store.Users().GetByID(context.Background(), userID)
	if err != nil || u == nil {
		return fmt.Sprintf("uid=%d", userID)
	}
	return u.UserID
}

// =====================================================================
// Compose
// =====================================================================

const (
	mailFocusTo = iota
	mailFocusSubject
	mailFocusBody
)

type mailComposeModel struct {
	deps     Deps
	parentID int64 // 0 ⇒ new thread; else reply
	to       textinput.Model
	subject  textinput.Model
	body     textarea.Model
	focus    int
	width    int
	height   int
	err      string
	sent     string // success: recipient userid
}

func newMailComposeModel(deps Deps, recipient string, parentID int64) mailComposeModel {
	to := textinput.New()
	to.Placeholder = "alice"
	to.CharLimit = 12
	to.Width = 30
	to.SetValue(recipient)

	subj := textinput.New()
	subj.Placeholder = "subject"
	subj.CharLimit = 64
	subj.Width = 60

	body := textarea.New()
	body.Placeholder = "message body…"
	body.CharLimit = 4000
	body.SetWidth(60)
	body.SetHeight(8)

	// On reply, default the subject from the parent (with "Re: " prefix
	// once and only once) and pre-fill the body with the parent quoted in
	// markdown blockquote (`> `) form so the reply reads like an email
	// thread. Cursor lands after the trailing blank line.
	if parentID != 0 && deps.Store != nil {
		if parent, err := deps.Store.Mail().GetByID(context.Background(), parentID); err == nil {
			s := parent.Subject
			if !strings.HasPrefix(strings.ToLower(s), "re:") {
				s = "Re: " + s
			}
			subj.SetValue(s)
			body.SetValue(quoteForReply(parent))
		}
	}

	m := mailComposeModel{
		deps:     deps,
		parentID: parentID,
		to:       to,
		subject:  subj,
		body:     body,
	}
	switch {
	case recipient == "":
		m.focus = mailFocusTo
		m.to.Focus()
	case subj.Value() == "":
		m.focus = mailFocusSubject
		m.subject.Focus()
	default:
		m.focus = mailFocusBody
		m.body.Focus()
	}
	return m
}

func (m mailComposeModel) Init() tea.Cmd { return textinput.Blink }

func (m mailComposeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.body.SetWidth(max(40, msg.Width-12))
		return m, nil

	case tea.KeyMsg:
		if m.sent != "" {
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMailInbox} }
		}
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMailInbox} }
		case "tab":
			m.cycleFocus()
			return m, nil
		case "ctrl+s":
			return m.submit()
		}

		switch m.focus {
		case mailFocusTo:
			if k := msg.String(); k == "enter" {
				m.cycleFocus()
				return m, nil
			}
			var cmd tea.Cmd
			m.to, cmd = m.to.Update(msg)
			return m, cmd
		case mailFocusSubject:
			if k := msg.String(); k == "enter" {
				m.cycleFocus()
				return m, nil
			}
			var cmd tea.Cmd
			m.subject, cmd = m.subject.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *mailComposeModel) cycleFocus() {
	m.to.Blur()
	m.subject.Blur()
	m.body.Blur()
	m.focus = (m.focus + 1) % 3
	switch m.focus {
	case mailFocusTo:
		m.to.Focus()
	case mailFocusSubject:
		m.subject.Focus()
	case mailFocusBody:
		m.body.Focus()
	}
}

func (m mailComposeModel) submit() (tea.Model, tea.Cmd) {
	to := strings.TrimSpace(m.to.Value())
	subj := strings.TrimSpace(m.subject.Value())
	body := strings.TrimSpace(m.body.Value())
	if to == "" {
		m.err = "recipient required"
		return m, nil
	}
	if subj == "" {
		m.err = "subject required"
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
	var parentPtr *int64
	if m.parentID != 0 {
		p := m.parentID
		parentPtr = &p
	}
	mail, err := m.deps.Store.Mail().Insert(ctx, from.ID, from.UserID, target.ID, subj, body, parentPtr)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if target.ID == from.ID {
		// Self-mail (memo). Skip Broker.Send: target is the caller, and
		// pushing onto bubbletea's unbuffered msgs channel from inside
		// the caller's own Update loop deadlocks the SSH session — see
		// pitfalls/water-balloon-self-send-hangs-server.md (same root
		// cause, different surface). Mark read immediately so the unread
		// counter doesn't bump for a memo the user just wrote.
		_ = m.deps.Store.Mail().MarkRead(ctx, mail.ID)
	} else if m.deps.Broker != nil {
		// Notify the recipient so an open inbox can refresh. Distinct from
		// water balloons (which always toast); mail is "silent" delivery.
		m.deps.Broker.Send(target.ID, MailIncomingMsg{
			ID:         mail.ID,
			ThreadID:   mail.ThreadID,
			FromUserID: from.UserID,
			Subject:    subj,
		})
	}
	m.sent = target.UserID
	return m, nil
}

// quoteForReply renders the parent mail as a markdown blockquote suitable
// for pre-filling the reply body. Format:
//
//	> alice · 2026-05-06 14:23 寫道:
//	>
//	> previous body line 1
//	> previous body line 2
//
//	<cursor lands here>
//
// Empty lines in the parent body are still prefixed with "> " so the
// blockquote stays contiguous (matches RFC 3676 / common email-client
// convention).
func quoteForReply(parent *store.Mail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "> %s · %s 寫道:\n>\n",
		parent.FromUserIDStr,
		parent.CreatedAt.Format("2006-01-02 15:04"),
	)
	for _, line := range strings.Split(parent.Body, "\n") {
		b.WriteString("> ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m mailComposeModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.parentID == 0 {
		b.WriteString(StyleHeader.Render(" 寫信 New Mail "))
	} else {
		b.WriteString(StyleHeader.Render(" 回信 Reply "))
	}
	b.WriteString("\n\n")

	if m.sent != "" {
		b.WriteString("  " + StyleSuccess.Render("✓ sent to "+m.sent) + "\n\n")
		b.WriteString("  " + StyleHelp.Render("any key returns to inbox"))
		return b.String()
	}

	b.WriteString("  " + StyleDim.Render("To (userid):") + "\n")
	b.WriteString("  " + m.to.View() + "\n\n")
	b.WriteString("  " + StyleDim.Render("Subject:") + "\n")
	b.WriteString("  " + m.subject.View() + "\n\n")
	b.WriteString("  " + StyleDim.Render("Body:") + "\n")
	b.WriteString("  " + indent(m.body.View(), "  ") + "\n")

	if m.err != "" {
		b.WriteString("\n  " + StyleError.Render("⚠ "+m.err) + "\n")
	}
	b.WriteString("\n  " + StyleHelp.Render("Tab cycle field · Enter (in To/Subject) → next · Ctrl+S send · Esc cancel"))
	return b.String()
}
