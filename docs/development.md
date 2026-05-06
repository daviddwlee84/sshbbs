# Development workflow

How to iterate on sshbbs locally: editing a `.go` (or `.sql`) file →
seeing the change in a fresh SSH session, with no manual `Ctrl-C` /
`make run` cycle. This page is the source of truth for "how do I run
this thing while I'm hacking on it" and complements
[`testing.md`](testing.md) (which covers how to verify it).

## TL;DR

```bash
make watch
```

…will boot the server, watch `cmd/` and `internal/`, and on every save
of a `.go` / `.sql` file: SIGINT the running process → wait for graceful
drain → rebuild → relaunch. Read the rest if you want to know **why** this
works for a server we cannot truly hot-reload, what graceful-shutdown
contract the watcher relies on, and how to attach a step debugger.

## Why a watcher works for a Go SSH server

Go is a compiled language with no Erlang-style mid-process code swap, so
"hot reload" here means **kill + restart**, not "patch the running
binary". That sounds drastic for a server, but for our case the state
that matters is durable:

- `data/bbs.db` (SQLite, with WAL) — accounts, articles, 推文, 水球 inbox
  all survive the restart. `cmd/sshbbs/main.go` defers `st.Close()` so
  WAL is checkpointed before the process exits.
- `.ssh/host_ed25519` — same host key on every relaunch, so SSH clients
  don't see `Host key verification failed`.
- The connected client **does** get disconnected on each rebuild. They
  just `ssh alice@localhost -p 2222` again and pick up where they were.
  This is the same trade-off `nodemon` makes for HTTP servers — the only
  difference is that browsers auto-reconnect and SSH doesn't.

In practice this is fine: a dev iteration is "save a file, switch to a
terminal, ssh in, see the change". The watcher does the rebuild for you
and your client takes ~2 seconds longer than it otherwise would.

## What we picked: `air` via Go 1.24+ `tool` directive

