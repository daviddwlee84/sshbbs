package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Article struct {
	ID             int64
	BoardID        int64
	AuthorID       int64
	AuthorUserID   string
	Title          string
	Body           string
	RecommendScore int64
	Filemode       int64
	CreatedAt      time.Time
	UpdatedAt      sql.NullTime
	PinnedAt       sql.NullTime
}

type ArticleRepo struct{ s *Store }

var (
	ErrArticleNotFound  = errors.New("article not found")
	ErrPermissionDenied = errors.New("permission denied")
)

const articleColumns = `id, board_id, author_id, author_userid, title, body,
	recommend_score, filemode, created_at, updated_at, pinned_at`

func scanArticle(row interface{ Scan(...any) error }) (*Article, error) {
	a := &Article{}
	err := row.Scan(
		&a.ID, &a.BoardID, &a.AuthorID, &a.AuthorUserID, &a.Title, &a.Body,
		&a.RecommendScore, &a.Filemode, &a.CreatedAt, &a.UpdatedAt, &a.PinnedAt,
	)
	return a, err
}

func (r *ArticleRepo) ListByBoard(ctx context.Context, boardID int64, limit int) ([]*Article, error) {
	if limit <= 0 {
		limit = 50
	}
	// Pinned articles surface first ((pinned_at IS NULL) sorts 0 before 1);
	// within each group, newest first by created_at then id (the latter
	// disambiguates rows that share a second-resolution timestamp).
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+articleColumns+` FROM articles WHERE board_id = ?
		 ORDER BY (pinned_at IS NULL), created_at DESC, id DESC LIMIT ?`,
		boardID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *ArticleRepo) GetByID(ctx context.Context, id int64) (*Article, error) {
	row := r.s.db.QueryRowContext(ctx, `SELECT `+articleColumns+` FROM articles WHERE id = ?`, id)
	a, err := scanArticle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrArticleNotFound
	}
	return a, err
}

// NeighboursOf returns the article IDs immediately older (prev) and newer
// (next) than articleID within the same board, or 0 when there is no
// neighbour on that side. "Older" / "newer" are by id (monotonic insert
// order); this matches the user-visible "[ / ]" semantics in article view.
func (r *ArticleRepo) NeighboursOf(ctx context.Context, boardID, articleID int64) (prev, next int64, err error) {
	row := r.s.db.QueryRowContext(ctx,
		`SELECT id FROM articles WHERE board_id = ? AND id < ? ORDER BY id DESC LIMIT 1`,
		boardID, articleID,
	)
	if scanErr := row.Scan(&prev); scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		return 0, 0, scanErr
	}
	row = r.s.db.QueryRowContext(ctx,
		`SELECT id FROM articles WHERE board_id = ? AND id > ? ORDER BY id ASC LIMIT 1`,
		boardID, articleID,
	)
	if scanErr := row.Scan(&next); scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		return 0, 0, scanErr
	}
	return prev, next, nil
}

func (r *ArticleRepo) Create(ctx context.Context, boardID, authorID int64, authorUserID, title, body string) (*Article, error) {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	res, err := r.s.db.ExecContext(ctx,
		`INSERT INTO articles (board_id, author_id, author_userid, title, body) VALUES (?, ?, ?, ?, ?)`,
		boardID, authorID, authorUserID, title, body,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	row := r.s.db.QueryRowContext(ctx, `SELECT `+articleColumns+` FROM articles WHERE id = ?`, id)
	return scanArticle(row)
}

// Delete hard-deletes the article. Permission: requester must be the author
// OR have a role at-or-above mod. Returns ErrArticleNotFound when the row
// is gone, ErrPermissionDenied when the requester is unauthorized.
// Cascades to pushes via the FK on pushes.article_id.
func (r *ArticleRepo) Delete(ctx context.Context, articleID, requesterID int64, requesterRole Role) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	var authorID int64
	err := r.s.db.QueryRowContext(ctx,
		`SELECT author_id FROM articles WHERE id = ?`, articleID,
	).Scan(&authorID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrArticleNotFound
	}
	if err != nil {
		return err
	}
	if authorID != requesterID && !requesterRole.AtLeast(RoleMod) {
		return ErrPermissionDenied
	}
	_, err = r.s.db.ExecContext(ctx, `DELETE FROM articles WHERE id = ?`, articleID)
	return err
}

// Update mutates the article's title and body and sets updated_at to now.
// Permission mirrors Delete: requester must be the author OR have a role
// at-or-above mod. Returns ErrArticleNotFound / ErrPermissionDenied. Score,
// board, and author_id are intentionally not editable here — those would be
// separate concerns (move-board / re-attribute) outside the user-facing
// edit flow.
func (r *ArticleRepo) Update(ctx context.Context, articleID, requesterID int64, requesterRole Role, newTitle, newBody string) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	var authorID int64
	err := r.s.db.QueryRowContext(ctx,
		`SELECT author_id FROM articles WHERE id = ?`, articleID,
	).Scan(&authorID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrArticleNotFound
	}
	if err != nil {
		return err
	}
	if authorID != requesterID && !requesterRole.AtLeast(RoleMod) {
		return ErrPermissionDenied
	}
	_, err = r.s.db.ExecContext(ctx,
		`UPDATE articles SET title = ?, body = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newTitle, newBody, articleID,
	)
	return err
}

// SetPinned pins (pinned=true) or unpins (pinned=false) an article. Permission
// shape mirrors BoardRepo.UpdateBanner: requesterRole must be at-or-above
// mod. Pinning is a moderation action — unlike Update/Delete it does NOT
// admit the article author. Returns ErrArticleNotFound when the row is gone,
// ErrPermissionDenied when the requester is unauthorized.
//
// Multi-pin per board is supported: the call simply sets pinned_at; nothing
// rejects a second pinned row on the same board.
func (r *ArticleRepo) SetPinned(ctx context.Context, articleID, requesterID int64, requesterRole Role, pinned bool) error {
	if !requesterRole.AtLeast(RoleMod) {
		return ErrPermissionDenied
	}
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	var exists int64
	err := r.s.db.QueryRowContext(ctx,
		`SELECT id FROM articles WHERE id = ?`, articleID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrArticleNotFound
	}
	if err != nil {
		return err
	}
	if pinned {
		_, err = r.s.db.ExecContext(ctx,
			`UPDATE articles SET pinned_at = CURRENT_TIMESTAMP WHERE id = ?`, articleID,
		)
	} else {
		_, err = r.s.db.ExecContext(ctx,
			`UPDATE articles SET pinned_at = NULL WHERE id = ?`, articleID,
		)
	}
	return err
}
