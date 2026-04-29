# 03 · `fileheader_t` + `.DIR` → `articles` table

## pttbbs reference

Articles in pttbbs are stored as plain files inside `boards/<brdname>/`.
The per-board file `.DIR` is a packed array of `fileheader_t` records — 128
bytes each — that act as the index for ordering, listing, and metadata.

```c
typedef struct fileheader_t {
    char     filename[FNLEN];        // M.123456789.A
    char     recommend;              // 推/噓 net counter (-128..+127, capped)
    char     owner[IDLEN+2];
    char     date[6];                // "MM/DD"
    char     title[TTLEN+1];         // 65
    uint8_t  filemode;               // bitmask: digest / solved / voted ...
    int32_t  modified;
    union { ... };
} fileheader_t;
```

Article body lives in `boards/<brdname>/<filename>`; the `.DIR` entry is the
seek index. Article addressing is by **recno** — the integer position inside
`.DIR`. Deleting an article either renumbers (breaks bookmarks) or leaves
tombstones (PTT's choice).

## Our mapping

```sql
CREATE TABLE articles (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    board_id        INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    author_id       INTEGER NOT NULL REFERENCES users(id),
    author_userid   TEXT    NOT NULL,                    -- denormalized for list render
    title           TEXT    NOT NULL,
    body            TEXT    NOT NULL,                    -- replaces the on-disk file
    recommend_score INTEGER NOT NULL DEFAULT 0,          -- cached pushes(+1) + boos(-1)
    filemode        INTEGER NOT NULL DEFAULT 0,          -- reserved (P1+)
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_articles_board_created ON articles(board_id, created_at DESC);
```

## What changed and why

- **No filesystem layout.** The body lives in the `body` TEXT column. SQLite
  stores it inline up to ~1KB then spills to overflow pages — fine for typical
  posts (< 8KB).
- **No recno.** Auto-increment `id` is opaque and never re-used; bookmarks
  stay valid forever even if rows are soft-deleted later.
- **`recommend_score` as a cached integer**, recomputed transactionally with
  every push insert (`PushRepo.Create` runs the update in the same tx, all
  under `Store.writeMu`). Lets the list view render scores without a join.
- **`author_userid` denormalized** so the list view doesn't need a users join
  for the common case.
- **No anonymous posts** (PTT `BRD_ANONYMOUS`). Reserved for P1+.

## Listing strategy

`ArticleRepo.ListByBoard(boardID, limit)` returns newest-first. The
`idx_articles_board_created` index makes it O(log n + limit). PTT scans
`.DIR` linearly — fine for a binary mmap'd file but we get the same effective
behaviour with the index for free.
