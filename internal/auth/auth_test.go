package auth_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/auth"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestRegister_Success(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	u, err := auth.Register(ctx, st, "alice", "pw123456", "愛麗絲", "alice@example.com")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if u.UserID != "alice" {
		t.Errorf("UserID = %q, want alice", u.UserID)
	}
	if u.Nickname != "愛麗絲" {
		t.Errorf("Nickname = %q, want 愛麗絲", u.Nickname)
	}
	if u.PasswordHash == "pw123456" {
		t.Error("PasswordHash stored in plaintext")
	}
}

func TestRegister_Validations(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	cases := []struct {
		name                string
		userID, pw, nick, email string
		want                error
	}{
		{"short userid", "ab", "pw123456", "", "", auth.ErrInvalidUserID},
		{"bad userid char", "ali!ce", "pw123456", "", "", auth.ErrInvalidUserID},
		{"reserved new", "new", "pw123456", "", "", auth.ErrReservedUsername},
		{"reserved guest", "guest", "pw123456", "", "", auth.ErrReservedUsername},
		{"reserved admin", "admin", "pw123456", "", "", auth.ErrReservedUsername},
		{"reserved NEW (mixed case)", "NEW", "pw123456", "", "", auth.ErrReservedUsername},
		{"reserved Admin (mixed case)", "Admin", "pw123456", "", "", auth.ErrReservedUsername},
		{"short password", "alice", "x", "", "", auth.ErrInvalidPassword},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := auth.Register(ctx, st, tc.userID, tc.pw, tc.nick, tc.email)
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestIsReservedUsername(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"new", true},
		{"NEW", true},
		{"  new  ", true},
		{"guest", true},
		{"Guest", true},
		{"admin", true},
		{"ADMIN", true},
		{"alice", false},
		{"", false},
		{"newish", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := auth.IsReservedUsername(tc.in); got != tc.want {
				t.Errorf("IsReservedUsername(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRegister_DuplicateUserID(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	if _, err := auth.Register(ctx, st, "alice", "pw123456", "", ""); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := auth.Register(ctx, st, "alice", "pw999999", "", "")
	if !errors.Is(err, auth.ErrUserExists) {
		t.Errorf("got %v, want ErrUserExists", err)
	}
	// Same name with different case should also collide (UNIQUE COLLATE NOCASE).
	_, err = auth.Register(ctx, st, "ALICE", "pw999999", "", "")
	if !errors.Is(err, auth.ErrUserExists) {
		t.Errorf("case-insensitive duplicate: got %v, want ErrUserExists", err)
	}
}

func TestVerifyLogin(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	if _, err := auth.Register(ctx, st, "alice", "pw123456", "愛麗絲", ""); err != nil {
		t.Fatalf("Register: %v", err)
	}

	t.Run("correct password", func(t *testing.T) {
		u, err := auth.VerifyLogin(ctx, st, "alice", "pw123456", "127.0.0.1:1234")
		if err != nil {
			t.Fatalf("VerifyLogin: %v", err)
		}
		if u.UserID != "alice" {
			t.Errorf("UserID = %q, want alice", u.UserID)
		}
		if !u.LastLoginAt.Valid {
			t.Error("LastLoginAt not set after successful login")
		}
		if u.NumLogins != 1 {
			t.Errorf("NumLogins = %d, want 1", u.NumLogins)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		_, err := auth.VerifyLogin(ctx, st, "alice", "wrong", "127.0.0.1:1234")
		if !errors.Is(err, auth.ErrBadCredentials) {
			t.Errorf("got %v, want ErrBadCredentials", err)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		_, err := auth.VerifyLogin(ctx, st, "ghost", "pw123456", "127.0.0.1:1234")
		if !errors.Is(err, auth.ErrBadCredentials) {
			t.Errorf("got %v, want ErrBadCredentials", err)
		}
	})

	t.Run("case-insensitive userid lookup", func(t *testing.T) {
		u, err := auth.VerifyLogin(ctx, st, "ALICE", "pw123456", "127.0.0.1:1234")
		if err != nil {
			t.Fatalf("VerifyLogin: %v", err)
		}
		if u.UserID != "alice" {
			t.Errorf("UserID = %q, want alice", u.UserID)
		}
	})
}
