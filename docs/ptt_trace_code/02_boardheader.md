# 02 · `boardheader_t` → `boards` table

## pttbbs reference

`include/pttstruct.h` defines `boardheader_t`, ~256 bytes, stored as a
fixed-record array in `.BRD`. Cached in shared memory at boot for fast
listing / lookup.

```c
typedef struct boardheader_t {
    char     brdname[IDLEN+1];   // 13 — the board's slug
    char     title[BTLEN+1];     // 49
    char     BM[IDLEN*3+3];      // up to 3 Board Masters
    uint32_t brdattr;            // bitmask: anonymous? voting? hidden? digest?
    uint8_t  level;              // min user level to read
    int32_t  bupdate;
    uint8_t  postexpire;
    uint8_t  bvote;
    int32_t  bgroupid;           // category / parent
    int32_t  nuser;              // live count cache
    // ...
} boardheader_t;
```

## Our mapping

```sql
CREATE TABLE boards (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL UNIQUE COLLATE NOCASE, -- brdname
    title           TEXT NOT NULL,                       -- title
    description     TEXT NOT NULL DEFAULT '',
    bm              TEXT NOT NULL DEFAULT '',            -- comma-separated user_ids
    attr            INTEGER NOT NULL DEFAULT 0,          -- brdattr (P1+)
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

## What changed and why

- **`bm` as a comma-separated string** instead of a fixed three-slot array.
  Trivially parseable and avoids a join table for v1.
- **`attr` bitmask reserved but unused.** When P2 adds permissions / hidden
  boards, we'll define the bit constants in code (not in the DB schema).
- **No `nuser` live count.** PTT keeps it in shared memory because computing
  it on demand was expensive; we'll compute it from the broker's online list
  if we need it (P1).
- **No `bgroupid` hierarchy.** Flat list for now. P1 may add `parent_id` for
  categories.
- **No `postexpire`.** Articles never auto-delete in v1.

## Default seed (`migrations/0002_seed_boards.sql` + `BoardRepo.SeedDefaults`)

Three boards: `Welcome`, `Test`, `ChitChat`. `INSERT OR IGNORE` so it's
idempotent. `BoardRepo.SeedDefaults()` runs every startup as belt-and-braces
in case the migration was rolled back manually.
