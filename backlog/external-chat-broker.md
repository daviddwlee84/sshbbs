# External chat broker (Redis pub/sub) for zero-downtime

**Status**: P? — needs spike
**Effort**: ?/L
**Related**: `TODO.md`, `docs/operations/03_deployment.md`, `internal/chat/broker.go`

## Context

The current `chat.Broker` (`internal/chat/broker.go:36-39`) is a
process-local `map[int64][]*Session`. On every restart, live presence
resets to empty and any in-flight `tea.Msg` queued for delivery is
lost. Persistent state (water balloons, mail) survives via the
`delivered_live` flag pattern, so users don't lose messages — but
the online-list flickers and the user experience screams "the server
restarted".

This is the second of two architectural ceilings on zero-downtime
deploys (see `docs/operations/03_deployment.md` and `00_overview.md`
ceiling #2). Decoupling the broker from process memory unblocks one
of the M3 escape hatches.

## Investigation

Not started. The spike below would be the first concrete work.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| A. Redis pub/sub + a presence SET per online user | Mature; well-understood; cheap infra; pub/sub model maps cleanly to current `Broker.Send` / `SendToAll` | New infra dependency; Redis becomes a single point of failure unless replicated |
| B. NATS / JetStream | Better delivery guarantees than pub/sub (queue groups, persistence) | Heavier infra; overkill for in-flight one-line messages |
| C. SQLite as the broker (poll for new rows, `LISTEN`-style) | No new dep | Polling latency contradicts the "live broadcast within 100ms" promise; same single-writer problem already documented |
| D. Socket-handoff via `SO_REUSEPORT` only (`03_deployment.md` option a) | No new dep at all; Linux-native | Solves the *deploy* problem but not the *crash* problem — a hard restart still drops the broker |

Likely answer: **A**, paired with the socket-handoff path (option D)
for deploys. They solve different parts: D handles the planned restart
(no session loss because the old binary keeps existing sessions until
they close naturally), A handles the unplanned crash (presence and
in-flight messages survive in Redis).

## Spike checklist (defines the "?" in `?/L`)

Before promoting from P? to P3, the spike must answer:

- [ ] Does the existing `Sender` interface (`internal/chat/broker.go:16-18`)
      cleanly accommodate a remote sender? It only needs `Send(tea.Msg)`,
      so a Redis-backed implementation would publish to a per-user
      channel; the question is whether `tea.Msg` (an `interface{}`) is
      serializable in practice. Most messages (`PushAddedMsg`,
      `WBReceivedMsg`, `MailReceivedMsg`, etc.) are plain structs —
      should be fine with gob or JSON, but verify.
- [ ] Latency budget: pub/sub round-trip on local Redis is ~1ms; on
      managed Redis (ElastiCache) it's ~5-10ms. The "live broadcast"
      tests currently assert sub-100ms; pick a latency ceiling and
      document it.
- [ ] Failure mode: what happens when Redis is unreachable? Probably
      degrade to local-only delivery (current behaviour) and log
      loudly. Don't fail SSH connections on broker connectivity.
- [ ] Per-user scaling: a SET-per-user for presence + a channel-per-user
      for delivery is N keys + N channels. At 1000 concurrent users
      that's still small; document the upper bound where this stops
      being trivial.

## Current blocker / open questions

- Need a concrete uptime SLO before this is worth doing. M2 is "accept
  3-second blips"; this work only pays off at M3.
- Open: do we want presence durable (Redis SET) or ephemeral (broker
  re-registers on reconnect, presence rebuilds in seconds)? Ephemeral
  is simpler; durable is more accurate during the cutover window.

## Decision (if any)

None yet. Re-evaluate when M3 triggers (first real outage, or uptime
SLA promised to anyone).

## References

- `internal/chat/broker.go` — the current broker
- `docs/operations/03_deployment.md` — the deploy options this unblocks
- `docs/operations/00_overview.md` ceiling #2 — the architectural problem
