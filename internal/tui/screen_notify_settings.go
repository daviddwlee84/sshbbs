package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/notify"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

// notifyTestResultMsg is delivered by the goroutine that ran SendTest.
// The screen renders msg.err in the error/flash row so the user sees
// the HTTP outcome inline.
type notifyTestResultMsg struct {
	targetID int64
	err      error
}

// notifySettingsModel is the unified screen for the per-event toggles
// (user_notif_prefs) and webhook target list (user_notif_targets).
//
// Layout:
//
//   - Top: 5 boolean toggles (Push / WB / Mail / Reply / OnlyWhenOffline)
//     navigated with j/k, flipped with Space. Persisted via Ctrl+S.
//   - Bottom: target list, then "+ Add new target" entry. Targets toggle
//     enabled with `t`, edit with `e`, delete with `d`. Adds and edits
//     pop an inline two-field form (label + URL).
//
// State machine: mode == modeList during normal browsing; switches to
// modeEdit while the inline form is up. Mirrors the inline-textinput-
// inside-list pattern used by screen_wb.go.
type notifySettingsModel struct {
	deps Deps

	prefs   store.NotifyPrefs
	targets []*store.NotifyTarget
	loadErr error

	cursor      int  // 0..4 = toggles, 5..5+N-1 = targets, 5+N = "+ add"
	prefsDirty  bool // true if Ctrl+S would persist a change

	mode editMode

	editLabel textinput.Model
	editURL   textinput.Model
	editFocus int   // 0 = label, 1 = url
	editingID int64 // 0 for new target, otherwise target.ID

	width  int
	height int
	err    string
	flash  string // transient success message
}

type editMode int

const (
	modeList editMode = iota
	modeEdit
)

const (
	notifPrefCount = 5 // Push, WB, Mail, Reply, OnlyWhenOffline
)

func newNotifySettingsModel(deps Deps) notifySettingsModel {
	loc := localeOf(deps)
	m := notifySettingsModel{deps: deps}
	m.editLabel = textinput.New()
	m.editLabel.Placeholder = i18n.T(loc, i18n.ScreenNotifyEditLabelPh)
	m.editLabel.CharLimit = 64
	m.editLabel.Width = 60
	m.editURL = textinput.New()
	m.editURL.Placeholder = i18n.T(loc, i18n.ScreenNotifyEditURLPh)
	m.editURL.CharLimit = 512
	m.editURL.Width = 60

	if deps.User != nil && deps.Store != nil {
		ctx := context.Background()
		p, err := deps.Store.Notify().GetPrefs(ctx, deps.User.ID)
		if err != nil {
			m.loadErr = err
		}
		m.prefs = p
		ts, err := deps.Store.Notify().ListTargets(ctx, deps.User.ID)
		if err != nil && m.loadErr == nil {
			m.loadErr = err
		}
		m.targets = ts
	}
	return m
}

func (m notifySettingsModel) Init() tea.Cmd { return textinput.Blink }

// rowCount returns the total navigable row count: 5 prefs + N targets + 1 add.
func (m notifySettingsModel) rowCount() int {
	return notifPrefCount + len(m.targets) + 1
}

// addRowIndex returns the cursor index of the "+ add target" row.
func (m notifySettingsModel) addRowIndex() int {
	return notifPrefCount + len(m.targets)
}

func (m notifySettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.editLabel.Width = max(20, msg.Width-16)
		m.editURL.Width = max(20, msg.Width-16)
		return m, nil

	case tea.KeyMsg:
		if m.mode == modeEdit {
			return m.updateEdit(msg)
		}
		return m.updateList(msg)

	case notifyTestResultMsg:
		if msg.err != nil {
			m.err = fmt.Sprintf("target #%d test failed: %v", msg.targetID, msg.err)
			m.flash = ""
		} else {
			m.flash = fmt.Sprintf("✓ target #%d test delivered — check your receiver", msg.targetID)
			m.err = ""
		}
		return m, nil
	}
	return m, nil
}

