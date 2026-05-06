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
	// Banner is per-board ANSI/ASCII-art text. Empty = no banner (column
	// NULL in DB). Edited by mod+ via the banner-edit screen; seeded from
	// internal/seed/banners on first boot.
	Banner string
}

type BoardRepo struct{ s *Store }

var ErrBoardNotFound = errors.New("board not found")

const boardColumns = `id, name, title, description, bm, attr, created_at, banner`

func scanBoard(row interface{ Scan(...any) error }) (*Board, error) {
	b := &Board{}
	var banner sql.NullString
	if err := row.Scan(&b.ID, &b.Name, &b.Title, &b.Description, &b.BM, &b.Attr, &b.CreatedAt, &banner); err != nil {
		return b, err
	}
	if banner.Valid {
		b.Banner = banner.String
	}
	return b, nil
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

// UpdateBanner sets the banner column on a board. Permission: requester
// must have a role at-or-above mod (the global mod role; per-board BM is
// not consulted). Returns ErrBoardNotFound if the board is missing,
// ErrPermissionDenied if the requester is unauthorized.
//
// Mirrors ArticleRepo.Update's permission shape; takes writeMu for the
// same reason (single-statement write is fine, but keeping the lock
// discipline consistent across repos is the point).
func (r *BoardRepo) UpdateBanner(ctx context.Context, boardID, requesterID int64, requesterRole Role, banner string) error {
	if !requesterRole.AtLeast(RoleMod) {
		return ErrPermissionDenied
	}
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	var exists int64
	err := r.s.db.QueryRowContext(ctx, `SELECT id FROM boards WHERE id = ?`, boardID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBoardNotFound
	}
	if err != nil {
		return err
	}
	_, err = r.s.db.ExecContext(ctx, `UPDATE boards SET banner = ? WHERE id = ?`, banner, boardID)
	return err
}
