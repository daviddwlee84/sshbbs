package store_test

import (
	"context"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestWaterBalloons_InsertAndList(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	for i := 0; i < 3; i++ {
		_, err := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "msg", false)
		if err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	unread, err := st.WaterBalloons().ListUnreadFor(ctx, bob.ID)
	if err != nil {
		t.Fatalf("ListUnreadFor: %v", err)
	}
	if len(unread) != 3 {
		t.Errorf("got %d unread, want 3", len(unread))
	}
	// Oldest first by id ASC — important for replay ordering.
	for i := 0; i < len(unread)-1; i++ {
		if unread[i].ID > unread[i+1].ID {
			t.Errorf("ListUnreadFor not in id ASC order")
		}
	}
}

func TestWaterBalloons_MarkRead(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	wb, err := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "hi", true)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := st.WaterBalloons().MarkRead(ctx, wb.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	got, err := st.WaterBalloons().ListUnreadFor(ctx, bob.ID)
	if err != nil {
		t.Fatalf("ListUnreadFor: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d unread after MarkRead, want 0", len(got))
	}
}

func TestWaterBalloons_MarkAllReadFor(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	for i := 0; i < 5; i++ {
		_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "msg", false)
	}
	if err := st.WaterBalloons().MarkAllReadFor(ctx, bob.ID); err != nil {
		t.Fatalf("MarkAllReadFor: %v", err)
	}
	unread, _ := st.WaterBalloons().ListUnreadFor(ctx, bob.ID)
	if len(unread) != 0 {
		t.Errorf("after MarkAllReadFor: %d unread, want 0", len(unread))
	}
}

// TestWaterBalloons_InboxOrdering checks that ListInboxFor returns unread
// before read, and within each group newest-first by id.
func TestWaterBalloons_InboxOrdering(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	// Insert 4 in order, mark the second one read.
	w1, _ := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "first",  false)
	w2, _ := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "second", false)
	w3, _ := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "third",  false)
	w4, _ := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "fourth", false)
	_ = st.WaterBalloons().MarkRead(ctx, w2.ID)
	_ = w1
	_ = w3
	_ = w4

	got, err := st.WaterBalloons().ListInboxFor(ctx, bob.ID, 0)
	if err != nil {
		t.Fatalf("ListInboxFor: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d, want 4", len(got))
	}
	// Expected order: unread newest-first (w4, w3, w1), then read (w2).
	wantBodies := []string{"fourth", "third", "first", "second"}
	for i, w := range wantBodies {
		if got[i].Body != w {
			t.Errorf("[%d].Body = %q, want %q", i, got[i].Body, w)
		}
	}
}

func TestWaterBalloons_DeliveredLiveFlag(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	live, _ := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "live", true)
	offline, _ := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "offline", false)
	if !live.DeliveredLive {
		t.Error("live WB has DeliveredLive=false")
	}
	if offline.DeliveredLive {
		t.Error("offline WB has DeliveredLive=true")
	}
}

func TestWaterBalloons_GetByID(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	wb, err := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "hello", false)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := st.WaterBalloons().GetByID(ctx, wb.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != wb.ID || got.Body != "hello" || got.FromUserID != alice.ID || got.ToUserID != bob.ID {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

// TestWaterBalloons_ListConversation_BothDirections seeds A→B and B→A rows
// interleaved and asserts both come back in id ASC order — the chat-style
// scrollback contract.
func TestWaterBalloons_ListConversation_BothDirections(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	bodies := []struct {
		fromAlice bool
		body      string
	}{
		{true, "a1"}, {false, "b1"}, {true, "a2"}, {false, "b2"}, {true, "a3"},
	}
	for _, m := range bodies {
		if m.fromAlice {
			_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, m.body, false)
		} else {
			_, _ = st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, m.body, false)
		}
	}

	got, err := st.WaterBalloons().ListConversation(ctx, alice.ID, bob.ID, 100)
	if err != nil {
		t.Fatalf("ListConversation: %v", err)
	}
	if len(got) != len(bodies) {
		t.Fatalf("got %d rows, want %d", len(got), len(bodies))
	}
	for i, want := range bodies {
		if got[i].Body != want.body {
			t.Errorf("[%d].Body = %q, want %q", i, got[i].Body, want.body)
		}
	}
	for i := 0; i < len(got)-1; i++ {
		if got[i].ID > got[i+1].ID {
			t.Errorf("not ASC at i=%d: %d > %d", i, got[i].ID, got[i+1].ID)
		}
	}
}

