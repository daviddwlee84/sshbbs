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

Block self-targeting at every site that calls `Broker.Send`:

```go
// internal/tui/screen_wb.go — wbComposeModel.submit
if target.ID == from.ID {
    m.err = "cannot send to yourself"
    return m, nil
}

// internal/tui/screen_wb.go — wbThreadModel.submit
if m.cpID == from.ID {
    m.err = "cannot send to yourself"
    return m, nil
}

// internal/tui/screen_wb.go — newWBThreadModel
if deps.User != nil && counterpartyID == deps.User.ID {
    return wbThreadModel{
        deps: deps, cpID: counterpartyID,
        loadErr: errors.New("cannot open thread with yourself"),
    }
}
```

Filter pre-existing self rows out of the inbox roll-up so users don't
re-discover the trap by clicking on themselves:

```sql
-- internal/store/waterballoons.go — ListCounterpartiesFor CTE
agg AS (
    SELECT cp_id, MAX(id) AS last_id, ...
    FROM related
    WHERE cp_id != ?           -- NEW: hide self-WBs
    GROUP BY cp_id
)
```

Add a comment on `Broker.Send` documenting the invariant so the next
caller doesn't reinvent the trap from a different screen.

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
- Mail's `Broker.Send` after `Mail().Insert` could in principle hit the
  same trap if the UI ever lets you mail yourself. Currently `mailCompose`
  doesn't have an explicit guard but the inbox/thread doesn't surface
  mail-from-self as a discoverable action; if a "mail yourself a
  reminder" feature ever lands, mirror the wbCompose guard.
