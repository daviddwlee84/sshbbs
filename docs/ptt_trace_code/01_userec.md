# 01 · `userec_t` → `users` table

## pttbbs reference

`include/pttstruct.h` defines `userec_t`, ~500 bytes per user, persisted as a
fixed-record array in `.PASSWDS`. Lookups are by hash on `userid`. The struct
includes account credentials, vanity fields (real name, nickname, email),
counters (logins, posts, money), game stats (five-in-a-row, chess, go),
last-login metadata, and a flags bitmap.

```c
typedef struct userec_t {
    char    userid[IDLEN+1];     // 13
    char    realname[20];
    char    nickname[24];
    char    passwd[14];          // crypt(3) DES
    int32_t uflag, userlevel;
    uint16_t numlogindays, numposts;
    char    lasthost[16];
    int32_t lastlogin;
    // ... ~30 more fields
} userec_t;
```

## Our mapping (`internal/store/users.go` + `migrations/0001_init.sql`)

```sql
CREATE TABLE users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         TEXT NOT NULL UNIQUE COLLATE NOCASE, -- userec_t.userid
    password_hash   TEXT NOT NULL,                       -- bcrypt cost 12 (was DES)
    nickname        TEXT NOT NULL DEFAULT '',
    realname        TEXT NOT NULL DEFAULT '',
    email           TEXT NOT NULL DEFAULT '',
    num_logins      INTEGER NOT NULL DEFAULT 0,          -- ~userec_t.numlogindays
    num_posts       INTEGER NOT NULL DEFAULT 0,
    last_login_at   DATETIME,
    last_host       TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

## What changed and why

- **`id` (auto-inc) is the canonical key**, not `user_id`. PTT uses the
  `user_id` string everywhere because its hash table indexes on it; we get
  fast lookups via `UNIQUE COLLATE NOCASE` so the int id is purely internal.
- **No `userlevel` / `uflag` bitmaps yet.** PTT's permission model is a
  bag-of-bits accreted over 20 years. Adding it on demand (P1+) is cheaper
  than reverse-engineering the existing bits.
- **No money / game counters.** Out of scope for v1.
- **`COLLATE NOCASE`** so `ALICE`, `Alice`, `alice` collide on UNIQUE and
  resolve to the same login — matches pttbbs `strcasecmp` lookups.
- **`bcrypt` not `crypt(3)`.** `crypt(3)` DES truncates to 8 chars and is
  trivially brute-forced. `auth.bcryptCost = 12`.

## Reserved username

`new` is reserved (`auth.ReservedUsernameNew`). The auth middleware
(`internal/server/auth_middleware.go`) intercepts SSH user `new` and routes
to the in-TUI register form. `auth.Register` rejects `new` to keep the
sentinel from ever colliding with a real account.
