# 00 · Overview — milestones, ceilings, and where to look

These notes describe how this project graduates from "wild-west prototype"
toward something closer to production. They exist so a future reader can
recognise *which milestone they are in* and pick the smallest set of
practices that gates the next bump — without re-researching every
alternative.

## What this is (and isn't)

This is a maturity ladder for a personal-scale BBS, not a platform
playbook. The recommendations shrink the gap between "I can `make run` it"
and "someone else relies on this", in roughly five steps. M0 is where the
project is today (2026-04-29).

For the *why* behind the design (PTT lineage), see `docs/ptt_trace_code/`.
For the testing strategy that runs alongside every milestone, see
`docs/testing.md`.

## The milestone ladder

Single trigger per row — when the trigger fires, you bump.

| Lvl | Trigger to enter | What becomes mandatory |
|---|---|---|
| **M0** Solo prototype | now (2026-04-29) | Forward-only migrations, CLI-flag config, `make db-reset` is fine, no Docker required |
| **M1** Closed beta | first non-author user has an account you don't want to lose | Daily SQLite backup (`.backup` cron), separate `data/dev.db` vs `data/prod.db`, host key persisted in a vault, **Dockerfile + compose** for reproducible local |
| **M2** Public beta | second engineer makes a commit, OR 10 concurrent SSH users | Reversible migrations (`pressly/goose`), env-var config, CI on PR (`go test -race` + `vet` + `staticcheck`), tagged releases, `slog` (P2 in `TODO.md`) |
| **M3** Production | first real outage, OR uptime SLA promised to anyone | Stage env mirroring prod, blue/green via `SO_REUSEPORT` *or* SSH-proxy, automated restore drill, Prometheus metrics |
| **M4** Scale-out | single-box CPU or write throughput sustained at 70% | Postgres migration, external broker (Redis pub/sub), horizontal SSH frontends behind an SSH-aware LB |

Most projects stop at **M2** indefinitely. M3 and M4 are named escape
hatches, not destinations.

## Architectural ceilings to flag now

Four constraints in the current code shape every decision above. Document
them here so they don't get re-discovered the hard way:

1. **SQLite WAL forbids two concurrent writers on one file.** A naive
   blue/green deploy means *both* binaries open `data/bbs.db`; SQLite's
   filesystem locks either flap with `SQLITE_BUSY` or — on NFS / some
   Docker volume drivers — silently corrupt. Either single-writer
   architecture forever (the M3 socket-handoff path), or Postgres at M4.
   Do **not** put the data volume on NFS. See `04_docker.md`.

2. **In-memory `chat.Broker` (`internal/chat/broker.go:36-39`) drops live
   presence on restart.** Persistent state (water balloons, mail) survives
   via the `delivered_live` flag pattern in
   `internal/store/migrations/0003_add_mail.sql`, but the online-list
   flickers across every deploy. Hard ceiling on uptime perception
   until M3.

3. **bcrypt cost-12 (`internal/auth/auth.go:20`) is a CPU spike on cold
   start.** If M3 puts you behind a load balancer with active health
   checks, repeated logins from monitoring will burn cores. Drop to
   cost-10 in prod env if needed; bcrypt embeds the cost, so existing
   hashes stay valid.

4. **`go:embed migrations/*.sql` (`internal/store/migrate.go:13-14`) is
   one-way out of the binary.** The M2 migration-runner switch must
   keep the `fs.FS` interface — running migrations from disk in a
   container is a regression (every deploy needs the SQL files mounted).

## Files in this directory

- `00_overview.md` — this file
- `01_environments.md` — dev / stage / prod separation; CLI flags → env vars → config file
- `02_migrations.md` — forward-only → reversible (goose) → expand/migrate/contract
- `03_deployment.md` — graceful shutdown today; zero-downtime escape hatches
- `04_docker.md` — the shipped Dockerfile + `docker-compose.yml` explained
