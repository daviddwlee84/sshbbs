# sshbbs

A simplified PTT-style BBS served over SSH. Built with
[charmbracelet/wish](https://github.com/charmbracelet/wish) +
[bubbletea](https://github.com/charmbracelet/bubbletea), backed by SQLite
(pure-Go via `modernc.org/sqlite`).

![demo](docs/demo.gif)

> Vim-style `h`/`j`/`k`/`l` navigation alongside arrow keys & Esc/Enter.
> The recording above shows login → boards → article → 推/噓 → `Ctrl+U`
> water-balloon inbox → disconnect, all driven from the keyboard.
> Re-record with `./scripts/record-demo.sh`.

Implements the MUST-HAVE feature set of the planning doc:

- Account register + login over SSH (bcrypt-hashed passwords)
- Default boards: `Welcome`, `Test`, `ChitChat`
- Read & post articles (UTF-8, CJK-aware widths via `go-runewidth`)
- 推文 (推 / 噓 / →) with live broadcast to other viewers of the same article
- 文章 `r` 回文 (Re:) — quoted reply preserves an email-style thread
- 水球 (water balloons) — private one-line messages with offline persistence
  and on-reconnect replay
- 個人設定: change password, edit bio, configure per-event webhook
  notifications (push / wb / mail / reply). Each user picks any
  HTTPS URL — Discord webhook, ntfy.sh topic, self-hosted apprise-api,
  Slack, custom — and the BBS auto-detects Discord URLs to use the
  embed payload format. See [`docs/notifications.md`](docs/notifications.md)
  for the per-target walkthroughs and the operator threat model

## Quickstart

```bash
make hostkey            # generates .ssh/host_ed25519 (run once)
make run                # starts the server on :2222

# In another terminal, register:
ssh new@localhost -p 2222     # password is ignored — fill the in-TUI form

# Then log in:
ssh alice@localhost -p 2222   # use the password you registered with
```

## Local dev with Docker

If you'd rather not install Go locally:

```bash
make compose-up         # builds, generates a host key into a named volume, runs on :2222
ssh new@localhost -p 2222
make compose-down
```

The `bbs-data` and `bbs-keys` volumes persist across restarts so accounts
and the host key survive `compose-down`/`compose-up`. See
[`docs/operations/04_docker.md`](docs/operations/04_docker.md) for what the
multi-stage build and `hostkey-init` service do, and a warning about
volume drivers that don't honour SQLite's filesystem locks (don't put
`bbs-data` on NFS).

Inside the TUI:

- `↑`/`↓` or `j`/`k` to move, `Enter` to select
- `p` in a board view to post
- `+` / `-` / `=` in an article view for 推 / 噓 / →
- `Ctrl+U` from anywhere logged-in to open the water-balloon inbox; `c` to
  compose, `r` to reply
- `Esc` / `Backspace` to navigate back
- `Ctrl+C` to disconnect

## Two-user demo

```bash
# T1
ssh alice@localhost -p 2222

# T2
ssh bob@localhost -p 2222

# From alice: Ctrl+U → c → bob → "hi" → Ctrl+S
# Bob sees a toast in the footer within ~100ms.
```

## Common gotchas

- **`Host key verification failed`** after running `make hostkey` again:

  ```bash
  ssh-keygen -R '[localhost]:2222'
  ```

- **CJK looks like garbage**: the client terminal must be UTF-8.

  ```bash
  echo $LANG     # should match *.UTF-8
  ```

- **`SQLITE_BUSY`** under high write load: the pure-Go driver is slower than
  the CGO `mattn/go-sqlite3`. WAL + a 5s busy_timeout + a process-level
  `writeMu` keeps us comfortable for dozens of users; swap drivers if it bites.

## Layout

```
cmd/sshbbs/             entrypoint, flag parsing, signal handling
internal/server/        wish bootstrap + password-auth middleware
internal/auth/          register/login (bcrypt, validation)
internal/store/         SQLite handle, migrations, repos
internal/store/migrations/  *.sql, embedded with go:embed
internal/chat/          in-memory broker (presence + live send)
internal/tui/           bubbletea root + per-screen models
docs/ptt_trace_code/    notes mapping pttbbs concepts to our schema
```

## Development

```bash
make build              # build the binary
make watch              # auto-rebuild on .go/.sql changes (dev loop)
make test               # run tests
make test-race          # race detector (CI standard)
make cover              # coverage by package
make db-reset           # delete the SQLite DB
make tidy               # go mod tidy
make fmt vet            # gofmt + go vet
```

`make watch` uses [`air`](https://github.com/air-verse/air), pinned via the
Go 1.24+ `tool` directive (`go tool air`) — no global install needed.
Edits to `.go` or `.sql` files trigger SIGINT → graceful drain → rebuild →
relaunch; active SSH clients get disconnected and just reconnect.

See [`docs/development.md`](docs/development.md) for the full local-dev
playbook (watcher tuning, Delve step-debugger, test-watch recipes,
graceful-shutdown contract) and [`docs/testing.md`](docs/testing.md) for
the layered testing strategy.

## Roadmap & lessons learned

> See `docs/ptt_trace_code/00_overview.md` for what we deliberately don't
> implement, and the project plan
> `./.claude/plans/ptt-bbs-user-register-account-gentle-cookie.md` for the P1 / P2 backlog.

Forward-looking work — long-term ideas, deferred items, things needing
evaluation — lives in [`TODO.md`](TODO.md), prioritised P1 → P3 with effort
estimates (S/M/L/XL). Items with accompanying research, design notes, or paused
troubleshooting link to a corresponding [`backlog/<slug>.md`](backlog/) doc.

Backward-looking knowledge — past traps and non-obvious debugging — lives in
[`pitfalls/`](pitfalls/), titled by symptom so future-you can grep the error
message and land on the root cause + workaround instead of re-debugging from
scratch.

## Resources

- [ptt/pttbbs: PTT BBS source code](https://github.com/ptt/pttbbs)
  - [ptt/pttbbs | DeepWiki](https://deepwiki.com/ptt/pttbbs)
  - [Home · ptt/pttbbs Wiki](https://github.com/ptt/pttbbs/wiki)
- [批踢踢 - 維基百科，自由的百科全書](https://zh.wikipedia.org/zh-tw/%E6%89%B9%E8%B8%A2%E8%B8%A2)
