package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestOpenRunsMigrations(t *testing.T) {
	st := storetest.New(t)

	// Default boards must exist after migrations + SeedDefaults isn't called
	// here — migration 0002 alone is enough.
	for _, name := range []string{"Welcome", "Test", "ChitChat"} {
		if _, err := st.Boards().GetByName(context.Background(), name); err != nil {
			t.Errorf("default board %q missing: %v", name, err)
		}
	}
}

func TestOpenIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "idem.db")

	// First open creates schema.
	s1, err := store.Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := s1.Boards().GetByName(context.Background(), "Test"); err != nil {
		t.Fatalf("first lookup: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Re-open must not re-apply migrations and must preserve data.
	s2, err := store.Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()
	boards, err := s2.Boards().List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(boards) != 3 {
		t.Errorf("got %d boards, want 3 (migrations re-applied?)", len(boards))
	}
}
