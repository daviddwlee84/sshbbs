package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID            int64
	UserID        string
	PasswordHash  string
	Nickname      string
	RealName      string
	Email         string
	NumLogins     int64
	NumPosts      int64
	LastLoginAt   sql.NullTime
	LastHost      sql.NullString
	CreatedAt     time.Time
}

type UserRepo struct{ s *Store }

var ErrUserNotFound = errors.New("user not found")

const userColumns = `id, user_id, password_hash, nickname, realname, email,
	num_logins, num_posts, last_login_at, last_host, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(
		&u.ID, &u.UserID, &u.PasswordHash, &u.Nickname, &u.RealName, &u.Email,
		&u.NumLogins, &u.NumPosts, &u.LastLoginAt, &u.LastHost, &u.CreatedAt,
	)
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
