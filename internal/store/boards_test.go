package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestBoards_List(t *testing.T) {
	st := storetest.New(t)
	got, err := st.Boards().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 default boards, got %d", len(got))
	}
	// Order is by name COLLATE NOCASE: ChitChat, Test, Welcome.
	wantOrder := []string{"ChitChat", "Test", "Welcome"}
	for i, w := range wantOrder {
		if got[i].Name != w {
			t.Errorf("List[%d].Name = %q, want %q", i, got[i].Name, w)
		}
	}
}

func TestBoards_GetByName_CaseInsensitive(t *testing.T) {
	st := storetest.New(t)
	for _, name := range []string{"test", "Test", "TEST"} {
		got, err := st.Boards().GetByName(context.Background(), name)
		if err != nil {
			t.Errorf("GetByName(%q): %v", name, err)
			continue
		}
		if got.Name != "Test" {
			t.Errorf("GetByName(%q).Name = %q, want %q", name, got.Name, "Test")
		}
	}
}

func TestBoards_GetByName_NotFound(t *testing.T) {
	st := storetest.New(t)
	_, err := st.Boards().GetByName(context.Background(), "DoesNotExist")
	if !errors.Is(err, store.ErrBoardNotFound) {
		t.Errorf("got %v, want ErrBoardNotFound", err)
	}
}

func TestBoards_SeedDefaults_Idempotent(t *testing.T) {
	st := storetest.New(t)
	// Migration 0002 already inserted them. SeedDefaults again must not
	// duplicate or error.
	for i := 0; i < 3; i++ {
		if err := st.Boards().SeedDefaults(context.Background()); err != nil {
			t.Fatalf("SeedDefaults #%d: %v", i, err)
		}
	}
	got, err := st.Boards().List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d boards after repeated seeding, want 3", len(got))
	}
}
