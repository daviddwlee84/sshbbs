package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestArticles_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "Alice")
	board := storetest.MustBoard(t, st, "Test")

	got, err := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "標題", "內容\n第二行")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == 0 {
		t.Error("Create returned zero ID")
	}
	if got.Title != "標題" || got.Body != "內容\n第二行" {
		t.Errorf("title/body mismatch: %q / %q", got.Title, got.Body)
	}
	if got.RecommendScore != 0 {
		t.Errorf("RecommendScore = %d, want 0", got.RecommendScore)
	}

	fetched, err := st.Articles().GetByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fetched.AuthorUserID != "alice" {
		t.Errorf("AuthorUserID = %q, want alice", fetched.AuthorUserID)
	}
}

func TestArticles_GetByID_NotFound(t *testing.T) {
	st := storetest.New(t)
	_, err := st.Articles().GetByID(context.Background(), 9999)
	if !errors.Is(err, store.ErrArticleNotFound) {
		t.Errorf("got %v, want ErrArticleNotFound", err)
	}
}

func TestArticles_ListByBoard_NewestFirst(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	// Insert three with explicit small sleeps so created_at differs.
	for i, title := range []string{"first", "second", "third"} {
		if _, err := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, title, "body"); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		time.Sleep(15 * time.Millisecond)
	}

	got, err := st.Articles().ListByBoard(ctx, board.ID, 0)
	if err != nil {
		t.Fatalf("ListByBoard: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	// Newest-first.
	wantOrder := []string{"third", "second", "first"}
	for i, w := range wantOrder {
		if got[i].Title != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].Title, w)
		}
	}
}

func TestArticles_ListByBoard_Limit(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	for i := 0; i < 5; i++ {
		_, _ = st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "t", "b")
	}
	got, err := st.Articles().ListByBoard(ctx, board.ID, 2)
	if err != nil {
		t.Fatalf("ListByBoard: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
}

func TestArticles_NeighboursOf(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	other := storetest.MustBoard(t, st, "ChitChat")

	a, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "first", "")
	b, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "second", "")
	c, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "third", "")

	// Middle article: both neighbours present.
	prev, next, err := st.Articles().NeighboursOf(ctx, board.ID, b.ID)
	if err != nil {
		t.Fatalf("middle: %v", err)
	}
	if prev != a.ID || next != c.ID {
		t.Errorf("middle: prev=%d next=%d, want prev=%d next=%d", prev, next, a.ID, c.ID)
	}

	// First article: prev=0, next=second.
	prev, next, _ = st.Articles().NeighboursOf(ctx, board.ID, a.ID)
	if prev != 0 || next != b.ID {
		t.Errorf("first: prev=%d next=%d, want prev=0 next=%d", prev, next, b.ID)
	}

	// Last article: prev=second, next=0.
	prev, next, _ = st.Articles().NeighboursOf(ctx, board.ID, c.ID)
	if prev != b.ID || next != 0 {
		t.Errorf("last: prev=%d next=%d, want prev=%d next=0", prev, next, b.ID)
	}

	// Wrong board: 0/0 even though the article exists elsewhere.
	prev, next, _ = st.Articles().NeighboursOf(ctx, other.ID, b.ID)
	if prev != 0 || next != 0 {
		t.Errorf("wrong board: prev=%d next=%d, want 0/0", prev, next)
	}

	// Single-article board: both 0. Use the empty `other` board for this.
	d, _ := st.Articles().Create(ctx, other.ID, user.ID, user.UserID, "alone", "")
	prev, next, _ = st.Articles().NeighboursOf(ctx, other.ID, d.ID)
	if prev != 0 || next != 0 {
		t.Errorf("single: prev=%d next=%d, want 0/0", prev, next)
	}
}

func TestArticles_Delete_OwnerSucceeds(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "x", "y")

	if err := st.Articles().Delete(ctx, a.ID, alice.ID, store.RoleUser); err != nil {
		t.Fatalf("Delete(owner): %v", err)
	}
	if _, err := st.Articles().GetByID(ctx, a.ID); !errors.Is(err, store.ErrArticleNotFound) {
		t.Errorf("after delete: got %v, want ErrArticleNotFound", err)
	}
}

