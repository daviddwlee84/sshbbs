package auth

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

const (
	// GuestPasswordSentinel is stored as the bcrypt hash for the guest
	// user. It is deliberately not a valid bcrypt prefix, so even if the
	// SSH password callback ever falls through to bcrypt.CompareHashAndPassword
	// for guest, the comparison fails. The SSH callback short-circuits guest
	// before reaching VerifyLogin, so this is belt-and-suspenders.
	GuestPasswordSentinel = "!"

	// DefaultAdminPassword is the password baked in when the operator
	// does not pass -admin-password. Combined with must_change_password=1
	// and the loopback-only restriction in MustChangePasswordRemoteBlocked,
	// the operator must rotate it locally before the admin account becomes
	// reachable from the network.
	DefaultAdminPassword = "admin"
)

// SeedOpts controls the bootstrap seed.
type SeedOpts struct {
	// AdminPassword overrides DefaultAdminPassword on first seed.
	// Empty string = use the baked default. Ignored if admin already exists.
	AdminPassword string
}

// SeedSystemAccounts creates the bootstrap admin and guest accounts if
// they do not exist. Idempotent — calling on a populated DB is a no-op.
// Lives in auth (not store) because it owns the bcrypt cost decision.
func SeedSystemAccounts(ctx context.Context, st *store.Store, opts SeedOpts) error {
	if err := seedAdmin(ctx, st, opts); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}
	if err := seedGuest(ctx, st); err != nil {
		return fmt.Errorf("seed guest: %w", err)
	}
	return nil
}

func seedAdmin(ctx context.Context, st *store.Store, opts SeedOpts) error {
	if _, err := st.Users().GetByUserID(ctx, ReservedUsernameAdmin); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrUserNotFound) {
		return err
	}
	pw := opts.AdminPassword
	if pw == "" {
		pw = DefaultAdminPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	if err != nil {
		return err
	}
	_, err = st.Users().InsertSystemAccount(
		ctx, ReservedUsernameAdmin, string(hash), store.RoleAdmin, true,
	)
	return err
}

func seedGuest(ctx context.Context, st *store.Store) error {
	if _, err := st.Users().GetByUserID(ctx, ReservedUsernameGuest); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrUserNotFound) {
		return err
	}
	_, err := st.Users().InsertSystemAccount(
		ctx, ReservedUsernameGuest, GuestPasswordSentinel, store.RoleGuest, false,
	)
	return err
}