func (m notifySettingsModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "left", "h", "backspace":
		return m, func() tea.Msg { return NavigateMsg{To: ScreenUserSettings} }
	case "Q":
		return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		m.err = ""
		return m, nil
	case "down", "j":
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
		m.err = ""
		return m, nil
	case " ":
		if m.cursor < notifPrefCount {
			m.flipPref(m.cursor)
			m.prefsDirty = true
			m.err = ""
		} else if m.cursor == m.addRowIndex() {
			m.startAdd()
		} else {
			// Space on a target → toggle enabled (alias of `t`).
			m.toggleTarget()
		}
		return m, nil
	case "ctrl+s":
		return m.savePrefs()
	case "a":
		m.startAdd()
		return m, nil
	case "t":
		if m.cursor >= notifPrefCount && m.cursor < m.addRowIndex() {
			m.toggleTarget()
		}
		return m, nil
	case "e":
		if m.cursor >= notifPrefCount && m.cursor < m.addRowIndex() {
			m.startEdit()
		}
		return m, nil
	case "d":
		if m.cursor >= notifPrefCount && m.cursor < m.addRowIndex() {
			m.deleteTarget()
		}
		return m, nil
	case "T":
		// Send a synthetic test notification to the cursored target.
		// Bypasses prefs (so users can test before flipping any toggles)
		// and bypasses the dispatch queue (so we get a synchronous
		// pass/fail to surface in the flash row).
		if m.cursor >= notifPrefCount && m.cursor < m.addRowIndex() {
			return m, m.startTest()
		}
		return m, nil
	case "enter", "right", "l":
		if m.cursor == m.addRowIndex() {
			m.startAdd()
			return m, nil
		}
		// Enter on a toggle = flip; on a target = edit.
		if m.cursor < notifPrefCount {
			m.flipPref(m.cursor)
			m.prefsDirty = true
			return m, nil
		}
		m.startEdit()
		return m, nil
	}
	return m, nil
}

func (m *notifySettingsModel) flipPref(idx int) {
	switch idx {
	case 0:
		m.prefs.OnPush = !m.prefs.OnPush
	case 1:
		m.prefs.OnWB = !m.prefs.OnWB
	case 2:
		m.prefs.OnMail = !m.prefs.OnMail
	case 3:
		m.prefs.OnReply = !m.prefs.OnReply
	case 4:
		m.prefs.OnlyWhenOffline = !m.prefs.OnlyWhenOffline
	}
}

func (m notifySettingsModel) savePrefs() (tea.Model, tea.Cmd) {
	if m.deps.User == nil {
		m.err = "internal error: no user"
		return m, nil
	}
	if !m.prefsDirty {
		m.flash = "(no changes)"
		return m, nil
	}
	if err := m.deps.Store.Notify().SetPrefs(context.Background(), m.deps.User.ID, m.prefs); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.prefsDirty = false
	m.flash = i18n.T(localeOf(m.deps), i18n.ScreenNotifyFlashPrefsSaved)
	m.err = ""
	return m, nil
}

func (m *notifySettingsModel) startAdd() {
	m.mode = modeEdit
	m.editingID = 0
	m.editLabel.SetValue("")
	m.editURL.SetValue("")
	m.editFocus = 0
	m.editLabel.Focus()
	m.editURL.Blur()
	m.err = ""
}

func (m *notifySettingsModel) startEdit() {
	idx := m.cursor - notifPrefCount
	if idx < 0 || idx >= len(m.targets) {
		return
	}
	t := m.targets[idx]
	m.mode = modeEdit
	m.editingID = t.ID
	m.editLabel.SetValue(t.Label)
	m.editURL.SetValue(t.URL)
	m.editFocus = 0
	m.editLabel.Focus()
	m.editURL.Blur()
	m.err = ""
}

func (m *notifySettingsModel) toggleTarget() {
	idx := m.cursor - notifPrefCount
	if idx < 0 || idx >= len(m.targets) {
		return
	}
	t := m.targets[idx]
	want := !t.Enabled
	if err := m.deps.Store.Notify().SetTargetEnabled(context.Background(), t.ID, m.deps.User.ID, want); err != nil {
		m.err = err.Error()
		return
	}
	t.Enabled = want
	loc := localeOf(m.deps)
	if want {
		m.flash = i18n.Tf(loc, i18n.ScreenNotifyFlashEnabled, t.ID)
	} else {
		m.flash = i18n.Tf(loc, i18n.ScreenNotifyFlashDisabled, t.ID)
	}
	m.err = ""
}

