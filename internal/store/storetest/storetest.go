// Package storetest provides shared helpers for tests that need a fresh
// SQLite store. It lives in its own package so any internal package can
// import it without creating cycles.
package storetest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

// New opens a fresh on-disk SQLite store in t.TempDir(), runs migrations,
// and registers a Cleanup to close it. Use it whenever you'd write
// `store.Open` in a test — never share stores across tests.
func New(tb testing.TB) *store.Store {
	tb.Helper()
	dir := tb.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		tb.Fatalf("store.Open: %v", err)
	}
	tb.Cleanup(func() { _ = st.Close() })
	return st
}

// MustUser inserts a user with a placeholder password hash and returns it.
// Convenient for tests that need a logged-in identity but don't care about auth.
func MustUser(tb testing.TB, st *store.Store, userID, nickname string) *store.User {
	tb.Helper()
	u, err := st.Users().Create(context.Background(), userID, "$2a$12$placeholderplaceholderplaceholderplaceholderplaceholder.", nickname, "")
	if err != nil {
		tb.Fatalf("create user %q: %v", userID, err)
	}
	return u
}

// MustBoard returns a seeded board by name, failing the test if not found.
// Useful with the default Welcome / Test / ChitChat boards which
// SeedDefaults / migration 0002 create automatically.
func MustBoard(tb testing.TB, st *store.Store, name string) *store.Board {
	tb.Helper()
	b, err := st.Boards().GetByName(context.Background(), name)
	if err != nil {
		tb.Fatalf("get board %q: %v", name, err)
	}
	return b
}
