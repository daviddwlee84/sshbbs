package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Board struct {
	ID          int64
	Name        string
	Title       string
	Description string
	BM          string
	Attr        int64
	CreatedAt   time.Time
}

type BoardRepo struct{ s *Store }

var ErrBoardNotFound = errors.New("board not found")

const boardColumns = `id, name, title, description, bm, attr, created_at`

func scanBoard(row interface{ Scan(...any) error }) (*Board, error) {
	b := &Board{}
	err := row.Scan(&b.ID, &b.Name, &b.Title, &b.Description, &b.BM, &b.Attr, &b.CreatedAt)
	return b, err
}

func (r *BoardRepo) List(ctx context.Context) ([]*Board, error) {
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+boardColumns+` FROM boards ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Board
	for rows.Next() {
		b, err := scanBoard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *BoardRepo) GetByID(ctx context.Context, id int64) (*Board, error) {
	row := r.s.db.QueryRowContext(ctx, `SELECT `+boardColumns+` FROM boards WHERE id = ?`, id)
	b, err := scanBoard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBoardNotFound
	}
	return b, err
}

func (r *BoardRepo) GetByName(ctx context.Context, name string) (*Board, error) {
	row := r.s.db.QueryRowContext(ctx, `SELECT `+boardColumns+` FROM boards WHERE name = ? COLLATE NOCASE`, name)
	b, err := scanBoard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBoardNotFound
	}
	return b, err
}

// SeedDefaults is a belt-and-braces complement to migration 0002. It runs
// every startup; INSERT OR IGNORE makes it a no-op once the rows exist.
func (r *BoardRepo) SeedDefaults(ctx context.Context) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO boards (name, title, description) VALUES
			('Welcome',  '歡迎來到 SSH-BBS', 'System welcome and announcements'),
			('Test',     '測試板',           'Try things here. Posts may be cleared periodically.'),
			('ChitChat', '閒聊',             'General chat. Be kind.')
	`)
	return err
}
