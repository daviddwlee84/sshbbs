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
