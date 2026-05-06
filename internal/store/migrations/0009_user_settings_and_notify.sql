-- User settings (bio) plus a per-user notification system.
-- bio is a free-form profile blurb. Empty default keeps existing rows valid.
-- user_notif_prefs holds the four event toggles (push / wb / mail / reply)
-- plus only_when_offline. Row is upserted lazily; absence ≡ all-default.
-- user_notif_targets stores per-user webhook URLs that the dispatcher
-- POSTs `{title, body}` JSON to. Compatible with caronc/apprise-api's
-- /notify/<key> endpoint and any other simple webhook (Discord, ntfy.sh,
-- Slack, custom). The BBS deliberately does NOT parse apprise:// URLs —
-- that is delegated to whatever webhook target the user points us at.

ALTER TABLE users ADD COLUMN bio TEXT NOT NULL DEFAULT '';

CREATE TABLE user_notif_prefs (
    user_id           INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    on_push           INTEGER NOT NULL DEFAULT 1, -- 推/噓/→ on my article
    on_wb             INTEGER NOT NULL DEFAULT 1, -- 水球
    on_mail           INTEGER NOT NULL DEFAULT 1, -- 站內信
    on_reply          INTEGER NOT NULL DEFAULT 1, -- 文章被回 (Re:) — fired when someone posts a Re: reply targeting one of my articles
    only_when_offline INTEGER NOT NULL DEFAULT 0  -- skip dispatch if any live session exists
);

CREATE TABLE user_notif_targets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label      TEXT NOT NULL DEFAULT '',
    url        TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_notif_targets_user ON user_notif_targets(user_id, enabled);
