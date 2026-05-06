package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// wbFixture builds a store with two users and returns a Deps for `bob`,
// the recipient. seedUnread is the number of unread water balloons from
// alice to bob to insert.
func wbFixture(t *testing.T, seedUnread int) (Deps, *store.User /*alice*/, *store.User /*bob*/) {
	t.Helper()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	for i := 0; i < seedUnread; i++ {
		_, err := st.WaterBalloons().Insert(context.Background(), alice.ID, alice.UserID, bob.ID, "msg", false)
		if err != nil {
			t.Fatalf("seed wb %d: %v", i, err)
		}
	}
	deps := Deps{Store: st, User: bob, Broker: chat.NewBroker()}
	return deps, alice, bob
}

// =====================================================================
// Inbox (counterparty roll-up)
// =====================================================================

func TestWBInbox_LoadsCounterparties(t *testing.T) {
	deps, _, _ := wbFixture(t, 3)
	m := newWBInboxModel(deps)
	if m.loadErr != nil {
		t.Fatalf("loadErr = %v", m.loadErr)
	}
	if len(m.items) != 1 {
		t.Errorf("got %d counterparties, want 1", len(m.items))
	}
	if len(m.items) > 0 && m.items[0].UnreadCount != 3 {
		t.Errorf("unread = %d, want 3", m.items[0].UnreadCount)
	}
}

// TestWBInbox_NoAutoMarkRead is a regression test for the UX change: opening
// the inbox no longer marks inbound messages as read. (Old behaviour
// `MarkAllReadFor` on construction was removed; mark-read now happens only
// when entering a specific thread.)
func TestWBInbox_NoAutoMarkRead(t *testing.T) {
	deps, alice, bob := wbFixture(t, 3)
	_ = alice
	_ = newWBInboxModel(deps)
	unread, err := deps.Store.WaterBalloons().ListUnreadFor(context.Background(), bob.ID)
	if err != nil {
		t.Fatalf("ListUnreadFor: %v", err)
	}
	if len(unread) != 3 {
		t.Errorf("after opening inbox: %d unread, want 3 (must NOT auto-mark)", len(unread))
	}
}

func TestWBInbox_BackKeys(t *testing.T) {
	for _, key := range []string{"esc", "backspace", "left", "h"} {
		t.Run(key, func(t *testing.T) {
			deps, _, _ := wbFixture(t, 1)
			m := newWBInboxModel(deps)
			_, cmd := m.Update(keyOf(key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok || nav.To != ScreenMainMenu {
				t.Errorf("got %+v, want NavigateMsg{To: ScreenMainMenu}", nav)
			}
		})
	}
}

// Inbox Enter/right/l/r open the thread for the cursored counterparty.
func TestWBInbox_OpenThread(t *testing.T) {
	for _, key := range []string{"enter", " ", "right", "l", "r"} {
		t.Run(key, func(t *testing.T) {
			deps, alice, _ := wbFixture(t, 1)
			m := newWBInboxModel(deps)
			_, cmd := m.Update(keyOf(key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok || nav.To != ScreenWBThread {
				t.Fatalf("got %+v, want NavigateMsg{To: ScreenWBThread}", nav)
			}
			if nav.CounterpartyUserID != alice.ID {
				t.Errorf("CounterpartyUserID = %d, want %d", nav.CounterpartyUserID, alice.ID)
			}
		})
	}
}

func TestWBInbox_ComposeKey(t *testing.T) {
	deps, _, _ := wbFixture(t, 0)
	m := newWBInboxModel(deps)
	_, cmd := m.Update(keyOf("c"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenWBCompose {
		t.Errorf("got %+v, want NavigateMsg{To: ScreenWBCompose}", nav)
	}
	if nav.Recipient != "" {
		t.Errorf("c should not pre-fill recipient, got %q", nav.Recipient)
	}
}

func TestWBInbox_NoopsWhenEmpty(t *testing.T) {
	deps, _, _ := wbFixture(t, 0)
	m := newWBInboxModel(deps)
	for _, key := range []string{"enter", " ", "right", "l", "r"} {
		_, cmd := m.Update(keyOf(key))
		if cmd != nil {
			if msg := runCmd(cmd); msg != nil {
				t.Errorf("key %q on empty inbox produced %T %+v, want nil", key, msg, msg)
			}
		}
	}
}

// =====================================================================
// Thread
// =====================================================================

func TestWBThread_LoadsBothDirections(t *testing.T) {
	deps, alice, bob := wbFixture(t, 0)
	ctx := context.Background()
	_, _ = deps.Store.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "a1", false)
	_, _ = deps.Store.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "b1", false)
	_, _ = deps.Store.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "a2", false)

	m := newWBThreadModel(deps, alice.ID)
	if m.loadErr != nil {
		t.Fatalf("loadErr = %v", m.loadErr)
	}
	if len(m.items) != 3 {
		t.Fatalf("got %d items, want 3", len(m.items))
	}
	wantBodies := []string{"a1", "b1", "a2"}
	for i, b := range wantBodies {
		if m.items[i].Body != b {
			t.Errorf("[%d].Body = %q, want %q", i, m.items[i].Body, b)
		}
	}
	if m.cpUserID != "alice" {
		t.Errorf("cpUserID = %q, want alice", m.cpUserID)
	}
}

// TestWBThread_OpeningMarksReadInbound asserts entering a thread marks only
// the inbound (alice→bob) rows as read; outbound (bob→alice) rows untouched.
func TestWBThread_OpeningMarksReadInbound(t *testing.T) {
	deps, alice, bob := wbFixture(t, 0)
	ctx := context.Background()
	in1, _ := deps.Store.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "in1", false)
	in2, _ := deps.Store.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "in2", false)
	out, _ := deps.Store.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "out", false)

	_ = newWBThreadModel(deps, alice.ID)

	got1, _ := deps.Store.WaterBalloons().GetByID(ctx, in1.ID)
	got2, _ := deps.Store.WaterBalloons().GetByID(ctx, in2.ID)
	gotOut, _ := deps.Store.WaterBalloons().GetByID(ctx, out.ID)
	if !got1.ReadAt.Valid || !got2.ReadAt.Valid {
		t.Errorf("inbound rows not marked: in1.read=%v in2.read=%v", got1.ReadAt.Valid, got2.ReadAt.Valid)
	}
	if gotOut.ReadAt.Valid {
		t.Error("outbound row was marked read on thread open — wrong")
	}
}

