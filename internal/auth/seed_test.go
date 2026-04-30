package auth_test

import (
	"context"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/daviddwlee84/sshbbs/internal/auth"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

func openSeedStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSeedSystemAccounts_CreatesAdminAndGuest(t *testing.T) {
	st := openSeedStore(t)
	ctx := context.Background()

	if err := auth.SeedSystemAccounts(ctx, st, auth.SeedOpts{}); err != nil {
		t.Fatalf("SeedSystemAccounts: %v", err)
	}

	admin, err := st.Users().GetByUserID(ctx, "admin")
	if err != nil {
		t.Fatalf("admin not seeded: %v", err)
	}
	if admin.Role != store.RoleAdmin {
		t.Errorf("admin.Role = %q, want admin", admin.Role)
	}
	if !admin.MustChangePassword {
		t.Error("admin.MustChangePassword = false, want true on bootstrap")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(auth.DefaultAdminPassword)); err != nil {
		t.Errorf("default admin password should hash to admin.PasswordHash: %v", err)
	}

	guest, err := st.Users().GetByUserID(ctx, "guest")
	if err != nil {
		t.Fatalf("guest not seeded: %v", err)
	}
	if guest.Role != store.RoleGuest {
		t.Errorf("guest.Role = %q, want guest", guest.Role)
	}
	if guest.MustChangePassword {
		t.Error("guest.MustChangePassword = true, want false")
	}
	if guest.PasswordHash != auth.GuestPasswordSentinel {
		t.Errorf("guest.PasswordHash = %q, want sentinel %q", guest.PasswordHash, auth.GuestPasswordSentinel)
	}
}

func TestSeedSystemAccounts_Idempotent(t *testing.T) {
	st := openSeedStore(t)
	ctx := context.Background()

	if err := auth.SeedSystemAccounts(ctx, st, auth.SeedOpts{}); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	// Snapshot the admin hash so we can verify it isn't rotated on the second call.
	first, err := st.Users().GetByUserID(ctx, "admin")
	if err != nil {
		t.Fatalf("admin lookup: %v", err)
	}
	if err := auth.SeedSystemAccounts(ctx, st, auth.SeedOpts{AdminPassword: "should-be-ignored"}); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	second, err := st.Users().GetByUserID(ctx, "admin")
	if err != nil {
		t.Fatalf("admin re-lookup: %v", err)
	}
	if first.PasswordHash != second.PasswordHash {
		t.Error("admin password hash rotated on idempotent seed (must remain stable)")
	}
	if first.ID != second.ID {
		t.Errorf("admin row id changed (first=%d second=%d)", first.ID, second.ID)
	}
}

func TestSeedSystemAccounts_AdminPasswordOverride(t *testing.T) {
	st := openSeedStore(t)
	ctx := context.Background()

	if err := auth.SeedSystemAccounts(ctx, st, auth.SeedOpts{AdminPassword: "my-secret-7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	admin, err := st.Users().GetByUserID(ctx, "admin")
	if err != nil {
		t.Fatalf("admin lookup: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("my-secret-7")); err != nil {
		t.Errorf("override password should match the stored hash: %v", err)
	}
	// Default password should not match.
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(auth.DefaultAdminPassword)); err == nil {
		t.Error("override seed left default 'admin' password active")
	}
}

func TestSeedSystemAccounts_GuestSentinelUnmatchable(t *testing.T) {
	st := openSeedStore(t)
	ctx := context.Background()

	if err := auth.SeedSystemAccounts(ctx, st, auth.SeedOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// VerifyLogin against guest with anything should fail (hash sentinel
	// is not parseable bcrypt; CompareHashAndPassword returns an error).
	for _, candidate := range []string{"", "guest", "admin", "anything"} {
		if _, err := auth.VerifyLogin(ctx, st, "guest", candidate, "127.0.0.1:1"); err == nil {
			t.Errorf("VerifyLogin(guest, %q) succeeded — sentinel must be unmatchable", candidate)
		}
	}
}
