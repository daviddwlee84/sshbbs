# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A simplified PTT-style BBS (批踢踢 clone) served over SSH. Built with
`charmbracelet/wish` (SSH framework) + `bubbletea` (TUI), persisted in
SQLite via the pure-Go `modernc.org/sqlite` driver. **Vim-style navigation
is a deliberate product decision** — `h`/`j`/`k`/`l` work alongside arrow
keys and Esc/Enter on every list/menu screen. Form screens (register,
post compose, water-balloon compose) intentionally do *not* bind h/l so
they remain usable as text-edit keys.

For the why-we-do-it-this-way notes mapping back to pttbbs internals, see
`docs/ptt_trace_code/`. For the testing strategy, see `docs/testing.md`.

## Common commands

```bash
make hostkey            # generate .ssh/host_ed25519 (run once before first run)
make run                # boots :2222, creates data/bbs.db with seed boards
make build              # produces ./sshbbs
make test               # plain go test
make test-race          # what we treat as the CI standard
make cover              # per-package coverage
make db-reset           # rm data/bbs.db*

# focused testing
go test -run TestPushes_ConcurrentScoreAtomicity -race -v ./internal/store/...
go test -race ./internal/chat/...

# manual two-user smoke
ssh new@localhost -p 2222     # password ignored — fills the in-TUI register form
ssh alice@localhost -p 2222   # use the password set during register
```

If `make hostkey` is re-run, clients see `Host key verification failed`.
Fix: `ssh-keygen -R '[localhost]:2222'`.

## Architecture — the non-obvious parts

### SSH-user-as-login, with `new` as the register sentinel
`auth.ReservedUsernameNew = "new"`. The password-auth callback in
`internal/server/auth_middleware.go` accepts any password for SSH user
`new` and stashes a "register mode" flag in `ssh.Context`; the TUI
handler reads it and routes straight to the register screen. For any
other SSH username, `auth.VerifyLogin` runs bcrypt against the DB and
stashes the `user_id` (int64) in the context for `makeProgramHandler`
to load.

This means `auth.Register` rejects the literal username `new` — that
constant is referenced in two places (auth + middleware) and they must
not drift. `auth.go` defines it once.

### Wish uses `MiddlewareWithProgramHandler`, not `Middleware`
We pick the *ProgramHandler* variant deliberately
(`internal/server/server.go`) because we need the `*tea.Program` pointer
to register the session with `chat.Broker` and to `program.Send(tea.Quit())`
during graceful shutdown. The simpler `Middleware(handler)` form constructs
the program internally and hides the pointer — you can't broadcast
into it. Don't switch back.

### The chat broker takes a `Sender` interface, not `*tea.Program`
`chat.Sender` is `interface { Send(tea.Msg) }`. `*tea.Program` satisfies
it naturally. The interface exists purely so unit tests can substitute
a recording fake (`fakeSender` in `broker_test.go`) without spinning up
a real bubbletea program. If you find yourself wanting to mock the
broker, you probably want to add a method here, not refactor.

### Live broadcasts are "send to everyone, filter on receive"
For 推文 (push) updates, `screen_article_view.updatePushInput` calls
`broker.SendToAll(senderUID, PushAddedMsg{ArticleID, ...})`. Every
connected session receives the message; each article-view model
ignores it unless `msg.ArticleID == m.article.ID`. This is simpler than
tracking who's viewing what and good enough for our scale. When the
filter matches, we **re-fetch from DB** rather than trust the broker
payload — that way timestamps and `recommend_score` are always canonical.

Water balloons (`broker.Send(toUID, ...)`) target a single user; the
multi-session "雙開" case fans out to every live `Session` of that user.

### `Store.writeMu` is process-level, not row-level
`internal/store/store.go` exposes a single `sync.Mutex` that's grabbed
inside every multi-statement repo method (e.g. `PushRepo.Create` runs
`INSERT INTO pushes` and `UPDATE articles SET recommend_score` in one
transaction under the mutex). This keeps the cached score atomic from
the application's perspective and avoids `SQLITE_BUSY` flapping with
the pure-Go driver. Don't use `sql.Tx` ad-hoc inside repo methods
without also taking `writeMu`. The `pushes` test
(`TestPushes_ConcurrentScoreAtomicity`, 50 concurrent inserts) is the
canary — if it ever fails under `-race`, the mutex discipline broke.

### Migrations live under the store package, not the repo root
`internal/store/migrations/*.sql` is embedded with `go:embed
migrations/*.sql` from `internal/store/migrate.go`. `go:embed` can't
ascend with `..`, so the SQL files live next to the Go code that loads
them. Filename format `NNNN_name.sql`; `migrate.apply()` parses the
prefix as the version and tracks applied versions in the
`schema_migrations` table.

### Data deliberately diverges from pttbbs in three places
1. **No recno-based article addressing** — `articles.id` (auto-increment)
   is opaque and never re-used. Bookmarks stay valid forever.
2. **bcrypt cost-12, not `crypt(3)` DES** — `auth.bcryptCost`.
3. **No binary shared memory / `userinfo_t` arrays** — presence is a
   `sync.RWMutex`-guarded `map[int64][]*Session` in `chat.Broker`.

The full mapping (userec_t / boardheader_t / fileheader_t / `.DIR` /
推文 / 水球 / userinfo_t) is in `docs/ptt_trace_code/01..06_*.md`.

## Testing layers (see `docs/testing.md` for the full doc)

- **Layer 1**: plain `go test` for `auth` / `store` / `chat`. Most coverage lives here.
- **Layer 2**: direct `tea.Model.Update()` tests in `internal/tui/`. Use
  `package tui` (internal), not `tui_test`, to reach unexported screen
  constructors. Keybinding-parity tests are table-driven over
  `[]string{"esc","backspace","left","h"}` etc. — when adding new keys,
  extend the table.
- **No `teatest` dependency.** It's experimental (`x/exp`); we test
  `Update()` directly with `tea.KeyMsg{...}` synthetic input.
- **Always run `-race`.** Several tests (`TestPushes_ConcurrentScoreAtomicity`,
  `TestBroker_Concurrency`) exist specifically as race-detector bait.
- **Any test needing a DB** uses `storetest.New(t)` which gives a
  fresh `t.TempDir()`-backed SQLite. Never share stores across tests.

## Conventions worth preserving

- **UTF-8 throughout, no Big5.** Use `internal/tui/runewidth.go` helpers
  (`PadRight`, `PadLeft`, `Truncate`, `Width`) for any list rendering
  that mixes ASCII and CJK — `lipgloss`'s automatic width is unreliable
  for double-width glyphs.
- **Form screens skip h/l navigation** so the keys remain available for
  text editing. Currently: `screen_register`, `screen_post_compose`,
  `screen_wb` (compose half).
- **NavigateMsg is the only legitimate way to swap screens.** Sub-models
  emit it from `Update`; `Root.navigate` is the single switch statement
  that constructs the new sub. When you add a new screen, add the case
  there and a `Screen*` constant in `messages.go`.
- **Window-resize forwarding**: `Root.Update` always forwards
  `tea.WindowSizeMsg` to the active sub before its own logic. When
  switching to a new sub, `Root.navigate` resends the last known size
  so the freshly-mounted screen lays out correctly on first render.
