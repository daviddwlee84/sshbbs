# PTT-style SSH BBS — Implementation Plan

## Context

The repo at `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs` is empty. We're building a simplified PTT-like (批踢踢) BBS served over SSH, where users can register an account, log in, browse a few default boards, read & post articles, push (推/噓) on articles, and send "water balloons" (水球 — short live messages) to other online users.

**Why this stack**:
- **charmbracelet/wish** handles SSH transport, auth callbacks, and bridging to bubbletea TUIs cleanly. Avoids reimplementing SSH plumbing.
- **bubbletea + lipgloss + bubbles** are the standard Charm stack for terminal UIs and the wish ecosystem expects them.
- **modernc.org/sqlite** (pure-Go SQLite) gives zero-CGO builds, easy cross-compilation, and is more than sufficient for a single-node BBS.
- We **deliberately do not** mimic PTT's binary `.PASSWDS` / `.DIR` / shared-memory layout (the source of most pttbbs's complexity); a relational DB does the same job with a fraction of the code and none of the recno/tombstone fragility.

User choices confirmed before planning:
1. **Login UX**: SSH username = DB login name. Special user `new` always passes SSH auth and routes to the in-TUI register screen.
2. **推文 (push) is P0**, not deferred.
3. **Default boards**: `Welcome`, `Test`, `ChitChat`.

## Repo skeleton

```
2026-04-29-ssh-bbs/
├── go.mod                          # module github.com/daviddwlee84/sshbbs
├── go.sum
├── Makefile                        # run, build, test, lint, hostkey, db-reset
├── .gitignore                      # /sshbbs, /data/, /.ssh/host_*, *.db, *.db-wal
├── README.md                       # quickstart
├── cmd/
│   └── sshbbs/
│       └── main.go                 # flag parsing, wires Store + Broker + Server, signal handling
├── internal/
│   ├── server/
│   │   ├── server.go               # wish.NewServer + middleware stack
│   │   └── auth_middleware.go      # SSH password-auth callback, ctx.SetValue("user_id", ...)
│   ├── auth/
│   │   ├── auth.go                 # Register(), VerifyLogin(), bcrypt helpers, validation
│   │   └── auth_test.go
│   ├── store/
│   │   ├── store.go                # *sql.DB wrapper, Open(), Close(), tx helper
│   │   ├── migrate.go              # embed migrations/*.sql, schema_migrations table
│   │   ├── users.go                # UserRepo
│   │   ├── boards.go               # BoardRepo (+ SeedDefaults)
│   │   ├── articles.go             # ArticleRepo
│   │   ├── pushes.go               # PushRepo
│   │   └── waterballoons.go        # WBRepo (Insert / ListUnreadFor / MarkRead)
│   ├── chat/
│   │   ├── broker.go               # Broker: Register/Unregister session, Send, OnlineList
│   │   └── presence.go             # OnlineSet helpers
│   ├── tui/
│   │   ├── root.go                 # tea.Model state machine; routes to per-screen sub-models
│   │   ├── styles.go               # lipgloss styles
│   │   ├── runewidth.go            # CJK-safe truncate/pad helpers
│   │   ├── messages.go             # tea.Msg types: WBIncomingMsg, NavigateMsg, ErrorMsg, PushAddedMsg
│   │   ├── screen_welcome.go
│   │   ├── screen_register.go      # only used when SSH user == "new"
│   │   ├── screen_main_menu.go
│   │   ├── screen_board_list.go
│   │   ├── screen_board_view.go
│   │   ├── screen_article_view.go  # body + push list + add-push input
│   │   ├── screen_post_compose.go
│   │   └── screen_wb.go            # inbox + compose
│   └── version/version.go
├── migrations/
│   ├── 0001_init.sql
│   └── 0002_seed_boards.sql
├── docs/
│   └── ptt_trace_code/
│       ├── 00_overview.md
│       ├── 01_userec.md
│       ├── 02_boardheader.md
│       ├── 03_fileheader_dir.md
│       ├── 04_push_comment.md
│       ├── 05_water_balloon.md
│       └── 06_session_userinfo.md
├── scripts/
│   └── gen-hostkey.sh              # ssh-keygen -t ed25519 -f .ssh/host_ed25519
├── .ssh/.gitkeep                   # host key dropped here at runtime, not committed
└── data/.gitkeep                   # bbs.db lives here at runtime, not committed
```

## Database schema (SQLite — `migrations/0001_init.sql`)

