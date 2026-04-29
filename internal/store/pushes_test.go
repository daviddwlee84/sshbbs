package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestPushes_ScoreDeltas(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	art, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "t", "b")

	cases := []struct {
		kind      store.PushKind
		userID    int64
		wantScore int64
	}{
		{store.PushKindPush, bob.ID, 1},
		{store.PushKindPush, alice.ID, 2},
		{store.PushKindBoo, bob.ID, 1},
		{store.PushKindArrow, alice.ID, 1}, // arrow = 0 delta
		{store.PushKindBoo, bob.ID, 0},
		{store.PushKindBoo, bob.ID, -1},
	}
	for i, tc := range cases {
		if _, err := st.Pushes().Create(ctx, art.ID, tc.userID, "x", tc.kind, "msg"); err != nil {
			t.Fatalf("[%d] Create: %v", i, err)
		}
		fresh, err := st.Articles().GetByID(ctx, art.ID)
		if err != nil {
			t.Fatalf("[%d] GetByID: %v", i, err)
		}
		if fresh.RecommendScore != tc.wantScore {
			t.Errorf("[%d] kind=%s: score = %d, want %d", i, tc.kind, fresh.RecommendScore, tc.wantScore)
		}
	}
}

func TestPushes_ListPreservesInsertionOrder(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	art, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "t", "b")

	bodies := []string{"first push", "second push", "third push"}
	for _, body := range bodies {
		if _, err := st.Pushes().Create(ctx, art.ID, alice.ID, alice.UserID, store.PushKindPush, body); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	got, err := st.Pushes().ListByArticle(ctx, art.ID)
	if err != nil {
		t.Fatalf("ListByArticle: %v", err)
	}
	if len(got) != len(bodies) {
		t.Fatalf("got %d, want %d", len(got), len(bodies))
	}
	for i, want := range bodies {
		if got[i].Body != want {
			t.Errorf("[%d].Body = %q, want %q", i, got[i].Body, want)
		}
	}
}

func TestPushes_InvalidKind(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	art, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "t", "b")
	if _, err := st.Pushes().Create(ctx, art.ID, alice.ID, alice.UserID, "bogus", "x"); err == nil {
		t.Error("expected error for invalid kind, got nil")
	}
}

// TestPushes_ConcurrentScoreAtomicity asserts that N concurrent push inserts
// against the same article yield exactly N as the cached score — no lost
// updates from interleaved RMW. Run with `go test -race`.
func TestPushes_ConcurrentScoreAtomicity(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	art, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "t", "b")

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if _, err := st.Pushes().Create(ctx, art.ID, alice.ID, alice.UserID, store.PushKindPush, "x"); err != nil {
				t.Errorf("Create: %v", err)
			}
		}()
	}
	wg.Wait()

	fresh, err := st.Articles().GetByID(ctx, art.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fresh.RecommendScore != int64(N) {
		t.Errorf("score = %d, want %d (lost update?)", fresh.RecommendScore, N)
	}
	pushes, _ := st.Pushes().ListByArticle(ctx, art.ID)
	if len(pushes) != N {
		t.Errorf("inserted %d, found %d in DB", N, len(pushes))
	}
}