// While input is focused (the default), only Esc maps to back. The other
// vim-style "back" keys (backspace/left/h) are typed into the input. Tab
// switches focus to the scrollback, where the full keymap applies.
func TestWBThread_BackKeys_InputFocused(t *testing.T) {
	deps, alice, _ := wbFixture(t, 1)
	m := newWBThreadModel(deps, alice.ID)
	_, cmd := m.Update(keyOf("esc"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenWBInbox {
		t.Errorf("esc → %+v, want NavigateMsg{To: ScreenWBInbox}", nav)
	}
}

func TestWBThread_BackKeys_ScrollbackFocused(t *testing.T) {
	deps, alice, _ := wbFixture(t, 1)
	for _, key := range []string{"esc", "backspace", "left", "h"} {
		t.Run(key, func(t *testing.T) {
			m := newWBThreadModel(deps, alice.ID)
			updated, _ := m.Update(keyOf("tab")) // focus scrollback
			m = updated.(wbThreadModel)
			_, cmd := m.Update(keyOf(key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok || nav.To != ScreenWBInbox {
				t.Errorf("got %+v, want NavigateMsg{To: ScreenWBInbox}", nav)
			}
		})
	}
}

// 'Q' is treated as a literal char while input is focused; only fires
// after Tab.
func TestWBThread_QuitKey_RequiresScrollbackFocus(t *testing.T) {
	deps, alice, _ := wbFixture(t, 1)
	m := newWBThreadModel(deps, alice.ID)
	updated, _ := m.Update(keyOf("tab"))
	m = updated.(wbThreadModel)
	_, cmd := m.Update(keyOf("Q"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenMainMenu {
		t.Errorf("got %+v, want NavigateMsg{To: ScreenMainMenu}", nav)
	}
}

// TestWBThread_TabTogglesFocus checks Tab cycles input ↔ scrollback.
func TestWBThread_TabTogglesFocus(t *testing.T) {
	deps, alice, _ := wbFixture(t, 1)
	m := newWBThreadModel(deps, alice.ID)
	if !m.focusInput {
		t.Fatal("default focus should be input")
	}
	updated, _ := m.Update(keyOf("tab"))
	m = updated.(wbThreadModel)
	if m.focusInput {
		t.Error("Tab from input did not flip focus")
	}
	updated, _ = m.Update(keyOf("tab"))
	m = updated.(wbThreadModel)
	if !m.focusInput {
		t.Error("Tab from scrollback did not flip focus back")
	}
}

// TestWBThread_SendOnEnter types a body and presses Enter; expects the
// thread to grow by one row, the textinput to clear, and the row to
// persist via the existing Insert path.
func TestWBThread_SendOnEnter(t *testing.T) {
	deps, alice, _ := wbFixture(t, 0)
	m := newWBThreadModel(deps, alice.ID)
	m.input.SetValue("hello alice")
	updated, _ := m.Update(keyOf("enter"))
	tm := updated.(wbThreadModel)
	if len(tm.items) != 1 {
		t.Fatalf("got %d items after send, want 1", len(tm.items))
	}
	if tm.items[0].Body != "hello alice" {
		t.Errorf("appended body = %q, want %q", tm.items[0].Body, "hello alice")
	}
	if tm.input.Value() != "" {
		t.Errorf("input not cleared after send: %q", tm.input.Value())
	}
	got, err := deps.Store.WaterBalloons().GetByID(context.Background(), tm.items[0].ID)
	if err != nil || got.Body != "hello alice" {
		t.Errorf("DB row missing or wrong: %+v err=%v", got, err)
	}
}

// TestWBThread_SendEmptyNoop guards against accidental empty inserts.
func TestWBThread_SendEmptyNoop(t *testing.T) {
	deps, alice, _ := wbFixture(t, 0)
	m := newWBThreadModel(deps, alice.ID)
	updated, _ := m.Update(keyOf("enter"))
	tm := updated.(wbThreadModel)
	if len(tm.items) != 0 {
		t.Errorf("empty Enter created %d items, want 0", len(tm.items))
	}
}

// TestWBThread_TypingForwardsToInput sends a regular character key; the
// textinput should receive it without any navigation side-effect.
func TestWBThread_TypingForwardsToInput(t *testing.T) {
	deps, alice, _ := wbFixture(t, 0)
	m := newWBThreadModel(deps, alice.ID)
	updated, _ := m.Update(keyOf("h"))
	tm := updated.(wbThreadModel)
	if tm.input.Value() != "h" {
		t.Errorf("input value = %q, want %q (h should not navigate while input focused)", tm.input.Value(), "h")
	}
}

// TestWBThread_AllowsSelfThread confirms self-threads open normally and
// the header surfaces a "yourself / memo" marker so users see they're in
// a self-conversation, not a generic chat. The bubbletea-deadlock concern
// is sidestepped at submit time (skip Broker.Send when cpID == User.ID),
// not by refusing construction.
func TestWBThread_AllowsSelfThread(t *testing.T) {
	deps, _, bob := wbFixture(t, 0)
	m := newWBThreadModel(deps, bob.ID) // bob is deps.User
	if m.loadErr != nil {
		t.Fatalf("self-thread should open, got loadErr=%v", m.loadErr)
	}
	if m.cpID != bob.ID {
		t.Errorf("cpID = %d, want %d", m.cpID, bob.ID)
	}
	v := m.View()
	if !strings.Contains(v, "yourself") {
		t.Errorf("self-thread View missing 'yourself' marker; got:\n%s", v)
	}
}

// TestWBThread_SelfSubmitSkipsBroker is the load-bearing test: a self-DM
// must Insert into the DB and append locally WITHOUT calling Broker.Send
// (that's the deadlock — see pitfall doc). We can't easily assert "broker
// not called" with the real broker, but we can prove the row is persisted
// AND already marked read (which the non-self path only does after a
// successful broker.Send returns delivered=true).
func TestWBThread_SelfSubmitSkipsBroker(t *testing.T) {
	deps, _, bob := wbFixture(t, 0)
	m := newWBThreadModel(deps, bob.ID) // self
	m.input.SetValue("buy milk")
	updated, _ := m.submit()
	tm := updated.(wbThreadModel)

	if len(tm.items) != 1 {
		t.Fatalf("self-submit appended %d items, want 1", len(tm.items))
	}
	if tm.items[0].Body != "buy milk" {
		t.Errorf("body = %q, want %q", tm.items[0].Body, "buy milk")
	}
	got, _ := deps.Store.WaterBalloons().GetByID(t.Context(), tm.items[0].ID)
	if got == nil || got.FromUserID != bob.ID || got.ToUserID != bob.ID {
		t.Errorf("DB row mis-shaped for self-WB: %+v", got)
	}
	if !got.ReadAt.Valid {
		t.Error("self-WB not marked read on insert — would replay as toast on reconnect")
	}
}

// =====================================================================
// Compose self-send (memo)
// =====================================================================

// TestWBCompose_AllowsSelfMemo: self-recipient succeeds, persists, and
// auto-marks-read so it doesn't replay on reconnect. The guard against
// the original hang is now "skip Broker.Send when target==self" rather
// than "refuse the operation". (Pre-fix this test would have deadlocked
// the goroutine running it; the fix is verified by the test completing.)
func TestWBCompose_AllowsSelfMemo(t *testing.T) {
	deps, _, bob := wbFixture(t, 0)
	m := newWBComposeModel(deps, "")
	m.to.SetValue("bob") // bob is deps.User
	m.body.SetValue("remember the milk")
	updated, _ := m.submit()
	cm := updated.(wbComposeModel)
	if cm.err != "" {
		t.Fatalf("self-memo errored: %q", cm.err)
	}
	if cm.sent != "bob" {
		t.Errorf("sent = %q, want bob", cm.sent)
	}
	rows, _ := deps.Store.WaterBalloons().ListConversation(t.Context(), bob.ID, bob.ID, 10)
	if len(rows) != 1 {
		t.Fatalf("got %d self-WB rows, want 1", len(rows))
	}
	if !rows[0].ReadAt.Valid {
		t.Error("self-memo not marked read on insert — would replay as toast on reconnect")
	}
}

// TestWBThread_RendersCurrentHandle ensures the sender label uses the
// current handle from cpUserID (resolved via users JOIN at construction)
// rather than each row's from_userid snapshot. We can't easily simulate a
// rename without raw SQL access, so we test the render path directly: the
// view should print "alice" for inbound rows.
func TestWBThread_RendersCurrentHandle(t *testing.T) {
	deps, alice, _ := wbFixture(t, 1)
	m := newWBThreadModel(deps, alice.ID)
	v := m.View()
	if !strings.Contains(v, "alice") {
		t.Errorf("View missing counterparty handle 'alice'; got:\n%s", v)
	}
}

// TestWBThread_EmptyThread doesn't crash and shows the placeholder along
// with the input row (you can type to start a conversation).
func TestWBThread_EmptyThread(t *testing.T) {
	deps, alice, _ := wbFixture(t, 0)
	m := newWBThreadModel(deps, alice.ID)
	if m.loadErr != nil {
		t.Fatalf("loadErr = %v", m.loadErr)
	}
	v := m.View()
	if !strings.Contains(v, "empty thread") {
		t.Errorf("View missing empty-thread placeholder; got:\n%s", v)
	}
}

// TestWBThread_LiveAppendOnIncoming feeds a WBIncomingMsg whose FromUserID
// matches the counterparty and asserts (a) the row is appended and (b) the
// underlying DB row gets marked read.
func TestWBThread_LiveAppendOnIncoming(t *testing.T) {
	deps, alice, bob := wbFixture(t, 0)
	ctx := context.Background()
	m := newWBThreadModel(deps, alice.ID)
	if len(m.items) != 0 {
		t.Fatalf("expected empty thread to start, got %d", len(m.items))
	}

	// Simulate alice sending bob a new wb: insert + dispatch the broker msg.
	wb, err := deps.Store.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "live", false)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	updated, _ := m.Update(WBIncomingMsg{ID: wb.ID, FromUserID: "alice", Body: "live"})
	tm, ok := updated.(wbThreadModel)
	if !ok {
		t.Fatalf("Update returned %T, want wbThreadModel", updated)
	}
	if len(tm.items) != 1 {
		t.Errorf("got %d items after live append, want 1", len(tm.items))
	}
	got, _ := deps.Store.WaterBalloons().GetByID(ctx, wb.ID)
	if !got.ReadAt.Valid {
		t.Error("live-appended wb not marked read in DB")
	}
}

// TestWBThread_LiveAppendIgnoresOtherCounterparty fires a WBIncomingMsg
// from a different sender and asserts the thread is untouched.
func TestWBThread_LiveAppendIgnoresOtherCounterparty(t *testing.T) {
	deps, alice, bob := wbFixture(t, 0)
	ctx := context.Background()
	carol := storetest.MustUser(t, deps.Store, "carol", "")

	m := newWBThreadModel(deps, alice.ID)
	wb, _ := deps.Store.WaterBalloons().Insert(ctx, carol.ID, carol.UserID, bob.ID, "from carol", false)

	updated, _ := m.Update(WBIncomingMsg{ID: wb.ID, FromUserID: "carol", Body: "from carol"})
	tm := updated.(wbThreadModel)
	if len(tm.items) != 0 {
		t.Errorf("thread with alice should not absorb a wb from carol, got %d items", len(tm.items))
	}
	// And the carol→bob row must NOT have been marked read by this path.
	got, _ := deps.Store.WaterBalloons().GetByID(ctx, wb.ID)
	if got.ReadAt.Valid {
		t.Error("non-matching wb was marked read — wrong")
	}
}
