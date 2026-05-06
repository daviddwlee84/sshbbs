package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// NotifyPrefs is the per-user notification toggle bag. The boolean fields map
// 1:1 to the `on_*` columns in user_notif_prefs. Defaults (all event toggles
// on, only_when_offline off) are returned when the row is absent — the row is
// upserted lazily on the first SetPrefs call.
type NotifyPrefs struct {
	OnPush          bool
	OnWB            bool
	OnMail          bool
	OnReply         bool
	OnlyWhenOffline bool
}

// DefaultNotifyPrefs is what GetPrefs returns for users that have never
// touched the notify settings screen. Mirrors the column DEFAULTs in
// migrations/0009_user_settings_and_notify.sql.
func DefaultNotifyPrefs() NotifyPrefs {
	return NotifyPrefs{OnPush: true, OnWB: true, OnMail: true, OnReply: true}
}

// NotifyTarget is one configured webhook URL for a user. The dispatcher
// POSTs `{title, body}` JSON to URL when an event fires; Enabled lets the
// user pause a target without losing it.
type NotifyTarget struct {
	ID        int64
	UserID    int64
	Label     string
	URL       string
	Enabled   bool
	CreatedAt time.Time
}

// NotifyRepo owns the user_notif_prefs and user_notif_targets tables.
type NotifyRepo struct{ s *Store }

var (
	ErrNotifyTargetNotFound = errors.New("notify target not found")
	ErrInvalidNotifyURL     = errors.New("notify target url must be http(s)://...")
)

// ValidateNotifyURL enforces the bare minimum: scheme must be http or https.
// Anything beyond that (DNS, reachability) is the dispatcher's problem at
// delivery time — failing here would block legitimate self-hosted endpoints
// behind VPNs / private DNS.
func ValidateNotifyURL(u string) error {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return nil
	}
	return ErrInvalidNotifyURL
}

// GetPrefs returns the user's stored prefs, or DefaultNotifyPrefs() when
// the row is absent.
func (r *NotifyRepo) GetPrefs(ctx context.Context, userID int64) (NotifyPrefs, error) {
	row := r.s.db.QueryRowContext(ctx,
		`SELECT on_push, on_wb, on_mail, on_reply, only_when_offline
		 FROM user_notif_prefs WHERE user_id = ?`, userID,
	)
	var onPush, onWB, onMail, onReply, onlyOffline int64
	err := row.Scan(&onPush, &onWB, &onMail, &onReply, &onlyOffline)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultNotifyPrefs(), nil
	}
	if err != nil {
		return NotifyPrefs{}, err
	}
	return NotifyPrefs{
		OnPush:          onPush != 0,
		OnWB:            onWB != 0,
		OnMail:          onMail != 0,
		OnReply:         onReply != 0,
		OnlyWhenOffline: onlyOffline != 0,
	}, nil
}

// SetPrefs upserts the prefs row for a user.
func (r *NotifyRepo) SetPrefs(ctx context.Context, userID int64, p NotifyPrefs) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	_, err := r.s.db.ExecContext(ctx,
		`INSERT INTO user_notif_prefs
		   (user_id, on_push, on_wb, on_mail, on_reply, only_when_offline)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   on_push=excluded.on_push,
		   on_wb=excluded.on_wb,
		   on_mail=excluded.on_mail,
		   on_reply=excluded.on_reply,
		   only_when_offline=excluded.only_when_offline`,
		userID, b2i(p.OnPush), b2i(p.OnWB), b2i(p.OnMail), b2i(p.OnReply), b2i(p.OnlyWhenOffline),
	)
	return err
}

// ListTargets returns every target row for a user, oldest-first so the
// settings screen renders a stable list.
func (r *NotifyRepo) ListTargets(ctx context.Context, userID int64) ([]*NotifyTarget, error) {
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT id, user_id, label, url, enabled, created_at
		 FROM user_notif_targets WHERE user_id = ? ORDER BY id ASC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NotifyTarget
	for rows.Next() {
		t := &NotifyTarget{}
		var enabled int64
		if err := rows.Scan(&t.ID, &t.UserID, &t.Label, &t.URL, &enabled, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Enabled = enabled != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListEnabledTargets is the dispatcher's hot path: only enabled rows.
func (r *NotifyRepo) ListEnabledTargets(ctx context.Context, userID int64) ([]*NotifyTarget, error) {
	rows, err := r.s.db.QueryContext(ctx,
		`SELECT id, user_id, label, url, enabled, created_at
		 FROM user_notif_targets WHERE user_id = ? AND enabled = 1 ORDER BY id ASC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*NotifyTarget
	for rows.Next() {
		t := &NotifyTarget{}
		var enabled int64
		if err := rows.Scan(&t.ID, &t.UserID, &t.Label, &t.URL, &enabled, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Enabled = enabled != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

// AddTarget validates the URL and inserts a new row. Returns the assigned id.
func (r *NotifyRepo) AddTarget(ctx context.Context, userID int64, label, url string) (int64, error) {
	if err := ValidateNotifyURL(url); err != nil {
		return 0, err
	}
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	res, err := r.s.db.ExecContext(ctx,
		`INSERT INTO user_notif_targets (user_id, label, url) VALUES (?, ?, ?)`,
		userID, strings.TrimSpace(label), strings.TrimSpace(url),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateTarget mutates label/url/enabled in place. URL is validated.
// requesterUserID is checked against the row's user_id so a malicious
// caller can't update someone else's target by guessing the id.
func (r *NotifyRepo) UpdateTarget(ctx context.Context, id, requesterUserID int64, label, url string, enabled bool) error {
	if err := ValidateNotifyURL(url); err != nil {
		return err
	}
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	res, err := r.s.db.ExecContext(ctx,
		`UPDATE user_notif_targets SET label = ?, url = ?, enabled = ?
		 WHERE id = ? AND user_id = ?`,
		strings.TrimSpace(label), strings.TrimSpace(url), b2i(enabled), id, requesterUserID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotifyTargetNotFound
	}
	return nil
}

// SetTargetEnabled is the lightweight toggle path used by the `t` shortcut
// in the settings screen — separate from UpdateTarget to avoid re-asserting
// label/url validation on every keystroke.
func (r *NotifyRepo) SetTargetEnabled(ctx context.Context, id, requesterUserID int64, enabled bool) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	res, err := r.s.db.ExecContext(ctx,
		`UPDATE user_notif_targets SET enabled = ?
		 WHERE id = ? AND user_id = ?`,
		b2i(enabled), id, requesterUserID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotifyTargetNotFound
	}
	return nil
}

// DeleteTarget removes a row, gated on user_id so the caller can only
// delete their own targets.
func (r *NotifyRepo) DeleteTarget(ctx context.Context, id, requesterUserID int64) error {
	r.s.writeMu.Lock()
	defer r.s.writeMu.Unlock()
	res, err := r.s.db.ExecContext(ctx,
		`DELETE FROM user_notif_targets WHERE id = ? AND user_id = ?`,
		id, requesterUserID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotifyTargetNotFound
	}
	return nil
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
