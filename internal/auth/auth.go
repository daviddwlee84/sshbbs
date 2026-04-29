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

// ReservedUsernameNew is the SSH username that triggers the in-TUI register
// flow instead of a real DB login.
const ReservedUsernameNew = "new"

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
	ErrReservedUsername = fmt.Errorf("user id %q is reserved", ReservedUsernameNew)
	ErrBadCredentials   = errors.New("invalid user id or password")
)

var userIDRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{2,11}$`)

// Register validates inputs, hashes the password with bcrypt, and inserts a
// new user. Returns ErrUserExists on UNIQUE conflict.
func Register(ctx context.Context, st *store.Store, userID, password, nickname, email string) (*store.User, error) {
	userID = strings.TrimSpace(userID)
	nickname = strings.TrimSpace(nickname)
	email = strings.TrimSpace(email)

	if !userIDRe.MatchString(userID) {
		return nil, ErrInvalidUserID
	}
	if strings.EqualFold(userID, ReservedUsernameNew) {
		return nil, ErrReservedUsername
	}
	if l := len(password); l < minPasswordLen || l > maxPasswordLen {
		return nil, ErrInvalidPassword
	}
	if len([]rune(nickname)) > maxNickname {
		return nil, ErrNicknameTooLong
	}
	if len(email) > maxEmail {
		return nil, ErrEmailTooLong
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u, err := st.Users().Create(ctx, userID, string(hash), nickname, email)
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
	if err := st.Users().NoteLogin(ctx, u.ID, host); err != nil {
		return nil, fmt.Errorf("note login: %w", err)
	}
	// Re-fetch so callers see the updated last_login_at / num_logins.
	return st.Users().GetByID(ctx, u.ID)
}
