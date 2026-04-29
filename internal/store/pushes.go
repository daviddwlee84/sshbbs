package store

import (
	"context"
	"fmt"
	"time"
)

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

// Create inserts a push and updates the cached recommend_score on the
// article (push: +1, boo: -1, arrow: 0). Both writes happen under the
// store-level writeMu so they're effectively atomic from the app's view;
// SQLite's WAL gives us read-consistency for the recompute.
func (r *PushRepo) Create(ctx context.Context, articleID, userID int64, userUserID string, kind PushKind, body string) (*Push, error) {
	delta := 0
	switch kind {
	case PushKindPush:
		delta = 1
	case PushKindBoo:
		delta = -1
	case PushKindArrow:
		delta = 0
	default:
		return nil, fmt.Errorf("invalid push kind: %q", kind)
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