func TestArticles_Delete_NonOwnerNonModDenied(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "x", "y")

	if err := st.Articles().Delete(ctx, a.ID, bob.ID, store.RoleUser); !errors.Is(err, store.ErrPermissionDenied) {
		t.Errorf("got %v, want ErrPermissionDenied", err)
	}
	// Article should still exist.
	if _, err := st.Articles().GetByID(ctx, a.ID); err != nil {
		t.Errorf("article was deleted despite permission error: %v", err)
	}
}

func TestArticles_Delete_ModAndAdminCanDeleteAnyones(t *testing.T) {
	ctx := context.Background()
	board := storetest.MustBoard(t, storetest.New(t), "Test") // separate sanity store
	_ = board                                                 // unused; per-case stores below

	cases := []store.Role{store.RoleMod, store.RoleAdmin}
	for _, role := range cases {
		t.Run(string(role), func(t *testing.T) {
			st := storetest.New(t)
			alice := storetest.MustUser(t, st, "alice", "")
			bob := storetest.MustUser(t, st, "bob", "")
			board := storetest.MustBoard(t, st, "Test")
			a, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "x", "y")

			if err := st.Articles().Delete(ctx, a.ID, bob.ID, role); err != nil {
				t.Fatalf("Delete(%s): %v", role, err)
			}
			if _, err := st.Articles().GetByID(ctx, a.ID); !errors.Is(err, store.ErrArticleNotFound) {
				t.Errorf("after %s delete: got %v, want ErrArticleNotFound", role, err)
			}
		})
	}
}

func TestArticles_Delete_NotFound(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	if err := st.Articles().Delete(ctx, 9999, alice.ID, store.RoleUser); !errors.Is(err, store.ErrArticleNotFound) {
		t.Errorf("got %v, want ErrArticleNotFound", err)
	}
}

func TestArticles_Delete_CascadesPushes(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "x", "y")
	if _, err := st.Pushes().Create(ctx, a.ID, bob.ID, bob.UserID, store.PushKindPush, "+1"); err != nil {
		t.Fatalf("seed push: %v", err)
	}
	pre, _ := st.Pushes().ListByArticle(ctx, a.ID)
	if len(pre) != 1 {
		t.Fatalf("seed push count = %d, want 1", len(pre))
	}

	if err := st.Articles().Delete(ctx, a.ID, alice.ID, store.RoleUser); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	post, err := st.Pushes().ListByArticle(ctx, a.ID)
	if err != nil {
		t.Fatalf("ListByArticle after delete: %v", err)
	}
	if len(post) != 0 {
		t.Errorf("pushes not cascaded: %d remain", len(post))
	}
}

func TestArticles_Update_OwnerSucceeds(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "old title", "old body")

	if err := st.Articles().Update(ctx, a.ID, alice.ID, store.RoleUser, "new title", "new body"); err != nil {
		t.Fatalf("Update(owner): %v", err)
	}
	got, err := st.Articles().GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != "new title" || got.Body != "new body" {
		t.Errorf("title/body = %q/%q, want new", got.Title, got.Body)
	}
	if !got.UpdatedAt.Valid {
		t.Error("UpdatedAt not set after Update")
	}
}

func TestArticles_Update_ModCanEditAnyones(t *testing.T) {
	ctx := context.Background()
	for _, role := range []store.Role{store.RoleMod, store.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			st := storetest.New(t)
			alice := storetest.MustUser(t, st, "alice", "")
			bob := storetest.MustUser(t, st, "bob", "")
			board := storetest.MustBoard(t, st, "Test")
			a, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "x", "y")

			if err := st.Articles().Update(ctx, a.ID, bob.ID, role, "edited by mod", "edited body"); err != nil {
				t.Fatalf("Update(%s): %v", role, err)
			}
			got, _ := st.Articles().GetByID(ctx, a.ID)
			if got.Title != "edited by mod" {
				t.Errorf("%s edit: title = %q, want 'edited by mod'", role, got.Title)
			}
		})
	}
}

func TestArticles_Update_NonOwnerNonModDenied(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, alice.ID, alice.UserID, "untouched", "body")

	if err := st.Articles().Update(ctx, a.ID, bob.ID, store.RoleUser, "evil", "evil"); !errors.Is(err, store.ErrPermissionDenied) {
		t.Errorf("got %v, want ErrPermissionDenied", err)
	}
	got, _ := st.Articles().GetByID(ctx, a.ID)
	if got.Title != "untouched" {
		t.Errorf("title was changed despite permission error: %q", got.Title)
	}
	if got.UpdatedAt.Valid {
		t.Error("UpdatedAt set despite denied update")
	}
}

