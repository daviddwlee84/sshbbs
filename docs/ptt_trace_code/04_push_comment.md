# 04 · 推文 (push / boo / arrow) — design notes

## pttbbs reference

Early pttbbs stored **only a counter** in `fileheader_t.recommend` (signed
8-bit, capped). Individual push records were not persisted — you couldn't
audit who pushed what or when. Later pttbbs added `commentd` (a daemon) to
record individual pushes alongside the article file, but this remained an
"experimental" path.

The classic UI is single-key: `%` enters push mode, then `1` 推, `2` 噓,
`3` →, type a one-line comment, Enter to send.

## Our model

We store every push as a row in `pushes`, and keep `articles.recommend_score`
as a cached aggregate so the article-list view can render scores cheaply.

```sql
CREATE TABLE pushes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    article_id      INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    user_id         INTEGER NOT NULL REFERENCES users(id),
    user_userid     TEXT    NOT NULL,                          -- denormalized
    kind            TEXT    NOT NULL CHECK (kind IN ('push','boo','arrow')),
    body            TEXT    NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_pushes_article ON pushes(article_id, id);
```

## Atomic insert + score update

`PushRepo.Create` runs both writes inside one `BEGIN`/`COMMIT`, under the
`Store.writeMu` mutex:

```go
INSERT INTO pushes (...) VALUES (...);
UPDATE articles SET recommend_score = recommend_score + ? WHERE id = ?;
```

`+ delta` where `delta = +1 / -1 / 0` for push / boo / arrow. The mutex is
process-level (not row-level) — fine for our scale, and avoids the
`SQLITE_BUSY` retry dance.

## Live broadcast (Step 7)

The article-view model (`internal/tui/screen_article_view.go`) listens for
`PushAddedMsg`. After a successful insert, `screen_article_view.openPush ->
updatePushInput` calls:

```go
broker.SendToAll(senderUserID, PushAddedMsg{ArticleID, UserUserID, Kind, Body})
```

Every other live session receives the message and ignores it unless
`msg.ArticleID == m.article.ID`. Matching viewers re-fetch the push list
and the article (for the updated `recommend_score`) — using DB as the
source of truth means timestamps and ordering are always canonical.

## Hotkeys (single-key, PTT-flavoured)

- `+` → push (推, +1)
- `-` → boo (噓, -1)
- `=` → arrow (→, 0)

Each opens an inline comment input at the bottom of the article view.
Enter sends, Esc cancels.
