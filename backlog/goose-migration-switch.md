# Switch migration runner to pressly/goose

**Status**: P3 — gated on M2 entry trigger
**Effort**: M
**Related**: `TODO.md`, `docs/operations/02_migrations.md`, `internal/store/migrate.go`

## Context

The current home-grown runner (`internal/store/migrate.go:13-103`) is
forward-only and has no down migration. It's correct for M0–M1 because
exactly one developer touches the schema and `make db-reset` is the
rollback. The switch becomes mandatory when either of two M2 triggers
fires:

1. A second engineer commits a migration → can't roll back to test
   against the previous binary without nuking data.
2. First prod deploy → a bad migration that you can't reverse means
   a corrupt database, not a 30-second outage.

`docs/operations/02_migrations.md` documents the recommended target
(`pressly/goose`) and the expand/migrate/contract pattern that comes
with it.

## Investigation

Not started — this is a design note for the eventual switch.

Things known up-front:

- `goose.SetBaseFS(migrationsFS)` accepts an `fs.FS`, so the existing
  `//go:embed migrations/*.sql` directive doesn't have to move. This
  is the M0/M1 invariant the switch must preserve (see `00_overview.md`
  ceiling #4).
- Go-style migrations have signature
  `func(ctx context.Context, tx *sql.Tx) error` and can call into the
  existing repo helpers — important for data rewrites that need
  `Store.writeMu`.
- `goose_db_version` replaces `schema_migrations`. The first goose
  migration must `INSERT` historical versions to mark them applied;
  otherwise goose will try to re-run them.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| A. Switch to `pressly/goose` | Native Go migrations; small dep; `fs.FS` support; opt-in down migrations | Migration table rename + one-time backfill |
| B. Switch to `golang-migrate/migrate` | Largest community | Go migrations second-class; heavier dep; data rewrites can't reuse `writeMu` cleanly |
| C. Switch to `ariga/atlas` | Declarative schema diff | Different working style (write desired state, atlas computes diff); overkill for 6 tables |
| D. Hand-roll down migrations on the existing runner | No dep changes | Re-implementing what goose already does; will end up half a goose |

Decision: **A** at M2 entry. **D** is tempting but always grows the
wrong direction (next you want migration ordering checks, dry-run,
status commands — that's goose).

## Migration plan (when triggered)

1. Add `github.com/pressly/goose/v3` to `go.mod`.
2. Rewrite `internal/store/migrate.go` to call `goose.Up(db, "migrations")`
   with `SetBaseFS(migrationsFS)` and `SetTableName("goose_db_version")`.
3. One-time bootstrap migration: detect the legacy `schema_migrations`
   table, copy its rows into `goose_db_version`, drop it.
4. Convert one existing migration's `down` to a no-op
   (`-- +goose Down\nSELECT 1;`) to confirm the syntax round-trips.
5. New migrations from this point on must include a real `down` unless
   the change is destructive (DROP COLUMN — those can be `-- noop`).

## Risk

- **Bootstrap step is one-shot.** If it runs twice, the second run sees
  `goose_db_version` already populated and tries to re-apply legacy
  migrations. Guard with `IF NOT EXISTS` and a count check.
- **`writeMu` discipline.** Go migrations that mutate data must
  acquire `Store.writeMu` exactly the same way repo methods do today.
  Document this in the goose migration template.

## Decision (if any)

Recommended at M2; not yet triggered.

## References

- `internal/store/migrate.go` — the runner this replaces
- `docs/operations/02_migrations.md` — the doc that explains why and how
- `docs/operations/00_overview.md` ceiling #4 — the `fs.FS` invariant