func TestArticles_Update_NotFound(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	if err := st.Articles().Update(ctx, 9999, alice.ID, store.RoleUser, "t", "b"); !errors.Is(err, store.ErrArticleNotFound) {
		t.Errorf("got %v, want ErrArticleNotFound", err)
	}
}

func TestArticles_SetPinned_BasicToggle(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, "rules", "body")

	if a.PinnedAt.Valid {
		t.Fatalf("freshly created article should not be pinned, got %+v", a.PinnedAt)
	}

	if err := st.Articles().SetPinned(ctx, a.ID, mod.ID, store.RoleMod, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	got, _ := st.Articles().GetByID(ctx, a.ID)
	if !got.PinnedAt.Valid {
		t.Errorf("after pin: PinnedAt.Valid = false, want true")
	}

	if err := st.Articles().SetPinned(ctx, a.ID, mod.ID, store.RoleMod, false); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	got, _ = st.Articles().GetByID(ctx, a.ID)
	if got.PinnedAt.Valid {
		t.Errorf("after unpin: PinnedAt.Valid = true, want false (got %v)", got.PinnedAt.Time)
	}
}

func TestArticles_SetPinned_RequiresMod(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "rules", "body")

	// Author themselves cannot pin their own post — pinning is a moderation
	// action and intentionally diverges from Update/Delete which admit author.
	for _, role := range []store.Role{store.RoleGuest, store.RoleUser} {
		t.Run("denied_"+string(role), func(t *testing.T) {
			err := st.Articles().SetPinned(ctx, a.ID, user.ID, role, true)
			if !errors.Is(err, store.ErrPermissionDenied) {
				t.Errorf("got %v, want ErrPermissionDenied", err)
			}
			got, _ := st.Articles().GetByID(ctx, a.ID)
			if got.PinnedAt.Valid {
				t.Error("article was pinned despite permission denial")
			}
		})
	}

	for _, role := range []store.Role{store.RoleMod, store.RoleAdmin} {
		t.Run("allowed_"+string(role), func(t *testing.T) {
			if err := st.Articles().SetPinned(ctx, a.ID, user.ID, role, true); err != nil {
				t.Fatalf("SetPinned(%s): %v", role, err)
			}
			// reset for next iteration
			_ = st.Articles().SetPinned(ctx, a.ID, user.ID, role, false)
		})
	}
}

func TestArticles_SetPinned_NotFound(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	if err := st.Articles().SetPinned(ctx, 9999, mod.ID, store.RoleMod, true); !errors.Is(err, store.ErrArticleNotFound) {
		t.Errorf("got %v, want ErrArticleNotFound", err)
	}
}

func TestArticles_SetCommentsMode_RoundTrip(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, "rules", "body")

	if a.CommentsMode != store.CommentsModeOpen {
		t.Fatalf("freshly created article: CommentsMode = %q, want %q", a.CommentsMode, store.CommentsModeOpen)
	}

	for _, mode := range []store.CommentsMode{
		store.CommentsModeArrowsOnly,
		store.CommentsModeLocked,
		store.CommentsModeOpen,
	} {
		if err := st.Articles().SetCommentsMode(ctx, a.ID, mod.ID, store.RoleMod, mode); err != nil {
			t.Fatalf("SetCommentsMode(%s): %v", mode, err)
		}
		got, err := st.Articles().GetByID(ctx, a.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.CommentsMode != mode {
			t.Errorf("after SetCommentsMode(%s): CommentsMode = %q, want %q", mode, got.CommentsMode, mode)
		}
	}
}

func TestArticles_SetCommentsMode_RequiresMod(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "post", "body")

	// Author themselves cannot lock their own post — comments mode is a
	// moderation action (parity with SetPinned, NOT Update/Delete).
	for _, role := range []store.Role{store.RoleGuest, store.RoleUser} {
		t.Run("denied_"+string(role), func(t *testing.T) {
			err := st.Articles().SetCommentsMode(ctx, a.ID, user.ID, role, store.CommentsModeLocked)
			if !errors.Is(err, store.ErrPermissionDenied) {
				t.Errorf("got %v, want ErrPermissionDenied", err)
			}
			got, _ := st.Articles().GetByID(ctx, a.ID)
			if got.CommentsMode != store.CommentsModeOpen {
				t.Errorf("mode changed despite permission denial: got %q", got.CommentsMode)
			}
		})
	}

	for _, role := range []store.Role{store.RoleMod, store.RoleAdmin} {
		t.Run("allowed_"+string(role), func(t *testing.T) {
			if err := st.Articles().SetCommentsMode(ctx, a.ID, user.ID, role, store.CommentsModeArrowsOnly); err != nil {
				t.Fatalf("SetCommentsMode(%s): %v", role, err)
			}
			// reset for next iteration
			_ = st.Articles().SetCommentsMode(ctx, a.ID, user.ID, role, store.CommentsModeOpen)
		})
	}
}

