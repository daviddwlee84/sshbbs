# Postgres migration plan

**Status**: P? — gated on M4 trigger
**Effort**: ?/XL
**Related**: `TODO.md`, `docs/operations/03_deployment.md`, `internal/store/`

## Context

The codebase is SQLite-first. Forward-looking, the only thing that
forces a database change is the SQLite-WAL-one-writer ceiling (see
`docs/operations/00_overview.md` ceiling #1). At small scale this is
*not a problem*: the process-level `Store.writeMu` keeps writes
serialized and `SQLITE_BUSY` doesn't appear under tens of users.

It becomes a problem only at M4 — single-box CPU or write throughput
sustained at 70%+. Then horizontal scaling (N stateless `sshbbs`
processes) is the path forward, and that needs a database that
supports genuine concurrent writers. SQLite cannot.

This doc records the rough shape of that migration so future-us can
estimate it without re-researching.

## Investigation

Not started. This is a sketch, not a plan.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| A. Migrate to Postgres | Best-known; mature Go drivers (`pgx`); `goose` and `sqlc` both Postgres-native | Real infra now (replicas, backups, monitoring); migration is irreversible |
| B. Migrate to MySQL / MariaDB | Cheaper managed offerings on some clouds | Worse JSON / generated-column support; weaker at the kind of indexed range scans we use for board listings |
| C. Migrate to CockroachDB | Wire-compatible with Postgres; horizontal writes built-in | Operational overhead larger than Postgres + replicas; pricier; latency ceiling on multi-region writes |
| D. Don't migrate. Vertically scale the SQLite box | Cheapest; postpones architectural rewrite | Hard ceiling at single-box throughput; no escape hatch when reached |

Likely answer: **A** at M4. **D** is the right answer at M3 and below
(stay on SQLite, accept the ceiling). **C** is interesting only if the
project becomes geographically distributed, which a Taiwanese BBS
arguably wants — but that's a different conversation.

## Migration shape (rough)

The hard parts, ranked by effort:

1. **SQL dialect drift.** SQLite is permissive (`INTEGER PRIMARY KEY`
   auto-increment, no strict typing pre-3.37, partial-index syntax
   subset). Every migration in `internal/store/migrations/` would need
   review. `0003_add_mail.sql` uses SQLite-style `INTEGER PRIMARY KEY`;
   Postgres wants `BIGSERIAL` or `BIGINT GENERATED ALWAYS AS IDENTITY`.

2. **Connection pooling.** SQLite uses one connection per write
   (serialized by `writeMu`); Postgres expects a pool. `database/sql`
   with `pgx` handles this, but `Store.writeMu` becomes meaningless
   and has to come out — and every place it's relied on becomes a
   row-level transaction concern.

3. **`pushes` atomicity.** `PushRepo.Create` runs `INSERT INTO pushes` +
   `UPDATE articles SET recommend_score` under `writeMu` (canary test:
   `TestPushes_ConcurrentScoreAtomicity`). On Postgres this is a
   single transaction — easier in some ways (no app-level mutex needed)
   but needs explicit `SELECT ... FOR UPDATE` on the article row to
   avoid lost updates.

4. **Embedded DB → external DB.** `cmd/sshbbs/main.go:28` opens a file
   path. Postgres needs a connection string, env-var-driven. Aligns
   naturally with `docs/operations/01_environments.md` env-var
   migration (M2 prep).

5. **Migration data plan.** One-shot `pgloader` from
   `data/bbs.db` for the cutover, tested repeatedly against staging
   before any prod attempt.

## Spike checklist

Before promoting from P? to a real priority:

- [ ] Audit every migration file for SQLite-isms. List them.
- [ ] Run `pgloader` against a sample of `data/bbs.db`. Does it
      round-trip cleanly?
- [ ] Estimate connection-pool sizing for the broker fan-out workload.
      A push broadcast to 100 viewers is 100 reads — does this exhaust
      a default pool of 25?
- [ ] Confirm `goose` works with Postgres-flavoured migrations sharing
      the same `fs.FS` as SQLite (it should — `goose` dispatches by
      driver). If not, the migrations folder splits.

## Current blocker / open questions

- M4 hasn't triggered. This may never happen; that's fine.
- Open: do we want "Postgres at M4" or "Postgres if-and-when single-box
  is no longer enough, otherwise stay on SQLite forever"? The latter
  is honest; the former pre-commits a rewrite that may never pay off.

## Decision (if any)

None yet. Re-evaluate when single-box write throughput hits 70%
sustained — the M4 trigger.

## References

- `internal/store/store.go` — the SQLite handle + `writeMu`
- `internal/store/migrations/*.sql` — the SQL files that would need dialect review
- `docs/operations/00_overview.md` ceiling #1 — the architectural reason this exists
- `docs/operations/03_deployment.md` option (c) — the deploy pattern this unblocks
