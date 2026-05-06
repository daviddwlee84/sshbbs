package store_test

import (
	"context"
	"errors"
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

// Delete: owner can remove their own push; non-owner non-mod refused;
// mod/admin can remove anyone's. Score is reverted symmetrically with
// the kind's create-time delta.
func TestPushes_Delete_Permissions(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	art, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "t", "b")

	insertPush := func(kind store.PushKind, by *store.User) *store.Push {
		p, err := st.Pushes().Create(ctx, art.ID, by.ID, by.UserID, kind, "msg")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return p
	}

	// Owner can delete their own.
	p := insertPush(store.PushKindPush, alice)
	if err := st.Pushes().Delete(ctx, p.ID, alice.ID, store.RoleUser); err != nil {
		t.Errorf("owner delete: %v", err)
	}

	// Non-owner non-mod denied.
	p = insertPush(store.PushKindPush, alice)
	if err := st.Pushes().Delete(ctx, p.ID, bob.ID, store.RoleUser); !errors.Is(err, store.ErrPermissionDenied) {
		t.Errorf("non-owner: got %v, want ErrPermissionDenied", err)
	}

	// Mod can delete anyone's.
	if err := st.Pushes().Delete(ctx, p.ID, bob.ID, store.RoleMod); err != nil {
		t.Errorf("mod delete: %v", err)
	}

	// Admin can delete anyone's.
	p = insertPush(store.PushKindBoo, alice)
	if err := st.Pushes().Delete(ctx, p.ID, bob.ID, store.RoleAdmin); err != nil {
		t.Errorf("admin delete: %v", err)
	}

	// Not found.
	if err := st.Pushes().Delete(ctx, 99999, alice.ID, store.RoleAdmin); !errors.Is(err, store.ErrPushNotFound) {
		t.Errorf("missing push: got %v, want ErrPushNotFound", err)
	}
}

// Delete reverts the score symmetrically with create's delta.
// Push +1 → delete -1, boo -1 → delete +1, arrow 0 → delete 0.
func TestPushes_Delete_RevertsScore(t *testing.T) {
	cases := []struct {
		kind          store.PushKind
		preDeleteWant int64 // recommend_score after Create
	}{
		{store.PushKindPush, 1},
		{store.PushKindBoo, -1},
		{store.PushKindArrow, 0},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			ctx := context.Background()
			st := storetest.New(t)
			alice := storetest.MustUser(t, st, "alice", "")
			board := storetest.MustBoard(t, st, "Test")
			art, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "t", "b")

			p, err := st.Pushes().Create(ctx, art.ID, alice.ID, alice.UserID, tc.kind, "msg")
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			pre, _ := st.Articles().GetByID(ctx, art.ID)
			if pre.RecommendScore != tc.preDeleteWant {
				t.Fatalf("after Create: score = %d, want %d", pre.RecommendScore, tc.preDeleteWant)
			}
			if err := st.Pushes().Delete(ctx, p.ID, alice.ID, store.RoleUser); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			post, _ := st.Articles().GetByID(ctx, art.ID)
			if post.RecommendScore != 0 {
				t.Errorf("after Delete: score = %d, want 0 (reverted)", post.RecommendScore)
			}
		})
	}
}