func TestArticles_SetCommentsMode_RejectsInvalidMode(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, "post", "body")

	for _, bad := range []store.CommentsMode{"", "invalid", "OPEN", "LOCKED"} {
		err := st.Articles().SetCommentsMode(ctx, a.ID, mod.ID, store.RoleMod, bad)
		if !errors.Is(err, store.ErrInvalidCommentsMode) {
			t.Errorf("SetCommentsMode(%q): got %v, want ErrInvalidCommentsMode", bad, err)
		}
	}
	got, _ := st.Articles().GetByID(ctx, a.ID)
	if got.CommentsMode != store.CommentsModeOpen {
		t.Errorf("mode mutated by invalid input: got %q", got.CommentsMode)
	}
}

func TestArticles_SetCommentsMode_NotFound(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	if err := st.Articles().SetCommentsMode(ctx, 9999, mod.ID, store.RoleMod, store.CommentsModeLocked); !errors.Is(err, store.ErrArticleNotFound) {
		t.Errorf("got %v, want ErrArticleNotFound", err)
	}
}

func TestArticles_ListByBoard_PinnedFirst(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	board := storetest.MustBoard(t, st, "Test")

	// Create four articles in chronological order. Pin the second one
	// (oldest pin candidate that isn't the very first row, so we can prove
	// it bubbles to the top regardless of created_at).
	titles := []string{"first", "second", "third", "fourth"}
	ids := make(map[string]int64, len(titles))
	for _, ti := range titles {
		a, err := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, ti, "")
		if err != nil {
			t.Fatalf("create %q: %v", ti, err)
		}
		ids[ti] = a.ID
		time.Sleep(15 * time.Millisecond)
	}
	if err := st.Articles().SetPinned(ctx, ids["second"], mod.ID, store.RoleMod, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	got, err := st.Articles().ListByBoard(ctx, board.ID, 0)
	if err != nil {
		t.Fatalf("ListByBoard: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d articles, want 4", len(got))
	}
	// Expected order: pinned first ("second"), then unpinned by created_at DESC.
	want := []string{"second", "fourth", "third", "first"}
	for i, w := range want {
		if got[i].Title != w {
			t.Errorf("[%d] = %q, want %q (full order: %v)", i, got[i].Title, w, titlesOf(got))
		}
	}

	// Pin a second article — both should now be at the top, ordered by
	// created_at DESC within the pinned group.
	if err := st.Articles().SetPinned(ctx, ids["fourth"], mod.ID, store.RoleMod, true); err != nil {
		t.Fatalf("pin fourth: %v", err)
	}
	got, _ = st.Articles().ListByBoard(ctx, board.ID, 0)
	wantMulti := []string{"fourth", "second", "third", "first"}
	for i, w := range wantMulti {
		if got[i].Title != w {
			t.Errorf("multi-pin [%d] = %q, want %q (full order: %v)", i, got[i].Title, w, titlesOf(got))
		}
	}
}

