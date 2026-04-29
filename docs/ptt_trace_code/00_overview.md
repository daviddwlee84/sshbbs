# 00 · Overview — what we mimic from pttbbs and what we drop

These notes trace pttbbs (https://github.com/ptt/pttbbs) concepts to what we
actually implement in this repo. They exist so future readers know which design
decisions are intentional simplifications vs. things still to do.

## What we mimic

| pttbbs concept                | Where in our repo                                 |
|-------------------------------|---------------------------------------------------|
| `userec_t` (user record)      | `users` table + `internal/store/users.go`         |
| `boardheader_t` (board)       | `boards` table + `internal/store/boards.go`       |
| `fileheader_t` (article hdr)  | `articles` table + `internal/store/articles.go`   |
| `.DIR` per-board index        | `articles.board_id` + `idx_articles_board_created`|
| 推文 (push counter / log)      | `pushes` table + `articles.recommend_score` cache |
| 水球 (water balloon, live IPC) | `internal/chat/broker.go` (in-memory)             |
| `userinfo_t` UTMP table       | `Broker.sessions map[int64][]*Session` (in-mem)   |
| `mbbsd` per-session process   | one bubbletea program per SSH session             |

## What we drop on purpose (v1)

- **Binary on-disk shared memory** (`SHM`, `userinfo_t` arrays). pttbbs uses
  fixed-size mmap'd structs; we use SQLite + Go maps. Adds 5-10× the RAM but
  removes ~80% of pttbbs's complexity (alignment bugs, sizeof drift, daemon
  coordination).
- **`recno`-based article addressing.** `.DIR` indexes articles by record
  number, which leaves tombstones on delete and breaks bookmarks if you renumber.
  We use `articles.id` (auto-increment) and never reuse IDs.
- **Big5 encoding.** UTF-8 throughout. `go-runewidth` handles double-width
  CJK. No transcoding layer.
- **DES `crypt(3)` passwords.** bcrypt (cost 12) instead.
- **The `mbbsd` / `logind` / `boardd` / `commentd` / `utmpd` daemon split.**
  One Go process owns everything. PTT split because forking C processes
  per-session was the only way to scale on 1990s hardware; goroutines remove
  the constraint.
- **Games, BBSMovie, voting, mail-to-external-email, post-tracking AID system,
  archive/digest.** All deferred to P2 or out of scope.

## What we kept simple but recognisable

- **PTT-style login: SSH user is the login name.** `ssh alice@host` lands
  alice on her account (after SSH password auth verifies against bcrypt).
  `ssh new@host` is the in-TUI register sentinel.
- **Boards as flat list.** No hierarchy / categories yet (P1).
- **推 / 噓 / →** with single-key input (`+` / `-` / `=`) and live broadcast
  to other viewers of the same article.
- **One-line water balloons** with offline persistence and on-reconnect replay.

## Files in this directory

- `00_overview.md` — this file
- `01_userec.md` — user record mapping
- `02_boardheader.md` — board record mapping
- `03_fileheader_dir.md` — article + DIR mapping
- `04_push_comment.md` — 推文 design
- `05_water_balloon.md` — 水球 design
- `06_session_userinfo.md` — online presence design
