package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

// Reserved SSH usernames. `new` triggers the in-TUI register flow;
// `guest` short-circuits to a read-only spectator account; `admin` is
// the bootstrapped administrator and may not be re-registered.
const (
	ReservedUsernameNew   = "new"
	ReservedUsernameGuest = "guest"
	ReservedUsernameAdmin = "admin"
)

// IsReservedUsername reports whether s matches one of the three reserved
// SSH usernames (case-insensitive). Used by Register to refuse the names
// and by the SSH password callback to dispatch to the right flow.
func IsReservedUsername(s string) bool {
	s = strings.TrimSpace(s)
	return strings.EqualFold(s, ReservedUsernameNew) ||
		strings.EqualFold(s, ReservedUsernameGuest) ||
		strings.EqualFold(s, ReservedUsernameAdmin)
}

const (
	bcryptCost     = 12
	minPasswordLen = 6
	maxPasswordLen = 128
	maxNickname    = 32
	maxEmail       = 128
)

var (
	ErrUserExists       = errors.New("user already exists")
	ErrInvalidUserID    = errors.New("user id must be 3-12 chars, start with a letter, and use only letters/digits/underscore")
	ErrInvalidPassword  = fmt.Errorf("password must be %d-%d characters", minPasswordLen, maxPasswordLen)
	ErrNicknameTooLong  = fmt.Errorf("nickname must be %d characters or fewer", maxNickname)
	ErrEmailTooLong     = fmt.Errorf("email must be %d characters or fewer", maxEmail)
	ErrReservedUsername = fmt.Errorf("user id is reserved (%q / %q / %q)",
		ReservedUsernameNew, ReservedUsernameGuest, ReservedUsernameAdmin)
	ErrBadCredentials   = errors.New("invalid user id or password")
)

var userIDRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{2,11}$`)

// ValidatePassword enforces the length rules. Used by Register and the
// password-change screen so the rules don't drift between call sites.
func ValidatePassword(password string) error {
	if l := len(password); l < minPasswordLen || l > maxPasswordLen {
		return ErrInvalidPassword
	}
	return nil
}

// HashPassword returns a bcrypt hash at the package's standard cost.
// Callers outside auth (TUI, seed) use this so bcryptCost stays in one place.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPasswordHash returns nil when the hash matches the password.
// Callers outside auth use this instead of importing bcrypt directly.
func VerifyPasswordHash(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// Register validates inputs, hashes the password with bcrypt, and inserts a
// new user. Returns ErrUserExists on UNIQUE conflict.
func Register(ctx context.Context, st *store.Store, userID, password, nickname, email string) (*store.User, error) {
	userID = strings.TrimSpace(userID)
	nickname = strings.TrimSpace(nickname)
	email = strings.TrimSpace(email)

	if !userIDRe.MatchString(userID) {
		return nil, ErrInvalidUserID
	}
	if IsReservedUsername(userID) {
		return nil, ErrReservedUsername
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	if len([]rune(nickname)) > maxNickname {
		return nil, ErrNicknameTooLong
	}
	if len(email) > maxEmail {
		return nil, ErrEmailTooLong
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	u, err := st.Users().Create(ctx, userID, hash, nickname, email)
	if err != nil {
		// modernc.org/sqlite returns a wrapped UNIQUE error; check the text.
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return u, nil
}

// VerifyLogin returns the user record if the password matches, ErrBadCredentials otherwise.
// On success, NoteLogin is called to update last_login_at / last_host / num_logins.
func VerifyLogin(ctx context.Context, st *store.Store, userID, password, host string) (*store.User, error) {
	u, err := st.Users().GetByUserID(ctx, strings.TrimSpace(userID))
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			// Run a dummy bcrypt check anyway to even out timing.
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvali"), []byte(password))
			return nil, ErrBadCredentials
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrBadCredentials
	}
	// Skip NoteLogin for guest — the row is shared across every spectator
	// session, so num_logins / last_login_at are meaningless and the write
	// would just create contention. Defensive: the SSH password callback
	// short-circuits guest before calling VerifyLogin in normal flow.
	if u.Role == store.RoleGuest {
		return u, nil
	}
	if err := st.Users().NoteLogin(ctx, u.ID, host); err != nil {
		return nil, fmt.Errorf("note login: %w", err)
	}
	// Re-fetch so callers see the updated last_login_at / num_logins.
	return st.Users().GetByID(ctx, u.ID)
}