[`air-verse/air`](https://github.com/air-verse/air) is the de-facto Go
file-watch / live-reload tool. Since Go 1.24 the `go.mod` `tool`
directive lets us declare it as a project-pinned dev dependency, runnable
through `go tool` — no global `go install`, no separate `tools.go`
blank-import workaround, version locked in `go.sum` like any other dep.

The one-time setup (already done — recorded here for posterity):

```bash
go get -tool github.com/air-verse/air@latest
```

Adds two things to `go.mod`:

```
require github.com/air-verse/air v1.65.1
tool github.com/air-verse/air
```

Day-to-day, `make watch` just runs `go tool air -c .air.toml`.

> **Heads-up about transitive bumps.** `go get -tool` participates in
> normal Go module MVS, so air's dep graph can pull our shared
> dependencies forward. Adding air bumped `golang.org/x/crypto`
> v0.37 → v0.41 and `golang.org/x/net` v0.38 → v0.43 in our require list.
> Both are minor patch bumps with no API changes — verify with
> `make test-race` after the next dependency refresh.

## How `.air.toml` is tuned for this project

```toml
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/sshbbs ./cmd/sshbbs"
  bin = "./tmp/sshbbs"
  args_bin = ["-addr=:2222", "-db=data/bbs.db", "-hostkey=.ssh/host_ed25519"]
  delay = 500
  send_interrupt = true
  kill_delay = "4s"
  stop_on_error = false
  include_ext = ["go", "sql"]
  exclude_dir = ["tmp", "data", ".ssh", "dist", ".specstory", ".claude", "scripts", "docs", "backlog", "pitfalls", "testdata"]
  exclude_regex = ["_test\\.go$"]
```

Project-specific knobs worth understanding:

| Setting | Why this value |
|---|---|
| `send_interrupt = true` | Air sends SIGINT (not the default SIGTERM). `cmd/sshbbs/main.go:73` listens for both via `signal.NotifyContext`, but SIGINT is the same path as a human pressing `Ctrl-C` — easier to mental-model. |
| `kill_delay = "4s"` | **Hard contract**: must be ≥ `main.go:97` `drainTimeout = 3 * time.Second`. The shutdown path in `main.go:96-131` takes up to 3 s to drain active sessions; if air SIGKILLs sooner, the SQLite WAL may not checkpoint and the next boot will see "ok but slow first read". 4 s gives ~1 s buffer. |
| `include_ext = ["go", "sql"]` | `internal/store/migrate.go` uses `go:embed migrations/*.sql`, so SQL changes need a **rebuild** to take effect. They're compiled into the binary, not read at runtime. |
| `exclude_regex = ["_test\\.go$"]` | Saving a test file should run tests, not bounce the server. Use `make test` separately, or pair with the test-watch recipe below. |
| `exclude_dir` includes `.specstory`, `.claude`, `docs`, `backlog`, `pitfalls` | Agent transcripts and design notes get written during normal sessions; we don't want those triggering rebuilds. |
| `stop_on_error = false` | Build fails (typo, unused import) → air prints the error and keeps watching. Fix and re-save. With `true`, you'd have to manually restart `make watch`. |
| `delay = 500` | Debounces flurry-saves (e.g. `gofmt -w` on save touching multiple files). 500 ms is short enough not to feel laggy, long enough to coalesce. |

## What graceful shutdown actually does

Worth knowing because the watcher's correctness depends on it.
`cmd/sshbbs/main.go:96-131`:

1. `srv.Shutdown(ctx)` — stop accepting new SSH connections (3 s timeout).
2. `broker.SessionsSnapshot()` — collect every live `*Session`.
3. For each session: `s.Program.Send(tea.Quit())` — bubbletea unmounts
   the view, returns from `program.Run`, and the wish middleware closes
   the channel.
4. Poll `broker.SessionsSnapshot()` every 100 ms for up to 3 s, waiting
   for the count to hit zero.
5. Deferred `st.Close()` flushes the SQLite WAL.

Air's `send_interrupt = true` + `kill_delay = "4s"` plugs straight into
this — no additional code in `main.go` is needed for watch mode.

## Tools we considered and didn't pick

- **[`gow`](https://github.com/mitranim/gow)** — `gow run ./cmd/sshbbs` is
  the simplest possible watcher. We didn't pick it because it doesn't
  expose `kill_delay` / `send_interrupt` knobs, and our 3 s graceful
  drain needs a watcher that won't `SIGKILL` us mid-drain. Fine for tiny
  CLIs and demos.
- **`fswatch` / `entr` + Makefile** —
  `find . -name '*.go' | entr -r make run` works, but ends up
  reimplementing air's exclude-rules / debounce / build-error display
  badly. Also a hard install dep on `brew install fswatch` for macOS
  contributors. Use it for a one-off shell hack, not as project default.
- **In-process hot-reload** (Go `plugin` package, `yaegi` interpreter) —
  too invasive. Would have to restructure how wish/bubbletea manage
  per-session program lifetimes. Engineering cost ≫ value.

## Step debugger (Delve)

Watch mode and a step debugger are **orthogonal** — air rebuilds and
restarts; Delve attaches to one process and lets you set breakpoints.
Don't combine them (air will keep killing the process you're attached
to).

```bash
# In one terminal: launch under dlv on a non-default port so it doesn't
# clash with whatever `make watch` may be running.
dlv debug ./cmd/sshbbs -- -addr=:2223 -db=data/dev.db -hostkey=.ssh/host_ed25519

# Then in dlv's REPL:
(dlv) break internal/tui/screen_article_view.go:142
(dlv) continue
```

VS Code / GoLand / nvim-dap drive the same Delve underneath; configure
their `launch` block with the same `-addr=:2223 -db=data/dev.db ...`
arguments.

For attaching to a server you already started under `make run`:

```bash
dlv attach $(pgrep sshbbs)
```

## Test watch (optional)

Not wired into the Makefile because `go test ./...` finishes in seconds
and most folks don't want a perma-watcher for tests. If you want one:

```bash
# Re-run on every .go save, in colour, just the package you're working on:
find internal/tui -name '*.go' | entr -c go test -race -v ./internal/tui/...
```

Or with [`gotestsum`](https://github.com/gotestyourself/gotestsum):

```bash
gotestsum --watch --format testname -- -race ./internal/tui/...
```

Neither is checked in — they're personal-preference tools.

## Reference: the dev command map

| Need | Command |
|---|---|
| One-shot run (no watcher) | `make run` |
| Watch + auto-rebuild | `make watch` |
| Race-detector full suite | `make test-race` |
| Step debugger | `dlv debug ./cmd/sshbbs -- -addr=:2223 -db=data/dev.db -hostkey=.ssh/host_ed25519` |
| Production-shape graceful shutdown drill | `make run`, in another terminal `kill -INT $(pgrep sshbbs)`, watch the drain logs |
| Reset DB to seed state | `make db-reset && make run` |

## See also

- `cmd/sshbbs/main.go:73-131` — the signal handling and graceful drain
  the watcher leans on.
- `.air.toml` — checked-in config, edit if you change build flags or add
  new file extensions to watch (e.g. if migrations move out of
  `internal/store/migrations/`).
- [Go 1.24 release notes — `tool` directive](https://go.dev/doc/go1.24#go-command).
- [`testing.md`](testing.md) — the testing pyramid that complements the
  dev loop.
