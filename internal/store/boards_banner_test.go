package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestBoards_UpdateBanner_AdminSucceeds(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	want := "\x1b[31mhello\x1b[0m"
	if err := st.Boards().UpdateBanner(ctx, board.ID, user.ID, store.RoleAdmin, want); err != nil {
		t.Fatalf("UpdateBanner: %v", err)
	}
	got, err := st.Boards().GetByID(ctx, board.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Banner != want {
		t.Errorf("Banner = %q, want %q", got.Banner, want)
	}
}

func TestBoards_UpdateBanner_ModSucceeds(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "modder", "")
	board := storetest.MustBoard(t, st, "Test")

	if err := st.Boards().UpdateBanner(ctx, board.ID, user.ID, store.RoleMod, "moddy"); err != nil {
		t.Fatalf("UpdateBanner(mod): %v", err)
	}
}

func TestBoards_UpdateBanner_UserDenied(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	for _, role := range []store.Role{store.RoleGuest, store.RoleUser} {
		t.Run(string(role), func(t *testing.T) {
			err := st.Boards().UpdateBanner(ctx, board.ID, user.ID, role, "evil")
			if !errors.Is(err, store.ErrPermissionDenied) {
				t.Errorf("got %v, want ErrPermissionDenied", err)
			}
		})
	}
	// Banner should still be empty since every attempt was denied.
	got, _ := st.Boards().GetByID(ctx, board.ID)
	if got.Banner != "" {
		t.Errorf("Banner = %q, want empty after all denied", got.Banner)
	}
}

func TestBoards_UpdateBanner_NotFound(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")

	err := st.Boards().UpdateBanner(ctx, 9999, user.ID, store.RoleAdmin, "x")
	if !errors.Is(err, store.ErrBoardNotFound) {
		t.Errorf("got %v, want ErrBoardNotFound", err)
	}
}

func TestBoards_UpdateBanner_EmptyClearsBanner(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")

	_ = st.Boards().UpdateBanner(ctx, board.ID, user.ID, store.RoleAdmin, "first")
	if err := st.Boards().UpdateBanner(ctx, board.ID, user.ID, store.RoleAdmin, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ := st.Boards().GetByID(ctx, board.ID)
	if got.Banner != "" {
		t.Errorf("Banner = %q, want empty after clear", got.Banner)
	}
}