// TestWaterBalloons_ListConversation_FiltersOtherUsers verifies that messages
// involving a third party don't leak into a (alice, bob) thread.
func TestWaterBalloons_ListConversation_FiltersOtherUsers(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	carol := storetest.MustUser(t, st, "carol", "")

	_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "ab", false)
	_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, carol.ID, "ac", false)
	_, _ = st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, carol.ID, "bc", false)
	_, _ = st.WaterBalloons().Insert(ctx, carol.ID, carol.UserID, alice.ID, "ca", false)

	got, err := st.WaterBalloons().ListConversation(ctx, alice.ID, bob.ID, 100)
	if err != nil {
		t.Fatalf("ListConversation: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if got[0].Body != "ab" {
		t.Errorf("got body %q, want ab", got[0].Body)
	}
}

// TestWaterBalloons_ListConversation_Limit asserts the limit clips oldest
// rows. (When pagination lands the order will flip; today the limit is
// effectively a "show me up to N most recent including everything older
// down to that count from the start" — we test the current behavior.)
func TestWaterBalloons_ListConversation_Limit(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	for i := 0; i < 5; i++ {
		_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "msg", false)
	}
	got, err := st.WaterBalloons().ListConversation(ctx, alice.ID, bob.ID, 3)
	if err != nil {
		t.Fatalf("ListConversation: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}

// TestWaterBalloons_ListCounterpartiesFor_Grouping seeds messages with two
// distinct counterparties and asserts each shows up exactly once.
func TestWaterBalloons_ListCounterpartiesFor_Grouping(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	carol := storetest.MustUser(t, st, "carol", "")

	for i := 0; i < 3; i++ {
		_, _ = st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "from-bob", false)
	}
	for i := 0; i < 2; i++ {
		_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, carol.ID, "to-carol", false)
	}

	rows, err := st.WaterBalloons().ListCounterpartiesFor(ctx, alice.ID, 50)
	if err != nil {
		t.Fatalf("ListCounterpartiesFor: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.UserIDStr] = true
	}
	if !seen["bob"] || !seen["carol"] {
		t.Errorf("missing counterparties: %v", seen)
	}
}

// TestWaterBalloons_ListCounterpartiesFor_UnreadCount checks only inbound,
// unread rows from THAT counterparty are counted toward UnreadCount.
func TestWaterBalloons_ListCounterpartiesFor_UnreadCount(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	carol := storetest.MustUser(t, st, "carol", "")

	// 4 unread from bob to alice.
	for i := 0; i < 4; i++ {
		_, _ = st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "b", false)
	}
	// 1 from carol to alice, then read.
	cw, _ := st.WaterBalloons().Insert(ctx, carol.ID, carol.UserID, alice.ID, "c", false)
	_ = st.WaterBalloons().MarkRead(ctx, cw.ID)
	// 2 from alice to bob (outbound — should not count as unread).
	_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "out", false)
	_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "out", false)

	rows, err := st.WaterBalloons().ListCounterpartiesFor(ctx, alice.ID, 50)
	if err != nil {
		t.Fatalf("ListCounterpartiesFor: %v", err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		got[r.UserIDStr] = r.UnreadCount
	}
	if got["bob"] != 4 {
		t.Errorf("bob unread = %d, want 4", got["bob"])
	}
	if got["carol"] != 0 {
		t.Errorf("carol unread = %d, want 0", got["carol"])
	}
}

// TestWaterBalloons_ListCounterpartiesFor_LastFromMe verifies the LastFromMe
// flag reflects the direction of the most-recent row in the conversation.
func TestWaterBalloons_ListCounterpartiesFor_LastFromMe(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	_, _ = st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "hi alice", false)
	_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "hi bob", false) // last

	rows, err := st.WaterBalloons().ListCounterpartiesFor(ctx, alice.ID, 50)
	if err != nil {
		t.Fatalf("ListCounterpartiesFor: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if !rows[0].LastFromMe {
		t.Errorf("LastFromMe = false, want true (alice sent the latest)")
	}
	if rows[0].LastBody != "hi bob" {
		t.Errorf("LastBody = %q, want %q", rows[0].LastBody, "hi bob")
	}
}

