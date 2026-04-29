# Testing strategy

We follow the conservative pyramid recommended by the bubbletea / wish
ecosystem: lots of plain Go tests at the bottom, a handful of focused
integration tests in the middle, and a thin layer of end-to-end smoke
tests on top. **No `teatest` dependency** — that package lives under
`x/exp` (experimental) and we don't want our core test strategy depending
on it.

## Layers

### Layer 1 — Pure Go unit tests (most of our coverage)

Anything that doesn't need an SSH session or a TUI program. Tested with
the standard library `testing` and run under `-race`.

| Area | Files | What it covers |
|------|-------|----------------|
| `auth` | `internal/auth/auth_test.go` | Register validation, bcrypt round-trip, login + `NoteLogin`, case-insensitive lookup |
| `store` | `internal/store/*_test.go` | Migrations idempotency, repo CRUD for boards/articles/pushes/water_balloons, **push score atomicity under N concurrent inserts**, inbox ordering, COLLATE NOCASE behaviour |
| `chat` | `internal/chat/broker_test.go` | Register / Unregister / 雙開 (multi-session), `Send`/`SendToAll` semantics, sender exclusion, **concurrent register/unregister/send stress** |

Coverage today: `auth` 81%, `chat` 84%, `store` 74%.

### Layer 2 — Bubble Tea `Update()` tests

Direct calls to a model's `Update` with synthetic `tea.KeyMsg` values, asserting
the returned model state and the `tea.Msg` produced by the `tea.Cmd`. No PTY,
no real `tea.Program`, no goroutines. This is what catches keybinding
regressions like the h/l navigation work.

| File | Focus |
|------|-------|
| `internal/tui/screen_main_menu_test.go` | Cursor movement & clamping; `enter`/`l`/`right`/`space`/`1`-`3` all route correctly; `3` quits |
| `internal/tui/screen_board_list_test.go` | All four back-keys (`esc`/`backspace`/`left`/`h`) → `ScreenMainMenu`; all four forward-keys (`enter`/`space`/`right`/`l`) → `ScreenBoardView` with the right `BoardID` |
| `internal/tui/screen_article_view_test.go` | Back-key parity; `+`/`-`/`=` open the inline push input with the right kind; `PushAddedMsg` for an unrelated article is ignored; for the current article triggers a re-fetch |

Pattern, distilled (see `keyOf` and `runCmd` helpers in
`screen_main_menu_test.go`):

```go
m := newBoardListModel(deps)
_, cmd := m.Update(keyOf("h"))
nav := runCmd(cmd).(NavigateMsg)
if nav.To != ScreenMainMenu { t.Errorf(...) }
```

We test in the same package (`package tui`, not `tui_test`) so we can
reach the unexported screen constructors. This is the standard Go pattern
for testing intra-package types and is how the bubbletea project itself
tests its sub-models.

### Layer 3 — SSH + middleware integration (deferred)

`charmbracelet/wish` ships `wish/testsession` for in-process SSH server
tests. Pattern:

```go
addr := testsession.Listen(t, srv)
sess, _ := testsession.NewClientSession(t, addr, &gossh.ClientConfig{...})
sess.Run("")
```

We **don't have these yet** — currently the only Layer-3 coverage is the
auth round-trip in `auth_test.go` (which exercises bcrypt + Users repo
together). If middleware composition or password-auth hand-off ever
breaks, this is where to add the test.

### Layer 4 — End-to-end (PTY + real ssh)

Driven by `expect`-based scripts during development. Scripts in
`/tmp/sshbbs_*.exp` (not committed; ad-hoc) covered the major flows:
- `sshbbs_register_test.exp` — SSH user `new` register flow
- `sshbbs_login_test.exp` — correct + wrong password
- `sshbbs_browse_test.exp` — board list / view / article navigation
- `sshbbs_post_test.exp` + `_newlines` — post compose with CJK + newlines
- `sshbbs_push_solo.exp` + `_live.exp` — 推/噓/→ + live broadcast across two sessions
- `sshbbs_wb_live.exp` + `_offline.exp` — water balloon live + replay
- `sshbbs_hl_test.exp` — h/l navigation parity smoke test
- `sshbbs_shutdown_test.exp` — SIGTERM during active session

Promote any of these to committed Go tests using
`golang.org/x/crypto/ssh` + `Netflix/go-expect` if regressions become
recurring.

## Running

```bash
go test ./...                       # quick run
go test -race ./...                 # what CI runs (always)
go test -race -count=1 ./...        # disable result cache
go test -cover ./...                # per-package coverage
go test -run TestPushes -v ./internal/store/...   # focused
```

## Conventions

- **Table-driven** for keybinding parity and validation matrices.
- **`t.TempDir()` + `storetest.New(t)`** for any test that needs a DB.
  Never share stores across tests.
- **`-race`** is mandatory; the `pushes` and `chat.broker` tests exist
  partly as race-detector bait.
- **No flake budget**: tests must be deterministic. Time-sensitive cases
  use `time.Sleep(15ms)` where ordering matters; if a test starts
  flaking, fix the root cause rather than retry.
- **Don't golden-file the TUI views.** ANSI escape sequences vary by
  color profile and break in CI. Assert on string contents instead
  (e.g. `contains(out, "看板列表")`).
