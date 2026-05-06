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

func TestWBThread_BackKeys(t *testing.T) {
	deps, alice, _ := wbFixture(t, 1)
	for _, key := range []string{"esc", "backspace", "left", "h"} {
		t.Run(key, func(t *testing.T) {
			m := newWBThreadModel(deps, alice.ID)
			_, cmd := m.Update(keyOf(key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok || nav.To != ScreenWBInbox {
				t.Errorf("got %+v, want NavigateMsg{To: ScreenWBInbox}", nav)
			}
		})
	}
}

func TestWBThread_QuitKey(t *testing.T) {
	deps, alice, _ := wbFixture(t, 1)
	m := newWBThreadModel(deps, alice.ID)
	_, cmd := m.Update(keyOf("Q"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenMainMenu {
		t.Errorf("got %+v, want NavigateMsg{To: ScreenMainMenu}", nav)
	}
}

// c and r both open compose pre-filled with the counterparty handle.
func TestWBThread_ComposeKey(t *testing.T) {
	deps, alice, _ := wbFixture(t, 1)
	for _, key := range []string{"c", "r"} {
		t.Run(key, func(t *testing.T) {
			m := newWBThreadModel(deps, alice.ID)
			_, cmd := m.Update(keyOf(key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok || nav.To != ScreenWBCompose {
				t.Fatalf("got %+v, want NavigateMsg{To: ScreenWBCompose}", nav)
			}
			if nav.Recipient != "alice" {
				t.Errorf("Recipient = %q, want alice", nav.Recipient)
			}
		})
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

// TestWBThread_EmptyThread doesn't crash and shows the placeholder.
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
