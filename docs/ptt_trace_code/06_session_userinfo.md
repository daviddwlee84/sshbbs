# 06 · `userinfo_t` (UTMP) → in-memory `Broker.sessions`

## pttbbs reference

`userinfo_t` is the per-session record kept in a fixed-size shared-memory
array (8000 slots historically). It tracks: which user, what board they're
on, idle time, current "mode" (talk/edit/invisible), pager settings, and
a pointer to the talk-buffer for water balloons. The `utmpd` daemon (or
embedded equivalent) maintains it.

The shared-memory choice is forced by pttbbs's multi-process architecture:
each `mbbsd` is its own Unix process, so they communicate through SHM.

## Our model

One Go process owns every session, so we use a plain `sync.RWMutex`-guarded
map.

```go
// internal/chat/broker.go
type Broker struct {
    mu       sync.RWMutex
    sessions map[int64][]*Session  // user_id -> active sessions
}

type Session struct {
    UserID    int64
    UserIDStr string
    Program   *tea.Program  // the bubbletea program for this SSH session
}
```

The same user may have multiple sessions ("雙開"). Operations:

- `Register(s)` — appends.
- `Unregister(uid, *tea.Program)` — removes the session whose Program
  pointer matches; deletes the user entry when empty.
- `Send(toUID, msg)` — fans out to every session of toUID. Returns true if
  any received (i.e. user online).
- `SendToAll(excludeUID, msg)` — broadcast (used for live push updates).
- `OnlineList()` — externally-visible snapshot for "who's online" UI.

## Lifecycle wiring (`internal/server/server.go`)

```go
broker.Register(&chat.Session{...})
go func() {
    <-sess.Context().Done()  // SSH session closed
    broker.Unregister(user.ID, p)
}()
```

## What we drop / defer

- **Idle time, current board, "mode" tracking.** We don't show or use any of
  it yet. P1 if we want the online-list screen to be richer.
- **Pager / DND settings.** Same: P1.
- **Cross-process IPC.** Hard requirement when scaling beyond one Go binary;
  for now, single-process is fine. Adding it later means a Redis pub/sub or
  similar in front of `Broker.Send` — the interface stays the same.

## Why not a goroutine per user with channels?

The `*tea.Program` *is* the goroutine — `program.Send(msg)` is the channel.
Adding a second goroutine layer would just rebroadcast and complicate
shutdown. The only thing we need is a way to look up programs by user, which
the map gives us.
