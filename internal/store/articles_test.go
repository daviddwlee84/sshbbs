package store_test

import (
	"context"
	"errors"
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
