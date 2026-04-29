# 02 · Migrations — forward-only today, reversible at M2

The project ships a tiny home-grown migration runner. It's correct for
solo development and stays correct through closed beta. This doc names
the trigger that forces a switch to reversible migrations and the
backward-compatible deploy pattern that comes with them.

## What we have today (M0–M1)

`internal/store/migrate.go:13-103` does the minimum:

- `//go:embed migrations/*.sql` bakes SQL files into the binary
- `schema_migrations(version INTEGER PRIMARY KEY, applied_at DATETIME)`
  tracks applied versions
- Files named `NNNN_name.sql`; the leading integer is the version
- Each migration runs in its own `tx.BeginTx` transaction
- Forward-only: there is no down migration, and `make db-reset` is the rollback

Three migrations exist:

```
internal/store/migrations/
  0001_init.sql           # six core tables
  0002_seed_boards.sql    # default Welcome / Test / ChitChat
  0003_add_mail.sql       # mailbox tables + indexes
```

This works because there is exactly one writer (the dev box), one DB
(`data/bbs.db`), and one consumer of the schema (the same dev). Nothing
is wrong with it; nothing about M0–M1 *needs* down migrations.

## When reversibility becomes mandatory (M2)

Two triggers, either is sufficient:

1. A second engineer commits a migration. Without `down`, you cannot
   roll back their schema change locally to test against the previous
   binary — only `make db-reset` works, and it nukes data.
2. The first prod deploy happens. A bad migration in prod that you
   can't reverse means a corrupted database, not a 30-second outage.

At that point swap the runner. Until then, the home-grown one is fine.

## Tool comparison

| Tool | Pros | Cons | Verdict |
|---|---|---|---|
| `golang-migrate/migrate` | Largest community | Go migrations are second-class; heavier dep tree | Skip |
| `pressly/goose` | Native Go func migrations participate in `Store.writeMu`; `fs.FS` support keeps `go:embed`; small dep | Smaller community than `migrate` | **Recommended at M2** |
| `ariga/atlas` | Declarative schema diff | Different working style; overkill for 6 tables | Skip unless committing to schema-as-code |

`goose` wins because:

- `goose.SetBaseFS(migrationsFS)` is a drop-in replacement for the
  current `embed.FS` lookup. The migration files don't move.
- Go-style migrations (`func upXxx(ctx context.Context, tx *sql.Tx) error`)
  let a data rewrite participate in `Store.writeMu`, which the SQL-only
  `migrate` runner can't.
- `down` is opt-in per file. The first ~3 M2 migrations can keep no-op
  downs without introducing risk.

## Expand / migrate / contract — mandatory from M2

Every schema change that affects an existing column or table goes out
in three deploys. Each individual deploy is backward-compatible with
the previous binary.

**Worked example**: rename `articles.recommend_score` → `articles.score_cached`.

1. **Expand** (migration `0007_add_score_cached.sql`):

   ```sql
   ALTER TABLE articles ADD COLUMN score_cached INTEGER NOT NULL DEFAULT 0;
   UPDATE articles SET score_cached = recommend_score;
   ```

   Old binary still reads `recommend_score` (untouched); new binary
   dual-writes both columns.

2. **Migrate** (no schema change, just a code release):

   App reads `score_cached`, writes both. Backfill any rows added since
   step 1 by old binaries. Wait long enough that no instance is running
   the pre-step-1 binary anymore.

3. **Contract** (migration `0008_drop_recommend_score.sql`):

   ```sql
   ALTER TABLE articles DROP COLUMN recommend_score;
   ```

   Now safe — nothing references it.

The `down` for **Expand** drops the new column; `down` for **Contract**
re-adds the old column (NULL) and you'd have to rebuild the cache. So
even with `goose`, contract migrations are expensive to reverse — that's
expected; the *point* of the three-step pattern is that you almost
never need to.

## What stays the same

The mutex discipline doesn't change. Go migrations that mutate data
must call `Store.writeMu.Lock()` exactly the same way repo methods do
today. The `schema_migrations` table is renamed to `goose_db_version`
but tracks the same thing — a one-shot bootstrap migration copies
existing rows over (see `backlog/goose-migration-switch.md`).

## Cross-references

- `internal/store/migrate.go:13-103` — the runner this doc proposes evolving
- `internal/store/store.go` — `writeMu` (documented in `CLAUDE.md`)
- `00_overview.md` ceiling #4 — the `fs.FS` invariant the switch must preserve
- `backlog/goose-migration-switch.md` — full migration plan for the runner switch