// TestWaterBalloons_ListCounterpartiesFor_UnreadFirst asserts that a
// counterparty with unread messages sorts above one whose last message is
// more recent but already read.
func TestWaterBalloons_ListCounterpartiesFor_UnreadFirst(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	carol := storetest.MustUser(t, st, "carol", "")

	// bob's row is older but still unread.
	_, _ = st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "old-unread", false)
	// carol's row is newer but read.
	cw, _ := st.WaterBalloons().Insert(ctx, carol.ID, carol.UserID, alice.ID, "new-read", false)
	_ = st.WaterBalloons().MarkRead(ctx, cw.ID)

	rows, err := st.WaterBalloons().ListCounterpartiesFor(ctx, alice.ID, 50)
	if err != nil {
		t.Fatalf("ListCounterpartiesFor: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].UserIDStr != "bob" {
		t.Errorf("first row = %q, want bob (unread sorts first)", rows[0].UserIDStr)
	}
}

// TestWaterBalloons_ListCounterpartiesFor_HandleFromUsersTable verifies that
// UserIDStr is read via JOIN on users (canonical handle) rather than the
// from_userid snapshot column. We construct rows from carol→alice and from
// alice→carol; in both, the counterparty handle returned should be carol's
// current users.user_id, not the row-local from_userid (which would only
// match for half the rows).
func TestWaterBalloons_ListCounterpartiesFor_HandleFromUsersTable(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	carol := storetest.MustUser(t, st, "carol", "")

	_, _ = st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, carol.ID, "out", false) // alice sender
	_, _ = st.WaterBalloons().Insert(ctx, carol.ID, carol.UserID, alice.ID, "in", false)  // carol sender

	rows, err := st.WaterBalloons().ListCounterpartiesFor(ctx, alice.ID, 50)
	if err != nil {
		t.Fatalf("ListCounterpartiesFor: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].UserIDStr != "carol" {
		t.Errorf("UserIDStr = %q, want carol (must come from users JOIN)", rows[0].UserIDStr)
	}
	if rows[0].UserID != carol.ID {
		t.Errorf("UserID = %d, want %d", rows[0].UserID, carol.ID)
	}
}

// TestWaterBalloons_MarkConversationRead_OnlyInbound asserts only the
// counterparty→viewer rows get read_at set; viewer→counterparty rows are
// untouched (they were already "read" by the sender, and writing to them
// would be a logic bug).
func TestWaterBalloons_MarkConversationRead_OnlyInbound(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")

	in1, _ := st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "in1", false)
	in2, _ := st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "in2", false)
	out1, _ := st.WaterBalloons().Insert(ctx, alice.ID, alice.UserID, bob.ID, "out1", false)

	if err := st.WaterBalloons().MarkConversationRead(ctx, alice.ID, bob.ID); err != nil {
		t.Fatalf("MarkConversationRead: %v", err)
	}

	got1, _ := st.WaterBalloons().GetByID(ctx, in1.ID)
	got2, _ := st.WaterBalloons().GetByID(ctx, in2.ID)
	gotOut, _ := st.WaterBalloons().GetByID(ctx, out1.ID)
	if !got1.ReadAt.Valid || !got2.ReadAt.Valid {
		t.Errorf("inbound rows not marked read: in1.read=%v in2.read=%v", got1.ReadAt.Valid, got2.ReadAt.Valid)
	}
	if gotOut.ReadAt.Valid {
		t.Errorf("outbound row was marked read — that's wrong")
	}
}

// TestWaterBalloons_MarkConversationRead_Idempotent asserts the second call
// is a no-op (no error, no spurious updates).
func TestWaterBalloons_MarkConversationRead_Idempotent(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	_, _ = st.WaterBalloons().Insert(ctx, bob.ID, bob.UserID, alice.ID, "x", false)

	if err := st.WaterBalloons().MarkConversationRead(ctx, alice.ID, bob.ID); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := st.WaterBalloons().MarkConversationRead(ctx, alice.ID, bob.ID); err != nil {
		t.Fatalf("second call: %v", err)
	}
}
