package store

import (
	"context"
	"database/sql"
	"time"
)

type WaterBalloon struct {
	ID            int64
	FromUserID    int64
	FromUserIDStr string
	ToUserID      int64
	Body          string
	DeliveredLive bool
	ReadAt        sql.NullTime
	CreatedAt     time.Time
}

// WBCounterparty is one row of the per-counterparty inbox roll-up. The fields
// describe the OTHER user (not the viewer): UserID/UserIDStr is the canonical
// identity from the users table (so a renamed handle reads correctly),
// LastBody/LastFromMe/LastAt summarise the latest message exchanged with them,
// and UnreadCount is how many they've sent the viewer that aren't read yet.
type WBCounterparty struct {
	UserID      int64
	UserIDStr   string
	LastBody    string
	LastFromMe  bool
	LastAt      time.Time
	UnreadCount int64
}

type WaterBalloonRepo struct{ s *Store }

const wbColumns = `id, from_user_id, from_userid, to_user_id, body, delivered_live, read_at, created_at`

func scanWB(row interface{ Scan(...any) error }) (*WaterBalloon, error) {
	w := &WaterBalloon{}
	var deliveredLive int64
	if err := row.Scan(&w.ID, &w.FromUserID, &w.FromUserIDStr, &w.ToUserID, &w.Body, &deliveredLive, &w.ReadAt, &w.CreatedAt); err != nil {
		return nil, err
	}
	w.DeliveredLive = deliveredLive != 0
	return w, nil
}

