package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrPushNotFound is returned by PushRepo lookups (Delete, future GetByID)
// when the row is missing. ErrPermissionDenied is shared with articles.
var ErrPushNotFound = errors.New("push not found")

type PushKind string

const (
	PushKindPush  PushKind = "push"  // 推 (+1)
	PushKindBoo   PushKind = "boo"   // 噓 (-1)
	PushKindArrow PushKind = "arrow" // → (0)
)

func (k PushKind) Glyph() string {
	switch k {
	case PushKindPush:
		return "推"
	case PushKindBoo:
		return "噓"
	case PushKindArrow:
		return "→"
	}
	return "?"
}

type Push struct {
	ID          int64
	ArticleID   int64
	UserID      int64
	UserUserID  string
	Kind        PushKind
	Body        string
	CreatedAt   time.Time
}

type PushRepo struct{ s *Store }

const pushColumns = `id, article_id, user_id, user_userid, kind, body, created_at`

func scanPush(row interface{ Scan(...any) error }) (*Push, error) {
	p := &Push{}
	var kind string
	if err := row.Scan(&p.ID, &p.ArticleID, &p.UserID, &p.UserUserID, &kind, &p.Body, &p.CreatedAt); err != nil {
		return nil, err
	}
	p.Kind = PushKind(kind)
	return p, nil
}

func (r *PushRepo) ListByArticle(ctx context.Context, articleID int64) ([]*Push, error) {
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+pushColumns+` FROM pushes WHERE article_id = ? ORDER BY id ASC`, articleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Push
	for rows.Next() {
		p, err := scanPush(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// scoreDelta returns the recommend_score contribution of a push of the
// given kind. Positive on add; negate on delete. Centralized so Create
// and Delete agree on the algebra.
func scoreDelta(kind PushKind) (int, error) {
	switch kind {
	case PushKindPush:
		return 1, nil
	case PushKindBoo:
		return -1, nil
	case PushKindArrow:
		return 0, nil
	}
	return 0, fmt.Errorf("invalid push kind: %q", kind)
}

// Create inserts a push and updates the cached recommend_score on the
// article (push: +1, boo: -1, arrow: 0). Both writes happen under the
// store-level writeMu so they're effectively atomic from the app's view;
// SQLite's WAL gives us read-consistency for the recompute.
func (r *PushRepo) Create(ctx context.Context, articleID, userID int64, userUserID string, kind PushKind, body string) (*Push, error) {
	delta, err := scoreDelta(kind)
	if err != nil {
		return nil, err
	}

	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	tx, err := r.s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO pushes (article_id, user_id, user_userid, kind, body) VALUES (?, ?, ?, ?, ?)`,
		articleID, userID, userUserID, string(kind), body,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if delta != 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE articles SET recommend_score = recommend_score + ? WHERE id = ?`,
			delta, articleID,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	row := r.s.db.QueryRowContext(ctx, `SELECT `+pushColumns+` FROM pushes WHERE id = ?`, id)
	return scanPush(row)
}

// Delete hard-deletes a push and reverts its score contribution on the
// parent article. Permission: requester must be the push author OR have
// a role at-or-above mod. Returns ErrPermissionDenied / ErrPushNotFound.
// Mirrors Create's writeMu+tx discipline so concurrent deletes can't
// double-revert the score.
func (r *PushRepo) Delete(ctx context.Context, pushID, requesterID int64, requesterRole Role) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	tx, err := r.s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var authorID, articleID int64
	var kindStr string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id, article_id, kind FROM pushes WHERE id = ?`, pushID,
	).Scan(&authorID, &articleID, &kindStr)
	if err == sql.ErrNoRows {
		return ErrPushNotFound
	}
	if err != nil {
		return err
	}
	if authorID != requesterID && !requesterRole.AtLeast(RoleMod) {
		return ErrPermissionDenied
	}
	delta, err := scoreDelta(PushKind(kindStr))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pushes WHERE id = ?`, pushID); err != nil {
		return err
	}
	if delta != 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE articles SET recommend_score = recommend_score - ? WHERE id = ?`,
			delta, articleID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
