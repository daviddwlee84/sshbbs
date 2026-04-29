# 01 · Environments — dev / stage / prod separation

Today the only "environment" knob is which database file you point the
binary at. That's enough for M0–M1. This doc names what changes per env
as the project climbs the ladder, and explains why CLI-flag-only config
(`cmd/sshbbs/main.go:22-26`) hits its limit at M2.

## What changes per environment

| Knob | dev | stage | prod |
|---|---|---|---|
| DB path | `data/dev.db` | `/srv/sshbbs/stage/bbs.db` | `/srv/sshbbs/prod/bbs.db` |
| Host key | `.ssh/host_ed25519` (ephemeral) | vaulted, persistent | vaulted, persistent |
| Listen addr | `:2222` | `:2222` (behind LB) | `:22` direct, or `:2222` behind LB |
| `bcrypt` cost | 12 (default) | 12 | 10 if cold-start CPU is an issue (M3) |
| Log level / format | text, debug | json, info | json, info |
| Seed default boards | yes | one-shot, then off | one-shot, then off |
| `SQLITE_BUSY` retry | fast-fail OK | retry once | retry with backoff |

The current code seeds default boards on every boot — fine for dev,
mildly wasteful for prod (the writes are no-ops on a populated DB but
still acquire the write mutex). Worth gating on a `--seed-defaults=false`
flag at M2.

## Config progression

### M0–M1 — CLI flags only (today)

```
go run ./cmd/sshbbs -addr=:2222 -db=data/dev.db -hostkey=.ssh/host_ed25519
```

Three flags handle every knob a single developer needs. Add a filename
split (`data/dev.db` vs `data/prod.db`) at M1; no code change required.

### M2 — env vars override flags (additive)

When a second engineer joins or you want to deploy via `docker compose`
without baking flags into the image, env vars are the natural extension:

```
SSHBBS_ADDR=:2222   SSHBBS_DB=/data/bbs.db   SSHBBS_HOSTKEY=/keys/host_ed25519   sshbbs
```

Implementation: in `cmd/sshbbs/main.go:22`, replace each `flag.String(...)`
default with `firstNonEmpty(os.Getenv("SSHBBS_X"), defaultX)`. The flag
still wins if explicitly set — so `make run` keeps working unchanged.

### M3 — config file + secret store

A single TOML or YAML file per env, checked into an infra repo, with
secrets pulled from a real KMS at boot (not embedded in the file):

```toml
# config/prod.toml
addr         = ":22"
db           = "/srv/sshbbs/prod/bbs.db"
hostkey_path = "/keys/host_ed25519"   # populated by infra job at boot
log_level    = "info"
log_format   = "json"
bcrypt_cost  = 10
```

The flag/env interface stays as a per-invocation override. New knobs
(log format, bcrypt cost) get added to the config struct, not the flag list.

## Why CLI flags hit a wall at M2

- **Secrets**: any flag value lands in `ps aux` output. A hostkey
  passphrase or DB password belongs in env vars (visible only to the
  process) or a file (read once, cleared from memory).
- **Combinatorial explosion**: per-env override sets become an N×M
  matrix that's painful to script. `--prod` / `--stage` macro flags
  push the problem into code instead of solving it.
- **Reload**: env vars + a SIGHUP handler can reload non-secret config
  without a restart. Flags can't.

## What the host key really is

It's a long-lived secret that defines the server's SSH identity to
clients. If it changes, every connecting client sees `Host key
verification failed` and refuses to log in until they `ssh-keygen -R` it.
So:

- **dev**: `.ssh/host_ed25519`, gitignored, regenerable any time.
- **stage / prod**: persisted to a volume, populated from a vault on
  first boot, *never* committed.

The `docker-compose.yml` `hostkey-init` service generates one only if
the volume is empty — see `04_docker.md`.

## Cross-references

- `cmd/sshbbs/main.go:22-26` — the three flags this doc proposes evolving
- `internal/auth/auth.go:20` — `bcryptCost` constant; reduce in prod at M3 if needed
- `internal/store/store.go` — DB connection string (`_journal_mode=WAL` etc.)
- `04_docker.md` — how env vars feed into the compose service definition