We use auto-increment `id` everywhere with a unique business key (`user_id`, `name`) where it makes sense. PTT's recno-based addressing is a holdover from mmap'd binary files; with a real DB it has no upside and creates well-known fragility (the recno-tombstone gotcha noted in pttbbs research).

```sql
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE users (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         TEXT NOT NULL UNIQUE COLLATE NOCASE, -- userec_t.userid
  password_hash   TEXT NOT NULL,                       -- bcrypt; replaces userec_t.passwd (DES)
  nickname        TEXT NOT NULL DEFAULT '',            -- userec_t.nickname
  realname        TEXT NOT NULL DEFAULT '',            -- userec_t.realname
  email           TEXT NOT NULL DEFAULT '',            -- userec_t.email
  num_logins      INTEGER NOT NULL DEFAULT 0,          -- userec_t.numlogindays (loose mapping)
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
  author_userid   TEXT NOT NULL,                       -- denormalized for fast list (fileheader_t.owner)
  title           TEXT NOT NULL,                       -- fileheader_t.title
  body            TEXT NOT NULL,
  recommend_score INTEGER NOT NULL DEFAULT 0,          -- cached: pushes(+1) - boos(-1)
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

CREATE TABLE schema_migrations (
  version         INTEGER PRIMARY KEY,
  applied_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

`migrations/0002_seed_boards.sql` does `INSERT OR IGNORE` for `Welcome`, `Test`, `ChitChat`. `BoardRepo.SeedDefaults()` runs the same on every startup as a belt-and-braces.

## Component breakdown

**1. SSH server bootstrap** — `internal/server/server.go`
- `func New(cfg Config, store *store.Store, broker *chat.Broker) (*ssh.Server, error)`
- Builds with `wish.NewServer(wish.WithAddress(cfg.Addr), wish.WithHostKeyPath(".ssh/host_ed25519"), wish.WithPasswordAuth(authmw.PasswordAuth(store)), wish.WithMiddleware(bm.Middleware(handler), activeterm.Middleware(), lm.Middleware()))`. Middleware is LIFO → logging wraps activeterm wraps bubbletea.
- `handler` returns `func(sess ssh.Session) (tea.Model, []tea.ProgramOption)`. It pulls `user_id` (or the sentinel "new") from `sess.Context()`, calls `tui.NewRoot(...)`, and returns `bubbletea.MakeOptions(sess)` plus `tea.WithAltScreen()`.

**2. Auth** — `internal/auth/auth.go` and `internal/server/auth_middleware.go`
- `auth.Register(ctx, store, userID, password, nickname, email) (*store.User, error)` — validates `^[a-zA-Z][a-zA-Z0-9_]{2,11}$`, password length ≥ 6, bcrypt-hashes (cost 12), inserts; rejects reserved name `new`.
- `auth.VerifyLogin(ctx, store, userID, password) (*store.User, error)` — fetch by `user_id`, `bcrypt.CompareHashAndPassword`, on success update `last_login_at` / `last_host` / `num_logins++`.
- `authmw.PasswordAuth(store)` — if `ctx.User() == "new"`, accept any password and stash a "register mode" flag in the context. Otherwise call `VerifyLogin`; on success `ctx.SetValue("user_id", user.ID)` and return true.

**3. Storage layer** — `internal/store/`
- `store.Open(path string) (*Store, error)` opens `modernc.org/sqlite` with DSN `file:<path>?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)`, then runs `migrate.Apply(db)`.
- `migrate.Apply` reads `embed.FS` of `migrations/*.sql`, sorts by filename, applies any version not in `schema_migrations`.
- Repos are stateless methods on `*Store`: `s.Users().GetByUserID(...)`, `s.Articles().Create(...)`, etc.
- Single `*sql.DB` shared. A single process-level `writeMu sync.Mutex` on `Store` is held during writes that touch multiple tables (e.g. push insert + recommend_score update); WAL handles concurrent reads.

**4. Chat broker** — `internal/chat/broker.go`
- `type Session struct { UserID int64; UserIDStr string; Program *tea.Program }`
- `Broker.Register(s *Session)` and `Broker.Unregister(userID int64, program *tea.Program)` — uses program pointer to handle the same user with multiple sessions ("雙開"), removing only the matching one.
- `Broker.Send(toUID int64, m tea.Msg) (delivered bool)` — looks up sessions, calls `program.Send(m)` on each. Returns false if nobody online.
- `Broker.SendToBoard(boardID int64, articleID int64, m tea.Msg)` — used for live push broadcast (P0 included since 推文 is in MUST HAVE).
- `Broker.OnlineList() []OnlineUser`
- All maps protected by `sync.RWMutex`. Pattern lifted from `wish/examples/multichat`.

**5. TUI root + screens** — `internal/tui/`
- `type Root struct { state Screen; user *store.User; isRegisterMode bool; store *store.Store; broker *chat.Broker; sub tea.Model; width, height int; toast string; toastUntil time.Time }`
- `Init() tea.Cmd` — if not register mode, drains unread WBs from `WBRepo.ListUnreadFor(user.id)` and emits each as `WBIncomingMsg`.
- `Update(msg)` handles `tea.WindowSizeMsg` (forwards to sub), global keys (`Ctrl+U` open WB inbox, `Ctrl+C` quit), `WBIncomingMsg` (set toast in footer for 3s + persist+mark unread tracking), `NavigateMsg{To: Screen}` (swap `sub`), `PushAddedMsg` (forward to article view if active).
- Each `screen_*.go` exports `newXxxModel(deps) tea.Model` and emits `NavigateMsg` upward when done.
- `screen_register` on submit calls `auth.Register`, then prints "登入成功，請重新連線使用 ssh <name>@..." and quits.
- `screen_post_compose` on submit calls `Articles().Create`.
- `screen_article_view` lists pushes; pressing `r` opens an inline push input (kind = push/boo/arrow chosen by hotkey: `+` `-` `=`); on submit, inserts a push, recomputes `recommend_score`, broadcasts `PushAddedMsg` via `broker.SendToBoard`.
- `screen_wb` inbox lists all WBs (unread first); compose targets a userid (autocompleted from online list); on send, calls `broker.Send` first then persists.

**6. Default-board seeding** — `migrations/0002_seed_boards.sql` + `BoardRepo.SeedDefaults()` at startup.

## Priority-tagged TODO list

### MUST HAVE (P0 — MVP demo)
Goal: SSH in → register → log in → browse boards → read+post articles → 推文 → send a water balloon to another online user.

- [ ] **P0-1** Repo skeleton, `go.mod`, `.gitignore`, `Makefile` (`run`, `build`, `hostkey`, `db-reset`).
- [ ] **P0-2** `scripts/gen-hostkey.sh`; `.ssh/.gitkeep`; key itself ignored.
- [ ] **P0-3** `internal/store` open + embedded migrations + `0001_init.sql` + `0002_seed_boards.sql`.
- [ ] **P0-4** Repos: `UserRepo`, `BoardRepo`, `ArticleRepo`, `PushRepo`, `WBRepo` — only the methods actually used by P0 screens.
- [ ] **P0-5** `internal/auth` Register / VerifyLogin with bcrypt; reserved name `new`.
- [ ] **P0-6** `internal/server` wish bootstrap + password-auth middleware. SSH user `new` is the registration sentinel.
- [ ] **P0-7** `internal/chat/broker.go` (Register / Unregister / Send / SendToBoard / OnlineList); session lifecycle wired to bubbletea handler with `defer Unregister`.
- [ ] **P0-8** TUI screens: welcome, register, main menu, board list, board view, article view (with **inline 推文 add + live update**), post compose, water balloon inbox + compose.
- [ ] **P0-9** `internal/tui/runewidth.go` CJK-safe pad/truncate helpers; use them in every list rendering.
- [ ] **P0-10** `cmd/sshbbs/main.go` flag-driven (`-addr=:2222`, `-db=data/bbs.db`, `-hostkey=.ssh/host_ed25519`); `signal.NotifyContext` for SIGINT/SIGTERM; graceful shutdown drains broker (sends `tea.Quit` to each session), waits up to 3s, then closes the DB.
- [ ] **P0-11** `docs/ptt_trace_code/` notes (six files; each ~150–300 words, mapping pttbbs concepts to our tables/components).
- [ ] **P0-12** `README.md` quickstart: `make hostkey && make run`, then `ssh new@localhost -p 2222`.

### SHOULD HAVE (P1 — PTT-feel polish)
- [ ] P1-1 Live broadcast of *new articles* to viewers of the same board list.
- [ ] P1-2 Online-user list screen.
- [ ] P1-3 PTT-style hotkeys (`hjkl`, `[` `]`, `r` reply, `Q` quit-to-menu) in addition to arrow keys.
- [ ] P1-4 Article pagination + jump-to-end / jump-to-top.
- [ ] P1-5 Mailbox (站內信) — distinct from water balloon, persistent threaded.
- [ ] P1-6 Per-board BM list shown in header.
- [ ] P1-7 Seed-data CLI: `sshbbs seed --articles 50` for demo content.
- [ ] P1-8 Structured logging via `log/slog` with per-session correlation IDs.
- [ ] P1-9 Soft delete for articles (`deleted_at`).
- [ ] P1-10 Pager-settings: accept-from-friends-only, do-not-disturb.

### NICE TO HAVE (P2 — defer or maybe never)
- [ ] P2-1 Public-key auth alongside password.
- [ ] P2-2 Board permissions / hidden boards via `boards.attr`.
- [ ] P2-3 SQLite FTS5 search across articles.
- [ ] P2-4 Friends / blocklist tables.
- [ ] P2-5 ANSI color art splash screen.
- [ ] P2-6 Article import from real PTT `.DIR` dumps (use `Ptt-official-app/go-bbs` library).
- [ ] P2-7 Web read-only mirror.
- [ ] P2-8 Prometheus metrics endpoint.
- [ ] P2-9 Rate limiting on register / post.
- [ ] P2-10 Migrate to `sqlc` for type-safe queries.
- [ ] P2-11 Games (五子棋 etc.) — explicitly out of scope.
- [ ] P2-12 BBSMovie ANSI animations.

## Implementation sequencing (P0 only)

Each step ends with a runnable smoke test. Build incrementally; never commit a broken intermediate state.

1. **Hello-world wish.** P0-1, P0-2, P0-6 with auth callback that returns `true` for any password, and a stub bubbletea handler that prints `hello, {{ ctx.User }}`. → `make hostkey && make run`, then `ssh anything@localhost -p 2222` shows the greeting; `Ctrl+C` exits cleanly.

2. **Storage + migrations.** P0-3. → `make run` creates `data/bbs.db`; `sqlite3 data/bbs.db ".tables"` lists all six tables; `SELECT name FROM boards` shows three rows.

3. **Register flow.** P0-4 (`UserRepo` only), P0-5, register screen (P0-8 partial), and SSH-user-`new` routing in P0-6. → `ssh new@localhost -p 2222`, fill form, see "registered" screen; `sqlite3 ... "SELECT user_id FROM users"` shows the row.

4. **Real login + main menu.** Wire `authmw.PasswordAuth` to `VerifyLogin`; build main menu screen. → `ssh alice@localhost -p 2222` with the password from step 3 lands on main menu; wrong password rejected by SSH itself (3 tries then disconnect).

5. **Boards + articles read.** Remaining repo methods, board list / board view / article view screens. → navigate menu → boards → board → article; CJK aligns correctly.

6. **Post article.** Post compose screen + `ArticleRepo.Create`. → post a Chinese title+body, return to board view, see new entry at top, re-open it, body matches.

7. **推文.** Inline push input in article view, `PushRepo.Create`, recompute and update `recommend_score` (under `writeMu`), broadcast `PushAddedMsg` via `broker.SendToBoard`. → from session A push an article; session B already viewing the same article sees the new push appear within ~100ms.

8. **Chat broker + presence.** P0-7 fully. Hook Register on session start, Unregister on session end. → start two sessions, server log shows two Registers; quit one, log shows one Unregister.

9. **Water balloon round-trip.** WB inbox + compose screens (final P0-8 piece) + `WBRepo`. Live delivery via `broker.Send`; persist always with `delivered_live` flag. On root model `Init`, drain unread WBs from DB and emit as `WBIncomingMsg`. → T1=alice, T2=bob: from T1 send WB to bob → T2 sees toast within ~100ms; T2 quits, T1 sends another → T2 reconnects → toast appears at startup.

10. **Documentation.** Write the six `docs/ptt_trace_code/*.md` notes (P0-11) and `README.md` (P0-12).

11. **Graceful shutdown polish.** Implement the signal handling + broker-drain in `main.go` (P0-10 final). → start server, connect, then `Ctrl+C` server: connected sessions get a goodbye toast and disconnect cleanly; `PRAGMA integrity_check` on restart passes.

## Critical files to be modified / created

- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/cmd/sshbbs/main.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/server/server.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/server/auth_middleware.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/auth/auth.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/store/*.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/chat/broker.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/root.go` and `internal/tui/screen_*.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/migrations/0001_init.sql`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/migrations/0002_seed_boards.sql`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/docs/ptt_trace_code/*.md`

## Reference implementations to borrow from

- `github.com/charmbracelet/wish/examples/multichat` — the `[]*tea.Program` + `program.Send(msg)` broadcast pattern is exactly what `internal/chat/broker.go` will implement.
- `github.com/charmbracelet/wish/examples/bubbletea` — minimal SSH-to-bubbletea handler skeleton.
- `github.com/charmbracelet/wish/examples/multi-auth` — password-auth callback example (we only need the password part).
- `github.com/charmbracelet/soft-serve` — production-grade wish app layout for `cmd/` + `internal/server/` + `internal/store/` conventions.
- `github.com/mattn/go-runewidth` — `StringWidth`, `Truncate`, `FillRight` for every CJK-aware render.

## Verification plan (end-to-end)

**Setup**
1. `make hostkey` (creates `.ssh/host_ed25519`).
2. `make run` (boots server on `:2222`, creates `data/bbs.db` with seed boards).

**Register**
3. `ssh new@localhost -p 2222` → register screen → fill `alice / pw123456 / 愛麗絲`. Server should print "alice registered".
4. Verify: `sqlite3 data/bbs.db "SELECT user_id, nickname FROM users"` shows `alice|愛麗絲`.
5. Repeat to register `bob`.

**Login + read**
6. `ssh alice@localhost -p 2222` with `pw123456` → main menu.
7. Navigate to board list → `ChitChat` → empty list.

**Post + 推文 (push) live update**
8. From alice: open `Test` → press `p` → write a post in Traditional Chinese (e.g. title `測試發文`, body `你好`) → submit → return to board view, see it at top.
9. Open the article. CJK should align in the header.
10. Open a second terminal: `ssh bob@localhost -p 2222`, navigate to the same article in `Test`.
11. From bob: press `r`, type `+` (push) and a comment `推 第一個!` → submit. Bob's screen shows the push appended.
12. Alice's screen should show the new push within ~100ms (live broadcast via `SendToBoard`).

**Water balloon**
13. From alice: `Ctrl+U` → compose → recipient `bob`, body `晚上吃啥` → send.
14. Bob's screen shows a toast in the footer within ~100ms.
15. Bob disconnects (`Ctrl+C`).
16. From alice: send another WB to bob.
17. Bob reconnects: toast for the unread WB appears at startup. Open inbox → both messages listed.

**Resize + crash safety**
18. Drag terminal width during board view; layout reflows without garbling CJK.
19. `kill -9` the server. Restart. `sqlite3 data/bbs.db "PRAGMA integrity_check"` returns `ok`.

**Platform gotchas (note in README)**
- `make hostkey` rotated → `Host key verification failed` from clients. Fix: `ssh-keygen -R '[localhost]:2222'`.
- macOS Terminal must use UTF-8 (`echo $LANG` should match `*.UTF-8`); otherwise CJK is mojibake regardless of server-side correctness.

## Risks and open questions

1. **`modernc.org/sqlite` write contention.** Pure-Go SQLite is correct but slower than CGO under heavy writes. With WAL + `busy_timeout=5000` + a process-level `writeMu`, we comfortably handle dozens of sessions. If we ever observe `SQLITE_BUSY` in logs, swap one driver line to `mattn/go-sqlite3` — same SQL.
2. **CJK width in lipgloss borders.** `lipgloss` uses `go-runewidth` internally but its `Width()` calls can still mis-align if a glyph straddles a column boundary inside a styled box. Always pre-pad/truncate via our `tui/runewidth.go` helpers before handing strings to lipgloss; never trust auto-padding for table columns.
3. **Same user, multiple sessions.** Allowed (PTT calls this "雙開"); broker keeps a slice of sessions per user. Water balloons broadcast to all of them. Acceptable for MVP.
4. **In-flight push broadcast race.** A user could be moving between articles when a `PushAddedMsg` arrives for the article they just left. The article-view model checks `msg.ArticleID == m.articleID` and ignores stale events — cheap and safe.
5. **Graceful shutdown.** `wish.Server.Shutdown(ctx)` closes the listener but does not Quit live tea programs. We must iterate broker sessions and `program.Send(tea.Quit())` ourselves, wait briefly, then close the DB. Skipping this leaves WAL un-checkpointed and clients hung.
6. **Reserved username `new`.** Hardcoded in two places (auth middleware + register validator). Add a `const ReservedUsernameNew = "new"` in `internal/auth` so neither place drifts.
