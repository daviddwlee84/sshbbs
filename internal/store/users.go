package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID                 int64
	UserID             string
	PasswordHash       string
	Nickname           string
	RealName           string
	Email              string
	NumLogins          int64
	NumPosts           int64
	LastLoginAt        sql.NullTime
	LastHost           sql.NullString
	CreatedAt          time.Time
	Role               Role
	MustChangePassword bool
	Bio                string
	Locale             string
}

type UserRepo struct{ s *Store }

var (
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidRole  = errors.New("invalid role")
)

const userColumns = `id, user_id, password_hash, nickname, realname, email,
	num_logins, num_posts, last_login_at, last_host, created_at, role, must_change_password, bio, locale`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	var mustChange int64
	err := row.Scan(
		&u.ID, &u.UserID, &u.PasswordHash, &u.Nickname, &u.RealName, &u.Email,
		&u.NumLogins, &u.NumPosts, &u.LastLoginAt, &u.LastHost, &u.CreatedAt,
		&u.Role, &mustChange, &u.Bio, &u.Locale,
	)
	u.MustChangePassword = mustChange != 0
	return u, err
}

func (r *UserRepo) Create(ctx context.Context, userID, passwordHash, nickname, email string) (*User, error) {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	res, err := r.s.db.ExecContext(ctx,
		`INSERT INTO users (user_id, password_hash, nickname, email) VALUES (?, ?, ?, ?)`,
		userID, passwordHash, nickname, email,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *UserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	row := r.s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return u, err
}

func (r *UserRepo) GetByUserID(ctx context.Context, userID string) (*User, error) {
	row := r.s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE user_id = ? COLLATE NOCASE`, userID)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	return u, err
}

func (r *UserRepo) NoteLogin(ctx context.Context, id int64, host string) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = CURRENT_TIMESTAMP, last_host = ?, num_logins = num_logins + 1 WHERE id = ?`,
		host, id,
	)
	return err
}

func (r *UserRepo) IncrementPosts(ctx context.Context, id int64) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE users SET num_posts = num_posts + 1 WHERE id = ?`, id,
	)
	return err
}

// InsertSystemAccount creates a privileged user (admin or guest) with the
// given role and must_change_password flag, bypassing the regular Create
// path which always defaults role='user'. Used only by the bootstrap seed.
func (r *UserRepo) InsertSystemAccount(ctx context.Context, userID, passwordHash string, role Role, mustChangePassword bool) (*User, error) {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	mc := int64(0)
	if mustChangePassword {
		mc = 1
	}
	res, err := r.s.db.ExecContext(ctx,
		`INSERT INTO users (user_id, password_hash, role, must_change_password) VALUES (?, ?, ?, ?)`,
		userID, passwordHash, string(role), mc,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// SetPassword updates the password hash and clears must_change_password
// in one statement. The TUI password-change screen calls this after
// validating the current password and the new-password rules.
func (r *UserRepo) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, must_change_password = 0 WHERE id = ?`,
		passwordHash, id,
	)
	return err
}

// SetLocale updates the user's UI locale preference. Callers (the
// locale-settings screen) are responsible for validating the value via
// i18n.Valid before invoking — the store accepts any string so seed/admin
// paths can write to it without importing internal/i18n.
func (r *UserRepo) SetLocale(ctx context.Context, id int64, locale string) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE users SET locale = ? WHERE id = ?`, locale, id,
	)
	return err
}

// SetBio updates the user's profile bio. Callers (the bio-edit screen) are
// responsible for length validation via auth.ValidateBio.
func (r *UserRepo) SetBio(ctx context.Context, id int64, bio string) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE users SET bio = ? WHERE id = ?`, bio, id,
	)
	return err
}

// SetRole updates the role of the given user. Callers (admin user-management
// screen) are responsible for enforcing higher-level invariants like the
// last-admin guard via CountByRole.
func (r *UserRepo) SetRole(ctx context.Context, id int64, role Role) error {
	if !role.Valid() {
		return ErrInvalidRole
	}
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE users SET role = ? WHERE id = ?`, string(role), id,
	)
	return err
}

// ListAll returns users ordered by id, paginated. limit<=0 defaults to 20.
func (r *UserRepo) ListAll(ctx context.Context, limit, offset int) ([]*User, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+userColumns+` FROM users ORDER BY id ASC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountByRole returns the number of users currently assigned a given role.
// Used by the admin screen's last-admin guard.
func (r *UserRepo) CountByRole(ctx context.Context, role Role) (int, error) {
	var n int
	err := r.s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE role = ?`, string(role),
	).Scan(&n)
	return n, err
}