func TestArticles_Update_PreservesPinnedAt(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, "rules v1", "body")
	if err := st.Articles().SetPinned(ctx, a.ID, mod.ID, store.RoleMod, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	pinnedBefore, _ := st.Articles().GetByID(ctx, a.ID)
	if !pinnedBefore.PinnedAt.Valid {
		t.Fatalf("pin precondition failed")
	}
	if err := st.Articles().Update(ctx, a.ID, mod.ID, store.RoleMod, "rules v2", "amended body"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := st.Articles().GetByID(ctx, a.ID)
	if !got.PinnedAt.Valid {
		t.Errorf("PinnedAt cleared by Update — must be preserved")
	}
	if !got.PinnedAt.Time.Equal(pinnedBefore.PinnedAt.Time) {
		t.Errorf("PinnedAt mutated by Update: before=%v after=%v", pinnedBefore.PinnedAt.Time, got.PinnedAt.Time)
	}
	if got.Title != "rules v2" {
		t.Errorf("Update did not apply: title=%q", got.Title)
	}
}

func titlesOf(arts []*store.Article) []string {
	out := make([]string, len(arts))
	for i, a := range arts {
		out[i] = a.Title
	}
	return out
}

func TestArticles_BoardIsolation(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	test := storetest.MustBoard(t, st, "Test")
	chitChat := storetest.MustBoard(t, st, "ChitChat")

	_, _ = st.Articles().Create(ctx, test.ID, user.ID, user.UserID, "in test", "")
	_, _ = st.Articles().Create(ctx, chitChat.ID, user.ID, user.UserID, "in chitchat", "")

	gotTest, _ := st.Articles().ListByBoard(ctx, test.ID, 0)
	if len(gotTest) != 1 || gotTest[0].Title != "in test" {
		t.Errorf("Test board: got %+v, want one article 'in test'", gotTest)
	}
	gotCC, _ := st.Articles().ListByBoard(ctx, chitChat.ID, 0)
	if len(gotCC) != 1 || gotCC[0].Title != "in chitchat" {
		t.Errorf("ChitChat board: got %+v, want one article 'in chitchat'", gotCC)
	}
}

func TestArticles_ListByBoardOpts_TitleSearch(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	titles := []string{"First post", "Second post", "Third entry", "First update"}
	for _, ti := range titles {
		if _, err := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, ti, ""); err != nil {
			t.Fatalf("create %q: %v", ti, err)
		}
		time.Sleep(15 * time.Millisecond)
	}

	got, err := st.Articles().ListByBoardOpts(ctx, board.ID, store.ListArticlesOpts{TitleSearch: "first"})
	if err != nil {
		t.Fatalf("ListByBoardOpts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d hits, want 2 (got: %v)", len(got), titlesOf(got))
	}
	// Newest-first: "First update" was created last among the two matches.
	want := []string{"First update", "First post"}
	for i, w := range want {
		if got[i].Title != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].Title, w)
		}
	}

	// Whitespace-only query is ignored (returns full list).
	got, _ = st.Articles().ListByBoardOpts(ctx, board.ID, store.ListArticlesOpts{TitleSearch: "   "})
	if len(got) != 4 {
		t.Errorf("whitespace query: got %d, want 4", len(got))
	}
}

func TestArticles_ListByBoardOpts_TitleSearch_EscapesLikeWildcards(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	for _, ti := range []string{"50% off", "5 things", "no match here"} {
		if _, err := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, ti, ""); err != nil {
			t.Fatalf("create %q: %v", ti, err)
		}
	}

	// "50%" must match only the literal "50% off" — without escaping the
	// percent would behave as a wildcard and also match "5 things".
	got, _ := st.Articles().ListByBoardOpts(ctx, board.ID, store.ListArticlesOpts{TitleSearch: "50%"})
	if len(got) != 1 || got[0].Title != "50% off" {
		t.Errorf("escaped %% search: got %v, want exactly [50%% off]", titlesOf(got))
	}

	// Bare "5" is a substring of both 50% off and 5 things.
	got, _ = st.Articles().ListByBoardOpts(ctx, board.ID, store.ListArticlesOpts{TitleSearch: "5"})
	if len(got) != 2 {
		t.Errorf("bare 5 search: got %d, want 2 (titles: %v)", len(got), titlesOf(got))
	}

	// Underscore must also be escaped — title "no match here" doesn't contain
	// the literal "_", so a "_" query should match nothing rather than acting
	// as a single-character wildcard.
	got, _ = st.Articles().ListByBoardOpts(ctx, board.ID, store.ListArticlesOpts{TitleSearch: "_"})
	if len(got) != 0 {
		t.Errorf("escaped _ search: got %d, want 0 (titles: %v)", len(got), titlesOf(got))
	}
}

func TestArticles_ListByBoardOpts_TitleSearch_BoardIsolation(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	test := storetest.MustBoard(t, st, "Test")
	chitChat := storetest.MustBoard(t, st, "ChitChat")

	_, _ = st.Articles().Create(ctx, test.ID, user.ID, user.UserID, "shared keyword", "")
	_, _ = st.Articles().Create(ctx, chitChat.ID, user.ID, user.UserID, "shared keyword", "")

	got, _ := st.Articles().ListByBoardOpts(ctx, test.ID,
		store.ListArticlesOpts{TitleSearch: "shared"})
	if len(got) != 1 {
		t.Errorf("got %d, want 1 (filter must scope to boardID)", len(got))
	}
	if len(got) > 0 && got[0].BoardID != test.ID {
		t.Errorf("BoardID = %d, want %d", got[0].BoardID, test.ID)
	}
}

