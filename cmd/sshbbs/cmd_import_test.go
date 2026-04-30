package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

func openTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Boards().SeedDefaults(context.Background()); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	if _, err := st.Users().InsertSystemAccount(
		context.Background(), "admin", "$2a$12$placeholderplaceholderplaceholderplaceholderplaceholder.",
		store.RoleAdmin, false,
	); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return st, dbPath
}

func TestRunImport_Success(t *testing.T) {
	st, dbPath := openTestStore(t)
	st.Close()

	mdPath := filepath.Join(t.TempDir(), "in.md")
	if err := os.WriteFile(mdPath, []byte("---\ntitle: Imported\nboard: Welcome\n---\n\nimported body\n"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	rc := runImport([]string{"--file", mdPath, "--db", dbPath})
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}

	// Re-open and verify the article landed.
	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer st2.Close()
	welcome, _ := st2.Boards().GetByName(context.Background(), "Welcome")
	got, _ := st2.Articles().ListByBoard(context.Background(), welcome.ID, 10)
	if len(got) != 1 || got[0].Title != "Imported" {
		t.Errorf("articles after import: %+v", got)
	}
}

func TestRunImport_MissingFile(t *testing.T) {
	rc := runImport([]string{})
	if rc == 0 {
		t.Error("expected non-zero rc for missing --file")
	}
}

func TestRunImport_BoardOverride(t *testing.T) {
	_, dbPath := openTestStore(t)
	mdPath := filepath.Join(t.TempDir(), "in.md")
	// Frontmatter says Welcome but --board overrides to Test.
	if err := os.WriteFile(mdPath, []byte("---\ntitle: Override\nboard: Welcome\n---\n\nbody\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rc := runImport([]string{"--file", mdPath, "--board", "Test", "--db", dbPath})
	if rc != 0 {
		t.Fatalf("rc = %d, want 0", rc)
	}
	st2, _ := store.Open(dbPath)
	defer st2.Close()
	test, _ := st2.Boards().GetByName(context.Background(), "Test")
	got, _ := st2.Articles().ListByBoard(context.Background(), test.ID, 10)
	if len(got) != 1 {
		t.Errorf("Test board article count = %d, want 1", len(got))
	}
}