// TestPushes_Delete_ConcurrentScoreAtomicity mirrors the Create canary:
// 50 simultaneous deletes must net to a score of 0 (each starts +1,
// reverts -1). Run with `go test -race`.
func TestPushes_Delete_ConcurrentScoreAtomicity(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	art, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "t", "b")

	const N = 50
	ids := make([]int64, N)
	for i := 0; i < N; i++ {
		p, err := st.Pushes().Create(ctx, art.ID, alice.ID, alice.UserID, store.PushKindPush, "x")
		if err != nil {
			t.Fatalf("seed Create: %v", err)
		}
		ids[i] = p.ID
	}
	pre, _ := st.Articles().GetByID(ctx, art.ID)
	if pre.RecommendScore != int64(N) {
		t.Fatalf("post-seed score = %d, want %d", pre.RecommendScore, N)
	}

	var wg sync.WaitGroup
	wg.Add(N)
	for _, id := range ids {
		go func(pid int64) {
			defer wg.Done()
			if err := st.Pushes().Delete(ctx, pid, alice.ID, store.RoleUser); err != nil {
				t.Errorf("Delete: %v", err)
			}
		}(id)
	}
	wg.Wait()

	post, _ := st.Articles().GetByID(ctx, art.ID)
	if post.RecommendScore != 0 {
		t.Errorf("after %d concurrent deletes: score = %d, want 0", N, post.RecommendScore)
	}
	left, _ := st.Pushes().ListByArticle(ctx, art.ID)
	if len(left) != 0 {
		t.Errorf("after %d concurrent deletes: %d pushes remain", N, len(left))
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

// TestPushes_Create_RejectsWhenLocked: when the parent article's
// comments_mode is 'locked', Create returns ErrCommentsLocked for every
// kind and does not insert a row or change the score.
func TestPushes_Create_RejectsWhenLocked(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	art, _ := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, "rules", "body")
	if err := st.Articles().SetCommentsMode(ctx, art.ID, mod.ID, store.RoleMod, store.CommentsModeLocked); err != nil {
		t.Fatalf("SetCommentsMode: %v", err)
	}

	for _, kind := range []store.PushKind{store.PushKindPush, store.PushKindBoo, store.PushKindArrow} {
		_, err := st.Pushes().Create(ctx, art.ID, bob.ID, bob.UserID, kind, "msg")
		if !errors.Is(err, store.ErrCommentsLocked) {
			t.Errorf("kind=%s: got %v, want ErrCommentsLocked", kind, err)
		}
	}

	fresh, _ := st.Articles().GetByID(ctx, art.ID)
	if fresh.RecommendScore != 0 {
		t.Errorf("score changed despite all rejections: %d", fresh.RecommendScore)
	}
	pushes, _ := st.Pushes().ListByArticle(ctx, art.ID)
	if len(pushes) != 0 {
		t.Errorf("rows inserted despite rejection: %d", len(pushes))
	}
}

// TestPushes_Create_ArrowsOnly: arrows succeed, push and boo are rejected
// with ErrCommentsArrowsOnly. Score stays at 0 (arrows have zero delta).
func TestPushes_Create_RejectsPushAndBooWhenArrowsOnly(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	art, _ := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, "faq", "body")
	if err := st.Articles().SetCommentsMode(ctx, art.ID, mod.ID, store.RoleMod, store.CommentsModeArrowsOnly); err != nil {
		t.Fatalf("SetCommentsMode: %v", err)
	}

	for _, kind := range []store.PushKind{store.PushKindPush, store.PushKindBoo} {
		_, err := st.Pushes().Create(ctx, art.ID, bob.ID, bob.UserID, kind, "msg")
		if !errors.Is(err, store.ErrCommentsArrowsOnly) {
			t.Errorf("kind=%s: got %v, want ErrCommentsArrowsOnly", kind, err)
		}
	}

	if _, err := st.Pushes().Create(ctx, art.ID, bob.ID, bob.UserID, store.PushKindArrow, "ok"); err != nil {
		t.Errorf("arrow rejected: %v", err)
	}

	fresh, _ := st.Articles().GetByID(ctx, art.ID)
	if fresh.RecommendScore != 0 {
		t.Errorf("score = %d, want 0 (arrow only)", fresh.RecommendScore)
	}
	pushes, _ := st.Pushes().ListByArticle(ctx, art.ID)
	if len(pushes) != 1 {
		t.Errorf("inserted %d rows, want 1 (just the arrow)", len(pushes))
	}
}

// TestPushes_Create_NotFoundWhenArticleMissing: surfaces ErrArticleNotFound
// from the comments_mode SELECT before attempting any insert. Without this
// gate, the FK on pushes.article_id would have eaten the bogus ID silently.
func TestPushes_Create_NotFoundWhenArticleMissing(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	_, err := st.Pushes().Create(ctx, 9999, alice.ID, alice.UserID, store.PushKindPush, "msg")
	if !errors.Is(err, store.ErrArticleNotFound) {
		t.Errorf("got %v, want ErrArticleNotFound", err)
	}
}
