package seed_test

import (
	"context"
	"strings"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/seed"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// mustAdmin inserts an admin-role system user and returns its UserID
// (string) so seed.Articles can look it up the same way main.go does.
func mustAdmin(t *testing.T, st *store.Store) string {
	t.Helper()
	if _, err := st.Users().InsertSystemAccount(
		context.Background(), "admin", "$2a$12$placeholderplaceholderplaceholderplaceholderplaceholder.",
		store.RoleAdmin, false,
	); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return "admin"
}

func TestArticles_SeedsWhenBoardEmpty(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	admin := mustAdmin(t, st)

	if err := seed.Articles(ctx, st, admin); err != nil {
		t.Fatalf("seed.Articles: %v", err)
	}

	welcome, err := st.Boards().GetByName(ctx, "Welcome")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	got, _ := st.Articles().ListByBoard(ctx, welcome.ID, 10)
	if len(got) != 1 {
		t.Fatalf("Welcome article count = %d, want 1", len(got))
	}
	if got[0].AuthorUserID != "admin" {
		t.Errorf("author = %q, want admin", got[0].AuthorUserID)
	}
	if !strings.Contains(got[0].Title, "歡迎") {
		t.Errorf("title = %q, want it to contain 歡迎", got[0].Title)
	}
	if got[0].Body == "" {
		t.Error("body is empty")
	}
}

func TestArticles_NoOpWhenBoardHasContent(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	admin := mustAdmin(t, st)

	welcome, _ := st.Boards().GetByName(ctx, "Welcome")
	// Pre-seed a fake article so seed.Articles should skip Welcome.
	if _, err := st.Articles().Create(ctx, welcome.ID, 1, "admin", "user-edited", "user-body"); err != nil {
		t.Fatalf("seed pre-existing: %v", err)
	}

	if err := seed.Articles(ctx, st, admin); err != nil {
		t.Fatalf("seed.Articles: %v", err)
	}

	got, _ := st.Articles().ListByBoard(ctx, welcome.ID, 10)
	if len(got) != 1 {
		t.Fatalf("Welcome article count = %d, want 1 (the pre-existing)", len(got))
	}
	if got[0].Title != "user-edited" {
		t.Errorf("title = %q, want 'user-edited' (pre-existing)", got[0].Title)
	}
}

func TestArticles_IdempotentOverRestarts(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	admin := mustAdmin(t, st)

	for i := 0; i < 3; i++ {
		if err := seed.Articles(ctx, st, admin); err != nil {
			t.Fatalf("seed run #%d: %v", i, err)
		}
	}

	welcome, _ := st.Boards().GetByName(ctx, "Welcome")
	got, _ := st.Articles().ListByBoard(ctx, welcome.ID, 10)
	if len(got) != 1 {
		t.Errorf("after 3 seed runs: %d articles, want 1", len(got))
	}
}

func TestArticles_AdminMissing(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	if err := seed.Articles(ctx, st, "admin"); err == nil {
		t.Error("expected error when admin user does not exist")
	}
}