// startTest fires a one-shot test notification at the cursored target.
// Returns a tea.Cmd that runs the HTTP call in a goroutine (the
// dispatcher's http.Client has a 5s timeout) and rendezvous-es the
// result back as a notifyTestResultMsg.
func (m *notifySettingsModel) startTest() tea.Cmd {
	idx := m.cursor - notifPrefCount
	if idx < 0 || idx >= len(m.targets) {
		return nil
	}
	t := m.targets[idx]
	if m.deps.Notify == nil {
		// Tests / register-only sessions: no dispatcher wired up. Fail
		// loud rather than silently no-op.
		m.err = "notify dispatcher not initialised"
		return nil
	}
	m.flash = fmt.Sprintf("🔔 testing target #%d…", t.ID)
	m.err = ""
	// Snapshot the values the goroutine needs so we don't reach back into
	// the model after returning (model updates are funnelled through
	// Update by bubbletea's main loop).
	target := *t
	user := ""
	if m.deps.User != nil {
		user = m.deps.User.UserID
	}
	dispatcher := m.deps.Notify
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		ev := notify.Event{
			Kind:       notify.KindPush, // bypassed by SendTest; here just for log readability
			ToUserID:   target.UserID,
			FromUserID: user,
			Title:      "[BBS-test] notification target verified",
			Body: fmt.Sprintf(
				"User: %s\nTarget: #%d (%s)\nTime: %s\n\nIf you can read this, the webhook URL is wired up correctly.",
				user, target.ID, target.Label, time.Now().Format("2006-01-02 15:04:05"),
			),
		}
		return notifyTestResultMsg{
			targetID: target.ID,
			err:      dispatcher.SendTest(ctx, &target, ev),
		}
	}
}

func (m *notifySettingsModel) deleteTarget() {
	idx := m.cursor - notifPrefCount
	if idx < 0 || idx >= len(m.targets) {
		return
	}
	t := m.targets[idx]
	if err := m.deps.Store.Notify().DeleteTarget(context.Background(), t.ID, m.deps.User.ID); err != nil {
		m.err = err.Error()
		return
	}
	m.targets = append(m.targets[:idx], m.targets[idx+1:]...)
	if m.cursor >= m.rowCount() {
		m.cursor = m.rowCount() - 1
	}
	m.flash = i18n.T(localeOf(m.deps), i18n.ScreenNotifyFlashDeleted)
	m.err = ""
}

func (m notifySettingsModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.err = ""
		return m, nil
	case "tab", "down":
		m.cycleEditFocus()
		return m, nil
	case "shift+tab", "up":
		m.cycleEditFocus()
		return m, nil
	case "enter", "ctrl+s":
		return m.submitEdit()
	}
	var cmd tea.Cmd
	if m.editFocus == 0 {
		m.editLabel, cmd = m.editLabel.Update(msg)
	} else {
		m.editURL, cmd = m.editURL.Update(msg)
	}
	return m, cmd
}

func (m *notifySettingsModel) cycleEditFocus() {
	if m.editFocus == 0 {
		m.editFocus = 1
		m.editLabel.Blur()
		m.editURL.Focus()
	} else {
		m.editFocus = 0
		m.editURL.Blur()
		m.editLabel.Focus()
	}
}

func (m notifySettingsModel) submitEdit() (tea.Model, tea.Cmd) {
	if m.deps.User == nil {
		m.err = "internal error: no user"
		return m, nil
	}
	label := strings.TrimSpace(m.editLabel.Value())
	url := strings.TrimSpace(m.editURL.Value())
	if err := store.ValidateNotifyURL(url); err != nil {
		m.err = err.Error()
		return m, nil
	}
	ctx := context.Background()
	loc := localeOf(m.deps)
	if m.editingID == 0 {
		// Insert new.
		id, err := m.deps.Store.Notify().AddTarget(ctx, m.deps.User.ID, label, url)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		// Re-list so created_at reflects the canonical row order.
		ts, _ := m.deps.Store.Notify().ListTargets(ctx, m.deps.User.ID)
		m.targets = ts
		m.flash = i18n.Tf(loc, i18n.ScreenNotifyFlashAdded, id)
	} else {
		// Update existing — preserve enabled state.
		var enabled = true
		for _, t := range m.targets {
			if t.ID == m.editingID {
				enabled = t.Enabled
				break
			}
		}
		if err := m.deps.Store.Notify().UpdateTarget(ctx, m.editingID, m.deps.User.ID, label, url, enabled); err != nil {
			m.err = err.Error()
			return m, nil
		}
		ts, _ := m.deps.Store.Notify().ListTargets(ctx, m.deps.User.ID)
		m.targets = ts
		m.flash = i18n.Tf(loc, i18n.ScreenNotifyFlashUpdated, m.editingID)
	}
	m.mode = modeList
	m.editingID = 0
	m.err = ""
	return m, nil
}