func (r *WaterBalloonRepo) Insert(ctx context.Context, fromID int64, fromUserID string, toID int64, body string, deliveredLive bool) (*WaterBalloon, error) {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	live := int64(0)
	if deliveredLive {
		live = 1
	}
	res, err := r.s.db.ExecContext(ctx,
		`INSERT INTO water_balloons (from_user_id, from_userid, to_user_id, body, delivered_live) VALUES (?, ?, ?, ?, ?)`,
		fromID, fromUserID, toID, body, live,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	row := r.s.db.QueryRowContext(ctx, `SELECT `+wbColumns+` FROM water_balloons WHERE id = ?`, id)
	return scanWB(row)
}

// ListUnreadFor returns unread water balloons for the recipient, oldest
// first, so the TUI can replay them on reconnect.
func (r *WaterBalloonRepo) ListUnreadFor(ctx context.Context, toUserID int64) ([]*WaterBalloon, error) {
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+wbColumns+` FROM water_balloons WHERE to_user_id = ? AND read_at IS NULL ORDER BY id ASC`,
		toUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WaterBalloon
	for rows.Next() {
		w, err := scanWB(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ListInboxFor returns the recipient's inbox: unread first (newest unread
// up top), then read messages newest-first.
func (r *WaterBalloonRepo) ListInboxFor(ctx context.Context, toUserID int64, limit int) ([]*WaterBalloon, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+wbColumns+` FROM water_balloons WHERE to_user_id = ?
		 ORDER BY (read_at IS NULL) DESC, id DESC LIMIT ?`,
		toUserID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WaterBalloon
	for rows.Next() {
		w, err := scanWB(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *WaterBalloonRepo) MarkRead(ctx context.Context, id int64) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE water_balloons SET read_at = CURRENT_TIMESTAMP WHERE id = ? AND read_at IS NULL`, id,
	)
	return err
}

func (r *WaterBalloonRepo) MarkAllReadFor(ctx context.Context, toUserID int64) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE water_balloons SET read_at = CURRENT_TIMESTAMP WHERE to_user_id = ? AND read_at IS NULL`,
		toUserID,
	)
	return err
}

// GetByID fetches a single water balloon. Used by the thread screen's
// live-append path so a freshly-arrived WBIncomingMsg can be turned into
// a canonical row (with read_at, created_at) rather than reconstructed.
func (r *WaterBalloonRepo) GetByID(ctx context.Context, id int64) (*WaterBalloon, error) {
	row := r.s.db.QueryRowContext(ctx, `SELECT `+wbColumns+` FROM water_balloons WHERE id = ?`, id)
	return scanWB(row)
}

// ListConversation returns every water balloon between viewerID and
// counterpartyID, both directions, oldest-first so the chat-style view can
// append at the bottom. Phase 1 doesn't paginate; the limit clips oldest.
// TODO: when "load older" lands, switch to ORDER BY id DESC + offset and
// reverse client-side.
func (r *WaterBalloonRepo) ListConversation(ctx context.Context, viewerID, counterpartyID int64, limit int) ([]*WaterBalloon, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT `+wbColumns+` FROM water_balloons
		 WHERE (from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?)
		 ORDER BY id ASC LIMIT ?`,
		viewerID, counterpartyID, counterpartyID, viewerID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WaterBalloon
	for rows.Next() {
		w, err := scanWB(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ListCounterpartiesFor returns one row per counterparty the viewer has
// exchanged water balloons with, sorted unread-first then by recency. The
// counterparty handle (UserIDStr) is read via JOIN on users.id so it tracks
// the user's current handle rather than the (potentially stale) from_userid
// snapshot stored on the row at insert time.
//
// MAX(id) is used to resolve the latest row per counterparty (not
// MAX(created_at)) because SQLite's CURRENT_TIMESTAMP only has 1-second
// resolution and rapid-fire inserts within the same second would tie.
//
// TODO: when user delete lands, the inner JOIN to users will silently drop
// conversations whose counterparty no longer exists. Switch to LEFT JOIN
// with a fallback handle (e.g., COALESCE(u.user_id, '[deleted]')) at that
// point.
func (r *WaterBalloonRepo) ListCounterpartiesFor(ctx context.Context, viewerID int64, limit int) ([]*WBCounterparty, error) {
	if limit <= 0 {
		limit = 50
	}
	// last_at is r.created_at from the JOIN'd latest row (resolved via
	// MAX(id), since SQLite's CURRENT_TIMESTAMP only has 1-second resolution
	// and would tie under rapid-fire inserts). We deliberately don't return
	// MAX(created_at) from agg — modernc/sqlite drops the time.Time type hint
	// across aggregates and returns it as a string, which would fail Scan.
	const q = `
		WITH related AS (
			SELECT id, from_user_id, to_user_id, body, read_at, created_at,
			       CASE WHEN from_user_id = ? THEN to_user_id ELSE from_user_id END AS cp_id
			FROM water_balloons
			WHERE from_user_id = ? OR to_user_id = ?
		),
		agg AS (
			SELECT cp_id,
			       MAX(id) AS last_id,
			       SUM(CASE WHEN to_user_id = ? AND read_at IS NULL THEN 1 ELSE 0 END) AS unread_count
			FROM related
			WHERE cp_id != ? -- hide self-WBs (self-thread would deadlock anyway)
			GROUP BY cp_id
		)
		SELECT a.cp_id, u.user_id, r.body, (r.from_user_id = ?), r.created_at, a.unread_count
		FROM agg a
		JOIN related r ON r.id = a.last_id
		JOIN users    u ON u.id = a.cp_id
		ORDER BY (a.unread_count > 0) DESC, r.created_at DESC
		LIMIT ?`
	rows, err := r.s.db.QueryContext(ctx, q,
		viewerID, viewerID, viewerID, viewerID, viewerID, viewerID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*WBCounterparty
	for rows.Next() {
		c := &WBCounterparty{}
		var lastFromMe int64
		if err := rows.Scan(&c.UserID, &c.UserIDStr, &c.LastBody, &lastFromMe, &c.LastAt, &c.UnreadCount); err != nil {
			return nil, err
		}
		c.LastFromMe = lastFromMe != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarkConversationRead marks every inbound water balloon from counterpartyID
// to viewerID as read in one statement. Idempotent (the read_at IS NULL
// filter makes the second call a no-op). Outbound rows (viewer→counterparty)
// are deliberately untouched: those are already read by the sender.
func (r *WaterBalloonRepo) MarkConversationRead(ctx context.Context, viewerID, counterpartyID int64) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`UPDATE water_balloons SET read_at = CURRENT_TIMESTAMP
		 WHERE to_user_id = ? AND from_user_id = ? AND read_at IS NULL`,
		viewerID, counterpartyID,
	)
	return err
}
