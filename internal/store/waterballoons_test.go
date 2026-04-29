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