func TestArticles_ListByBoardOpts_TitleSearch_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	_, _ = st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "MixedCase Title", "")

	got, _ := st.Articles().ListByBoardOpts(ctx, board.ID,
		store.ListArticlesOpts{TitleSearch: "mixedcase"})
	if len(got) != 1 {
		t.Errorf("lowercase query: got %d, want 1", len(got))
	}
	got, _ = st.Articles().ListByBoardOpts(ctx, board.ID,
		store.ListArticlesOpts{TitleSearch: "MIXEDCASE"})
	if len(got) != 1 {
		t.Errorf("uppercase query: got %d, want 1", len(got))
	}
}

func TestArticles_ListByBoardOpts_SortByScore(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	hot, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "hot", "")
	cold, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "cold", "")
	mid, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "mid", "")

	// Build distinct scores via real Pushes() so the cached recommend_score
	// is exercised end-to-end (mirrors how the production code drives it).
	mustPush(t, st, hot.ID, store.PushKindPush, 5)
	mustPush(t, st, mid.ID, store.PushKindPush, 3)
	mustPush(t, st, cold.ID, store.PushKindBoo, 1)

	got, err := st.Articles().ListByBoardOpts(ctx, board.ID,
		store.ListArticlesOpts{Sort: store.SortByScoreDesc})
	if err != nil {
		t.Fatalf("ListByBoardOpts: %v", err)
	}
	wantOrder := []string{"hot", "mid", "cold"}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	for i, w := range wantOrder {
		if got[i].Title != w {
			t.Errorf("[%d] = %q (score %d), want %q", i, got[i].Title, got[i].RecommendScore, w)
		}
	}
}

func TestArticles_ListByBoardOpts_SortByScore_PinnedStillFirst(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	board := storetest.MustBoard(t, st, "Test")

	pinned, _ := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, "pinned-rules", "")
	unpinned, _ := st.Articles().Create(ctx, board.ID, mod.ID, mod.UserID, "unpinned-hot", "")

	// Pinned article gets score 0; unpinned gets a much higher score. Pin
	// should still float to top — pin is an explicit moderation override.
	mustPush(t, st, unpinned.ID, store.PushKindPush, 10)
	if err := st.Articles().SetPinned(ctx, pinned.ID, mod.ID, store.RoleMod, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	got, _ := st.Articles().ListByBoardOpts(ctx, board.ID,
		store.ListArticlesOpts{Sort: store.SortByScoreDesc})
	if len(got) != 2 || got[0].Title != "pinned-rules" {
		t.Errorf("pinned must lead in score sort: got %v", titlesOf(got))
	}
}

func TestArticles_ListByBoardOpts_TitleSearchAndSort_Combined(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	hotMatch, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "hot match", "")
	coldMatch, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "cold match", "")
	_, _ = st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "no keyword", "")

	mustPush(t, st, hotMatch.ID, store.PushKindPush, 7)
	mustPush(t, st, coldMatch.ID, store.PushKindBoo, 2)

	got, _ := st.Articles().ListByBoardOpts(ctx, board.ID, store.ListArticlesOpts{
		TitleSearch: "match",
		Sort:        store.SortByScoreDesc,
	})
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 (titles: %v)", len(got), titlesOf(got))
	}
	if got[0].Title != "hot match" || got[1].Title != "cold match" {
		t.Errorf("combined order: %v, want [hot match, cold match]", titlesOf(got))
	}
}

// mustPush stacks n pushes of `kind` against articleID, each from a freshly
// registered helper user so the cached recommend_score reflects |delta| × n.
// Multiple invocations within one test must not collide on user_id — articleID
// is part of the helper's nickname seed so each (article, kind, index) tuple
// is globally unique.
func mustPush(t *testing.T, st *store.Store, articleID int64, kind store.PushKind, n int) {
	t.Helper()
	ctx := context.Background()
	for i := range n {
		nick := fmt.Sprintf("p%d_%s_%d", articleID, kind, i)
		pusher := storetest.MustUser(t, st, nick, "")
		if _, err := st.Pushes().Create(ctx, articleID, pusher.ID, pusher.UserID, kind, "."); err != nil {
			t.Fatalf("push %d on article %d: %v", i, articleID, err)
		}
	}
}
