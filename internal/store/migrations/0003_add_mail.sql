-- 站內信 (mailbox): persistent threaded private messages.
-- Distinct from water_balloons (transient, toast-on-arrival).
-- A reply has parent_id pointing to its predecessor and thread_id pointing
-- to the root message. For root messages, thread_id = id (filled in by the
-- repo's Insert in a follow-up UPDATE inside the same writeMu transaction).

CREATE TABLE mail (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id       INTEGER,                            -- root mail id; back-filled to self for roots
    parent_id       INTEGER REFERENCES mail(id),        -- NULL for thread roots
    from_user_id    INTEGER NOT NULL REFERENCES users(id),
    from_userid     TEXT NOT NULL,                      -- denormalized author handle
    to_user_id      INTEGER NOT NULL REFERENCES users(id),
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    read_at         DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_mail_to_unread ON mail(to_user_id, read_at);
CREATE INDEX idx_mail_thread ON mail(thread_id, id);
