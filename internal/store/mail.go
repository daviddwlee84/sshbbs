package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Mail struct {
	ID            int64
	ThreadID      int64
	ParentID      sql.NullInt64
	FromUserID    int64
	FromUserIDStr string
	ToUserID      int64
	Subject       string
	Body          string
	ReadAt        sql.NullTime
	CreatedAt     time.Time
}

type MailRepo struct{ s *Store }

var ErrMailNotFound = errors.New("mail not found")

const mailColumns = `id, thread_id, parent_id, from_user_id, from_userid, to_user_id, subject, body, read_at, created_at`

func scanMail(row interface{ Scan(...any) error }) (*Mail, error) {
	m := &Mail{}
	if err := row.Scan(
		&m.ID, &m.ThreadID, &m.ParentID, &m.FromUserID, &m.FromUserIDStr,
		&m.ToUserID, &m.Subject, &m.Body, &m.ReadAt, &m.CreatedAt,
	); err != nil {
		return nil, err
	}
	return m, nil
}

// Insert creates a new mail. If parentID is nil, this is a thread root and
// thread_id is back-filled to the new row's own id under writeMu so the
// invariant `thread_id IS NOT NULL` holds for every read.
func (r *MailRepo) Insert(ctx context.Context, fromID int64, fromUserID string, toID int64, subject, body string, parentID *int64) (*Mail, error) {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()

	var threadID sql.NullInt64
	var parent sql.NullInt64
	if parentID != nil {
		parent = sql.NullInt64{Int64: *parentID, Valid: true}
		// Look up the parent's thread_id so the new reply joins the same thread.
		row := r.s.db.QueryRowContext(ctx, `SELECT thread_id FROM mail WHERE id = ?`, *parentID)
		var t sql.NullInt64
		if err := row.Scan(&t); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrMailNotFound
			}
			return nil, err
		}
		threadID = t
	}

	res, err := r.s.db.ExecContext(ctx,
		`INSERT INTO mail (thread_id, parent_id, from_user_id, from_userid, to_user_id, subject, body) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		threadID, parent, fromID, fromUserID, toID, subject, body,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if !threadID.Valid {
		// Root mail — thread_id = id.
		if _, err := r.s.db.ExecContext(ctx, `UPDATE mail SET thread_id = ? WHERE id = ?`, id, id); err != nil {
			return nil, err
		}
	}
	row := r.s.db.QueryRowContext(ctx, `SELECT `+mailColumns+` FROM mail WHERE id = ?`, id)
	return scanMail(row)
}

// ListInboxFor returns the recipient's inbox: unread first, then read,
// each group ordered newest-first. Mirrors WaterBalloonRepo.ListInboxFor.
func (r *MailRepo) ListInboxFor(ctx context.Context, toUserID int64, limit int) ([]*Mail, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+mailColumns+` FROM mail WHERE to_user_id = ?
		 ORDER BY (read_at IS NULL) DESC, id DESC LIMIT ?`,
		toUserID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Mail
	for rows.Next() {
		m, err := scanMail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListThread returns every mail in a thread, oldest first (chronological
// reading order — matches how the article-view-style thread screen reads).
func (r *MailRepo) ListThread(ctx context.Context, threadID int64) ([]*Mail, error) {
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+mailColumns+` FROM mail WHERE thread_id = ? ORDER BY id ASC`,
		threadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Mail
	for rows.Next() {
		m, err := scanMail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *MailRepo) GetByID(ctx context.Context, id int64) (*Mail, error) {
	row := r.s.db.QueryRowContext(ctx, `SELECT `+mailColumns+` FROM mail WHERE id = ?`, id)
	m, err := scanMail(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMailNotFound
	}
	return m, err
}

func (r *MailRepo) MarkRead(ctx context.Context, id int64) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE mail SET read_at = CURRENT_TIMESTAMP WHERE id = ? AND read_at IS NULL`, id,
	)
	return err
}

func (r *MailRepo) MarkAllReadFor(ctx context.Context, toUserID int64) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE mail SET read_at = CURRENT_TIMESTAMP WHERE to_user_id = ? AND read_at IS NULL`,
		toUserID,
	)
	return err
}

// CountUnreadFor returns the number of unread mails for the recipient,
// useful for the main-menu badge.
func (r *MailRepo) CountUnreadFor(ctx context.Context, toUserID int64) (int, error) {
	row := r.s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mail WHERE to_user_id = ? AND read_at IS NULL`, toUserID,
	)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
