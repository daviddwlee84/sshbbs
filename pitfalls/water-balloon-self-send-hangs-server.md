---
title: Sending a water balloon to yourself hangs the SSH session (no further keys, can't relogin)
date: 2026-05-06
---

## Symptom

After typing your own userid into the 丟水球 compose form and pressing
Ctrl+S, the SSH session freezes:

- The compose screen stays on screen, "✓ sent" never appears.
- No further keystrokes are accepted (Esc, Ctrl+C, even ↑/↓ all do nothing).
- The Go process keeps running — `ps`/`top` show it alive, the listener
  socket is still bound — but **every existing AND new SSH session hangs**:
  newly logging in via `ssh alice@... -p 2222` connects but produces no
  TUI output; you have to `kill` the server.
- No panic, no goroutine dump (`SIGQUIT` to the server prints one — see
  Detection below).

The recipient field can be the literal userid (e.g. `david`) or any case
variation that resolves to the same `users.id` as the sender's. Mailing
yourself does NOT trigger this — only water balloons.

## Root cause

`tea.Program.Send(msg)` (bubbletea v1.3.10, `tea.go:244` and `tea.go:774-779`)
writes to a channel declared as:

```go
msgs: make(chan Msg)   // unbuffered
```

The send path inside `wbComposeModel.submit` (and identically inside
`wbThreadModel.submit` since Phase 2) calls:

```go
m.deps.Broker.Send(target.ID, WBIncomingMsg{ID: wb.ID, ...})
```

When `target.ID == from.ID`, the broker iterates over `b.sessions[target.ID]`
— which is the caller's OWN session — and does `s.Program.Send(msg)`. That
becomes a blocking `p.msgs <- msg` because:

1. The bubbletea program loop is currently INSIDE `Update(KeyMsg{Ctrl+S})`,
   processing the submit. It is the only goroutine that reads `p.msgs`.
2. Until `Update` returns, nobody reads the channel.
3. Until `p.msgs <- msg` succeeds, `Update` cannot return.

Classic deadlock. The lock isolates the affected session's goroutine, but
because wish multiplexes through a shared listener, other sessions stack
up behind whatever shared resource the hung goroutine still holds —
practically, the whole server stops servicing new TUI traffic.

`tea.Program.Send`'s `select { <-ctx.Done(); p.msgs <- msg }` only escapes
on program shutdown, which the user can't trigger from a frozen TUI.

## Workaround

**Sidestep the broker call when target == self** instead of refusing the
operation. The compose / thread / inbox stay usable as a self-memo
feature (Slack/WeChat/Telegram precedent), and the deadlock can't fire
because `Broker.Send` is never invoked with the caller's own UID:

```go
// internal/tui/screen_wb.go — wbComposeModel.submit
wb, err := m.deps.Store.WaterBalloons().Insert(ctx, from.ID, from.UserID, target.ID, body, false)
if err != nil { ... }
if target.ID == from.ID {
    // Self-WB (memo). Skip Broker.Send; mark read so it doesn't
    // replay as a toast on next reconnect.
    _ = m.deps.Store.WaterBalloons().MarkRead(ctx, wb.ID)
} else if m.deps.Broker != nil {
    delivered := m.deps.Broker.Send(target.ID, WBIncomingMsg{ID: wb.ID, ...})
    if delivered { _ = m.deps.Store.WaterBalloons().MarkRead(ctx, wb.ID) }
}
```

```go
// internal/tui/screen_wb.go — wbThreadModel.submit (Phase 2 DM input)
// Same shape: Insert, then if cpID == from.ID skip broker + mark read,
// else Broker.Send + maybe-mark-read.
```

UI markers so users see they're in a self-conversation and don't mistake
it for a generic chat:

```go
// inbox row
if it.UserID == viewerID {
    nameStr = "📝 yourself"
}

// thread header
if m.cpID == m.deps.User.ID {
    headerLabel = "📝 yourself (memo)"
}
```

`ListCounterpartiesFor` does NOT filter self-rows — they appear in the
inbox as a regular counterparty so the memo feature is discoverable.

A `Broker.Send` doc comment names the invariant ("don't call with toUID
== caller's own UserID inside an Update path") so a future feature
that genuinely targets self gets the warning and follows the
skip-broker-call pattern rather than reinventing the deadlock.

## Detection

If a future report sounds the same — server alive, no further input, no
new connections work — confirm via `SIGQUIT`:

```
kill -QUIT <pid>
```

You'll see a goroutine dump where one (or more) goroutines are blocked at:

```
runtime.gopark ...
runtime.chansend ...
github.com/charmbracelet/bubbletea.(*Program).Send ...
github.com/daviddwlee84/sshbbs/internal/chat.(*Broker).Send ...
github.com/daviddwlee84/sshbbs/internal/tui.wbComposeModel.submit ...
```

The Send→Send→submit chain inside the same session is the smoking gun.

## Prevention

- **Never call `Broker.Send(toUID, ...)` with `toUID == sender's UserID`
  from inside any bubbletea Update path.** Document this on the broker
  method itself; future agents/screens grep for "Broker.Send" before
  adding a new caller.
- If a self-send semantic is ever genuinely wanted, refactor `Broker.Send`
  to spawn a goroutine per session (`go s.Program.Send(msg)`) — but that
  loses ordering guarantees relative to other broker traffic and forces
  the existing tests (`TestBroker_MultipleSessionsSameUser`,
  `TestBroker_SendToAllExcludesSender`) to switch from "assert
  immediately after Send" to an `eventually`-style wait. Worth it only if
  the use case demands it; today the app-layer guard is sufficient.
- Mail's `Broker.Send` after `Mail().Insert` had the **same trap** —
  fixed in the same way (`screen_mail.go` `mailComposeModel.submit`):
  when `target.ID == from.ID`, skip `Broker.Send` and call
  `Mail().MarkRead(mail.ID)` immediately so the unread counter doesn't
  bump for memos. Reply quoting (markdown `> ` blockquote of parent
  body, attribution line) was added in the same change so a self-mail
  thread reads coherently.
