-- Initial schema for the SSH BBS.
-- All tables use auto-increment INTEGER PRIMARY KEY plus a unique business
-- key where appropriate. We deliberately do NOT mimic pttbbs's recno-based
-- addressing (a holdover from mmap'd binary .DIR files) — a relational DB
-- removes the recno-tombstone fragility documented in pttbbs.

PRAGMA foreign_keys = ON;

CREATE TABLE users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         TEXT NOT NULL UNIQUE COLLATE NOCASE, -- userec_t.userid
    password_hash   TEXT NOT NULL,                       -- bcrypt; replaces userec_t.passwd (DES)
    nickname        TEXT NOT NULL DEFAULT '',            -- userec_t.nickname
    realname        TEXT NOT NULL DEFAULT '',            -- userec_t.realname
    email           TEXT NOT NULL DEFAULT '',            -- userec_t.email
    num_logins      INTEGER NOT NULL DEFAULT 0,          -- userec_t.numlogindays (loose)
    num_posts       INTEGER NOT NULL DEFAULT 0,          -- userec_t.numposts
    last_login_at   DATETIME,                            -- userec_t.lastlogin
    last_host       TEXT,                                -- userec_t.lasthost
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE boards (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL UNIQUE COLLATE NOCASE, -- boardheader_t.brdname
    title           TEXT NOT NULL,                       -- boardheader_t.title
    description     TEXT NOT NULL DEFAULT '',
    bm              TEXT NOT NULL DEFAULT '',            -- boardheader_t.BM (comma-sep)
    attr            INTEGER NOT NULL DEFAULT 0,          -- boardheader_t.brdattr (P1+)
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE articles (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    board_id        INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    author_id       INTEGER NOT NULL REFERENCES users(id),
    author_userid   TEXT NOT NULL,                       -- denormalized (fileheader_t.owner)
    title           TEXT NOT NULL,                       -- fileheader_t.title
    body            TEXT NOT NULL,
    recommend_score INTEGER NOT NULL DEFAULT 0,          -- cached sum of pushes (+1) - boos (-1)
    filemode        INTEGER NOT NULL DEFAULT 0,          -- fileheader_t.filemode (P1+)
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_articles_board_created ON articles(board_id, created_at DESC);

CREATE TABLE pushes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    article_id      INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    user_userid     TEXT NOT NULL,                       -- denormalized
    kind            TEXT NOT NULL CHECK (kind IN ('push','boo','arrow')), -- 推/噓/→
    body            TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_pushes_article ON pushes(article_id, id);

CREATE TABLE water_balloons (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    from_user_id    INTEGER NOT NULL REFERENCES users(id),
    from_userid     TEXT NOT NULL,                       -- denormalized
    to_user_id      INTEGER NOT NULL REFERENCES users(id),
    body            TEXT NOT NULL,
    delivered_live  INTEGER NOT NULL DEFAULT 0,
    read_at         DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_wb_to_unread ON water_balloons(to_user_id, read_at);