func (m notifySettingsModel) View() string {
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenNotifyTitle)))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
	}

	// Section 1: prefs toggles.
	b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenNotifyEventsHeader)) + "\n")
	prefsRows := []struct {
		label string
		on    bool
		note  string
	}{
		{i18n.T(loc, i18n.ScreenNotifyLabelOnPush), m.prefs.OnPush, i18n.T(loc, i18n.ScreenNotifyHintOnPush)},
		{i18n.T(loc, i18n.ScreenNotifyLabelOnWB), m.prefs.OnWB, i18n.T(loc, i18n.ScreenNotifyHintOnWB)},
		{i18n.T(loc, i18n.ScreenNotifyLabelOnMail), m.prefs.OnMail, i18n.T(loc, i18n.ScreenNotifyHintOnMail)},
		{i18n.T(loc, i18n.ScreenNotifyLabelOnReply), m.prefs.OnReply, i18n.T(loc, i18n.ScreenNotifyHintOnReply)},
		{i18n.T(loc, i18n.ScreenNotifyLabelOffline), m.prefs.OnlyWhenOffline, i18n.T(loc, i18n.ScreenNotifyHintOffline)},
	}
	for i, r := range prefsRows {
		marker := "  "
		check := "[ ]"
		if r.on {
			check = "[x]"
		}
		row := fmt.Sprintf("  %s  %d. %s", check, i+1, r.label)
		if i == m.cursor && m.mode == modeList {
			marker = "▸ "
			row = StyleHighlight.Render(fmt.Sprintf("  %s  %d. %-44s", check, i+1, r.label))
		}
		b.WriteString(marker + row + "  " + StyleDim.Render(r.note) + "\n")
	}

	if m.prefsDirty {
		b.WriteString("\n  " + StyleDim.Render("(prefs unsaved — Ctrl+S to persist)") + "\n")
	}

	// Section 2: target list.
	b.WriteString("\n  " + StyleDim.Render(i18n.T(loc, i18n.ScreenNotifyTargetsHeader)) + "\n")
	if len(m.targets) == 0 {
		b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenNotifyNoTargets)) + "\n")
	} else {
		for i, t := range m.targets {
			rowIdx := notifPrefCount + i
			marker := "  "
			check := "[×]"
			if t.Enabled {
				check = "[✓]"
			}
			label := t.Label
			if label == "" {
				label = "(no label)"
			}
			row := fmt.Sprintf("  %s #%d %s  %s", check, t.ID, PadRight(Truncate(label, 24), 24), Truncate(t.URL, 60))
			if rowIdx == m.cursor && m.mode == modeList {
				marker = "▸ "
				row = StyleHighlight.Render(row)
			}
			b.WriteString(marker + row + "\n")
		}
	}

	// Add row.
	addRow := i18n.T(loc, i18n.ScreenNotifyAddTarget)
	if m.addRowIndex() == m.cursor && m.mode == modeList {
		b.WriteString("▸ " + StyleHighlight.Render(addRow) + "\n")
	} else {
		b.WriteString("  " + addRow + "\n")
	}

	// Inline edit form.
	if m.mode == modeEdit {
		b.WriteString("\n  " + StyleHeader.Render(" Webhook target ") + "\n\n")
		b.WriteString("  " + StyleDim.Render("Label:") + "\n  " + m.editLabel.View() + "\n\n")
		b.WriteString("  " + StyleDim.Render("URL (http:// or https://):") + "\n  " + m.editURL.View() + "\n")
		if m.err != "" {
			b.WriteString("\n  " + StyleError.Render("⚠ "+m.err))
		}
		b.WriteString("\n  " + StyleHelp.Render("Tab cycle field · Enter/Ctrl+S save · Esc cancel"))
		b.WriteString("\n")
		return b.String()
	}

	if m.err != "" {
		b.WriteString("\n  " + StyleError.Render("⚠ "+m.err))
	}
	if m.flash != "" {
		b.WriteString("\n  " + StyleSuccess.Render(m.flash))
	}
	b.WriteString("\n  " + StyleHelp.Render(i18n.T(loc, i18n.ScreenNotifyHelpLine)))
	b.WriteString("\n")
	return b.String()
}
