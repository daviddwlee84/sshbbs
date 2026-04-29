# 03 · Deployment — graceful shutdown today, zero-downtime later

Today the project does a clean *stop-the-world* shutdown that drops
every active SSH session in under 3 seconds. For M0–M2 that's the right
amount of complexity. This doc names the architectural barriers to
"true" zero-downtime and ranks the escape hatches by cost.

## What works today (M0–M2)

`cmd/sshbbs/main.go:65-103` already implements:

1. SIGTERM / SIGINT caught via `signal.NotifyContext` (line 45)
2. `srv.Shutdown(listenerCtx)` — refuses new connections (line 74)
3. `program.Send(tea.Quit())` to every live bubbletea program (line 85)
4. Poll `broker.SessionsSnapshot()` with 3s drain timeout (lines 89-102)
5. Deferred `st.Close()` flushes WAL

Real-world impact:

- Empty broker: shutdown is sub-second. The DB closes cleanly, the
  listener returns from `Accept`, the process exits.
- Active broker: capped at 3 seconds of "please log back in" UX. SSH
  clients see their session terminate; the next `ssh ...` reconnects.

For M0–M2 — solo dev through small public beta — accept the 3-second
maintenance blip and stop here.

## The architectural ceiling

True zero-downtime means *no client sees the cutover*. Two pieces of
this codebase block it:

1. **SQLite WAL allows exactly one writer per file.** Run two binaries
   against `data/bbs.db` and the second one's writes either flap with
   `SQLITE_BUSY` or — worse, on NFS / some Docker volume drivers —
   silently corrupt. So even a perfect blue/green proxy can't have
   both binaries writing at once.

2. **`chat.Broker` (`internal/chat/broker.go:36-39`) lives in process
   memory.** A new binary cannot inherit the `sessions map[int64][]*Session`
   from the old one. Live presence resets to empty.

These constraints rule out the lift-and-shift "two replicas behind a
load balancer" pattern that web services use. SSH's long-lived sessions
make it a different problem.

## Escape hatches, ranked by cost

### (a) Socket-handoff via `SO_REUSEPORT` (M3, cheapest)

Linux only. Both old and new binaries `bind(:2222)` with
`SO_REUSEPORT`; the kernel routes new connections to the new binary
while the old one keeps its existing sessions until they close
naturally. Single SQLite writer at any moment because the old binary
exits before the new one starts taking writes — except in a brief
overlap where both are reading. Reads are fine; writes need a small
piece of coordination (e.g. the old binary stops accepting `POST`
once SIGTERM arrives).

**Cost**: ~1 day of work + a Linux-only deploy assumption. New code
is ~20 lines in `internal/server/server.go`.

### (b) External broker (Redis pub/sub) (M3)

Replace `chat.Broker` with a thin wrapper around Redis pub/sub +
SETs for presence. Each binary subscribes to its users' channels;
sessions are still process-local, but presence and inter-session
delivery survive restarts. Solves problem #2 without touching #1.

**Cost**: ~1 week + a new infra dependency. Worth pairing with (a).
See `backlog/external-chat-broker.md`.

### (c) Postgres + horizontal SSH frontends (M4, expensive)

The only path that allows *true* concurrent writers. Drop SQLite,
move to Postgres, run N stateless `sshbbs` processes behind an
SSH-aware load balancer (HAProxy with `mode tcp`, or a dedicated SSH
proxy that understands session affinity).

**Cost**: ~1 month + ongoing infra spend. Rarely worth it for a
hobby-scale BBS. See `backlog/postgres-migration-plan.md`.

## Recommendation

Stay at the M2 "3-second maintenance blip" tier indefinitely unless
either ceiling forces the issue. If you must move, pick (a) before (b)
before (c). Each step buys ~10× more uptime headroom for ~10× more
cost.

## Cross-references

- `cmd/sshbbs/main.go:65-103` — the graceful-shutdown code this doc documents
- `internal/chat/broker.go:36-39` — the in-memory state that blocks zero-downtime
- `00_overview.md` ceilings #1 and #2 — the same constraints, summarised
- `backlog/external-chat-broker.md` — design spike for option (b)
- `backlog/postgres-migration-plan.md` — design spike for option (c)
