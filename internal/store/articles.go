package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// CommentsMode controls who can leave 推/噓/→ pushes on an article. The
// values are persisted as TEXT in articles.comments_mode (see migration
// 0008). States are mutually exclusive — 'locked' is strictly stronger
// than 'arrows_only', so a single-column enum keeps the precedence rule
// unambiguous. Mod-only mutator: ArticleRepo.SetCommentsMode.
type CommentsMode string

const (
	CommentsModeOpen       CommentsMode = "open"        // default — push, boo, arrow all allowed
	CommentsModeArrowsOnly CommentsMode = "arrows_only" // only neutral arrow comments
	CommentsModeLocked     CommentsMode = "locked"      // all pushes rejected
)

func (m CommentsMode) Valid() bool {
	switch m {
	case CommentsModeOpen, CommentsModeArrowsOnly, CommentsModeLocked:
		return true
	}
	return false
}

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
	CommentsMode   CommentsMode
}

type ArticleRepo struct{ s *Store }

// ArticleSort selects the secondary ordering inside ListByBoardOpts. Pinned
// articles always float to the top regardless of sort — pin is an explicit
// moderation signal stronger than score or recency.
type ArticleSort int

const (
	SortNewestFirst ArticleSort = iota // default: created_at DESC, id DESC
	SortByScoreDesc                    // recommend_score DESC, then created_at DESC, id DESC tiebreaker
)

// ListArticlesOpts parameterises ListByBoardOpts. Zero value behaves
// identically to the legacy ListByBoard(boardID, 0): newest-first, no title
// filter, default 50-row limit.
type ListArticlesOpts struct {
	Limit       int         // <=0 → 50
	TitleSearch string      // empty → no filter; matched as LIKE '%q%' COLLATE NOCASE
	Sort        ArticleSort
}

// likeEscape protects user input from being interpreted as LIKE wildcards.
// Paired with `ESCAPE '\\'` in the SQL.
var likeEscape = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

var (
	ErrArticleNotFound     = errors.New("article not found")
	ErrPermissionDenied    = errors.New("permission denied")
	ErrInvalidCommentsMode = errors.New("invalid comments mode")
)

const articleColumns = `id, board_id, author_id, author_userid, title, body,
	recommend_score, filemode, created_at, updated_at, pinned_at, comments_mode`

func scanArticle(row interface{ Scan(...any) error }) (*Article, error) {
	a := &Article{}
	var commentsMode string
	err := row.Scan(
		&a.ID, &a.BoardID, &a.AuthorID, &a.AuthorUserID, &a.Title, &a.Body,
		&a.RecommendScore, &a.Filemode, &a.CreatedAt, &a.UpdatedAt, &a.PinnedAt,
		&commentsMode,
	)
	a.CommentsMode = CommentsMode(commentsMode)
	return a, err
}

// ListByBoard returns the most-recent articles in a board, pinned first.
// Thin shim over ListByBoardOpts kept so existing call sites and tests
// (TestArticles_ListByBoard_NewestFirst, _Limit, _PinnedFirst) continue to
// exercise the legacy default behaviour as a regression guard.
func (r *ArticleRepo) ListByBoard(ctx context.Context, boardID int64, limit int) ([]*Article, error) {
	return r.ListByBoardOpts(ctx, boardID, ListArticlesOpts{Limit: limit})
}

// ListByBoardOpts is the parameterised list endpoint that supports a
// case-insensitive title substring filter and an alternate score-DESC sort.
//
// Pinned articles always surface first ((pinned_at IS NULL) sorts 0 before 1).
// Within each pinned/non-pinned group the secondary ordering depends on
// opts.Sort:
//   - SortNewestFirst:  created_at DESC, id DESC
//   - SortByScoreDesc:  recommend_score DESC, then created_at DESC, id DESC
//
// The created_at/id tiebreaker is essential in score mode because many fresh
// posts share recommend_score=0 and need a deterministic secondary order.
//
// TODO(score-vs-pushcount): SortByScoreDesc orders by recommend_score (signed
// sum: +1 push, -1 boo, 0 arrow). A future "raw 推文量" sort would need a
// separate maintained push_count column.
func (r *ArticleRepo) ListByBoardOpts(ctx context.Context, boardID int64, opts ListArticlesOpts) ([]*Article, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	var (
		where = "WHERE board_id = ?"
		args  = []any{boardID}
	)
	if q := strings.TrimSpace(opts.TitleSearch); q != "" {
		where += ` AND title LIKE ? ESCAPE '\' COLLATE NOCASE`
		args = append(args, "%"+likeEscape.Replace(q)+"%")
	}

	orderBy := `ORDER BY (pinned_at IS NULL), created_at DESC, id DESC`
	if opts.Sort == SortByScoreDesc {
		orderBy = `ORDER BY (pinned_at IS NULL), recommend_score DESC, created_at DESC, id DESC`
	}

	args = append(args, limit)
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+articleColumns+` FROM articles `+where+` `+orderBy+` LIMIT ?`,
		args...,
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

// SetCommentsMode flips an article's comments_mode (open / arrows_only /
// locked). Permission shape mirrors SetPinned: requesterRole must be
// at-or-above mod, and unlike Update/Delete this does NOT admit the
// article author — locking comments is a moderation action. Returns
// ErrInvalidCommentsMode if mode is not one of the three constants,
// ErrPermissionDenied / ErrArticleNotFound otherwise. requesterID is
// accepted for signature parity with the other mutators (Pin/Update/Delete)
// but unused for the gate.
func (r *ArticleRepo) SetCommentsMode(ctx context.Context, articleID, requesterID int64, requesterRole Role, mode CommentsMode) error {
	if !mode.Valid() {
		return ErrInvalidCommentsMode
	}
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
	_, err = r.s.db.ExecContext(ctx,
		`UPDATE articles SET comments_mode = ? WHERE id = ?`, string(mode), articleID,
	)
	return err
}
