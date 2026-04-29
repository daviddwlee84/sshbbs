# Plan: `docs/operations/` + Dockerfile + Milestone-Gated TODO Entries

## Context

The project is in MVP/prototype phase — DB schema is "wild west", config is
CLI-flags-only, no Docker, no CI. The user wants reference documentation for
*when and how* to graduate to more formal practices (reversible migrations,
dev/stage/prod separation, zero-downtime deploys) so future-them or a future
collaborator can recognize the trigger and execute without re-researching.

The deliverable has three parts:

1. **`docs/operations/`** — five English markdown files mirroring the
   `docs/ptt_trace_code/` style (30-80 lines each, numbered, code-cross-referenced).
2. **Dockerfile + `docker-compose.yml`** — a multi-stage build + a one-command
   local-dev compose that handles host-key generation and volume persistence.
   Cheap because `modernc.org/sqlite` is pure Go (no cgo).
3. **Milestone-gated TODO.md entries + backlog docs** — six entries surface
   the future-work ladder so it's visible from the index, not just the docs.

Architectural ceiling that shapes everything: **SQLite WAL forbids two
concurrent writers on the same DB file**. Combined with the in-memory
`chat.Broker` (`sessions map[int64][]*Session`), this means *true*
zero-downtime deploys require either (a) a single-writer architecture forever,
or (b) Postgres at the M4 milestone. The docs name this constraint up front.

---

## File layout

New files (all created in this batch):

```
docs/operations/
  00_overview.md         # index + the M0→M4 milestone ladder + the architectural ceiling
  01_environments.md     # dev/stage/prod separation; CLI flags → env vars → config file
  02_migrations.md       # forward-only → reversible (goose) → expand/migrate/contract
  03_deployment.md       # graceful shutdown today + zero-downtime escape hatches
  04_docker.md           # the shipped Dockerfile + compose.yml explained

Dockerfile                # multi-stage: golang:1.23-alpine → distroless/static-debian12:nonroot
docker-compose.yml        # service + named volumes (bbs-data, bbs-keys) + init service for hostkey
.dockerignore             # exclude data/, .ssh/, .git/, .specstory/, .claude/, etc.

backlog/
  goose-migration-switch.md         # design notes for the M2 goose migration
  external-chat-broker.md           # spike for P?/L Redis pub/sub broker
  postgres-migration-plan.md        # spike for P?/XL Postgres at M4
```

Files modified:

- `TODO.md` — six new entries via `scripts/add-todo.sh` (validates on insert)
- `Makefile` — three new targets: `compose-up`, `compose-down`, `docker-build`
- `README.md` — add a "Local dev with Docker" subsection pointing at compose

Files **not** modified (the docs-only contract): no Go source, no migrations,
no `cmd/sshbbs/main.go`. The docs describe future changes; the code stays put.

---

## The five docs (content sketch)

### `00_overview.md` — index + milestone ladder

Opens with the same "What this is / what we mimic" framing as
`docs/ptt_trace_code/00_overview.md:1-7`. Two tables:

**Table 1 — milestone ladder.** Single trigger per row (the bump rule):

| Lvl | Trigger to enter | What becomes mandatory |
|---|---|---|
| **M0** Solo prototype | now | Forward-only migrations, CLI-flag config, `make db-reset` is fine, no Docker required |
| **M1** Closed beta | first non-author user has an account you don't want to lose | Daily SQLite backup (cron + `.backup`), separate `data/dev.db` vs `data/prod.db`, host key in vault, **Dockerfile + compose** for reproducible local |
| **M2** Public beta | second engineer makes a commit, OR 10 concurrent SSH users | Reversible migrations (goose), env-var config, CI on PR (`go test -race` + `vet` + `staticcheck`), tagged releases, `slog` (P2 in `TODO.md`) |
| **M3** Production | first real outage, OR uptime SLA promised | Stage env mirroring prod, blue/green via socket-handoff *or* SSH-proxy, automated restore drill, Prometheus metrics |
| **M4** Scale-out | single-box CPU or write-throughput sustained at 70% | Postgres migration, external broker (Redis), horizontal SSH frontends — only if M3 is no longer enough |

**Table 2 — architectural ceilings to flag now**:

1. SQLite WAL forbids two writers → blue/green needs single-writer architecture or Postgres
2. In-memory `chat.Broker` → restart drops live presence (water balloons & mail are persistent, so survive)
3. bcrypt cost-12 → CPU spike on cold start; M3 health-check storms can burn cores
4. `go:embed migrations/*.sql` → migration runner switch must keep the `fs.FS` interface

### `01_environments.md` — dev / stage / prod separation

What changes per env: DB path, hostkey path, listen addr, bcrypt cost, log
level, seed-data behavior. Why CLI flags hit their limit at M2: secret
management (you can't put a hostkey passphrase on a flag without it landing in
`ps`), per-env override sets become an N×M matrix. The progression:

1. **M0–M1**: CLI flags + `data/dev.db` vs `data/prod.db` filename split
2. **M2**: env-vars override flags (`SSHBBS_DB`, `SSHBBS_ADDR`, etc.) — additive,
   doesn't break the flag interface. Touches `cmd/sshbbs/main.go:22-26`.
3. **M3**: per-env config file (TOML/YAML) checked into infra repo, secrets
   from a real KMS (not the config). The flag/env interface stays as override.

References `cmd/sshbbs/main.go:22-26` (config surface) and
`internal/auth/auth.go:20` (`bcryptCost = 12` — note in §M3 it can drop to 10
without invalidating existing hashes since bcrypt embeds cost).

### `02_migrations.md` — forward-only → reversible → expand/migrate/contract

Section 1 documents the current mechanism (`internal/store/migrate.go:13-103`):
embedded `migrations/NNNN_name.sql`, `schema_migrations` table tracks version,
each migration runs in `tx.BeginTx`. Forward-only is *correct* for M0/M1.

Section 2 — when reversibility becomes mandatory (M2 entry trigger). Comparison
table:

| Tool | Pros | Cons | Verdict |
|---|---|---|---|
| `golang-migrate/migrate` | most popular | Go migrations are second-class; heavier dep | skip |
| `pressly/goose` | native Go func migrations participate in `Store.writeMu`; `fs.FS` support keeps `go:embed`; smaller dep | smaller community | **recommended at M2** |
| `ariga/atlas` | declarative schema diff | different working style; overkill for 6 tables | skip unless going schema-as-code |

Section 3 — the **expand / migrate / contract** pattern, mandatory from M2:

1. **Expand**: additive migration (add nullable column, new table). Old + new
   binary both work.
2. **Migrate**: backfill data; deploy a release that dual-writes/dual-reads.
3. **Contract**: drop old column/table once all instances are on the new code.

Worked example: renaming `articles.recommend_score` → `articles.score_cached`.
Three migrations, three deploys. Each individual deploy is backward-compatible
with the previous binary.

### `03_deployment.md` — graceful shutdown today + zero-downtime options

Section 1 documents what already works (`cmd/sshbbs/main.go:65-103`): SIGTERM
→ refuse new connections (`srv.Shutdown`) → `program.Send(tea.Quit())` to
every live bubbletea program → poll `broker.SessionsSnapshot()` with 3s
timeout → DB close. Real-world impact is sub-second for an empty broker,
capped at 3s for an active one. **For M0–M2, this is enough — accept a 3s
maintenance blip.**

Section 2 — why "true" zero-downtime is architecturally blocked here:
SQLite-two-writers + in-memory broker. Three escape hatches in increasing
cost (recommend the cheapest that fits the milestone):

- **(a) Socket-handoff via `SO_REUSEPORT`** (M3): old binary keeps existing
  sessions, new binary takes new connections, old binary exits when last
  session closes. Single SQLite writer at a time.
- **(b) External broker (Redis pub/sub)** (M3): broker becomes stateless;
  new binary inherits presence from Redis. Still single SQLite writer.
- **(c) Postgres + horizontal stateless servers fronted by SSH-aware LB** (M4):
  the only path for *true* concurrent writers. Big rewrite.

Recommendation: **stay at M2's 30-second-maintenance-window tier indefinitely
unless §00 architectural ceiling forces escalation.**

### `04_docker.md` — the shipped Dockerfile + compose explained

Walks through the actual files committed in this PR (so the doc and the
artifact stay in sync). Key shape decisions:

- **distroless/static-debian12:nonroot** over scratch: `~2MB` extra for free
  CA certs and `/etc/passwd` (lets `USER nonroot` work). Worth it.
- **`CGO_ENABLED=0`** in builder: pure-Go SQLite makes this trivial; static
  binary works in any final image.
- **Two named volumes**: `bbs-data` for `/data` (the SQLite WAL files),
  `bbs-keys` for `/keys` (host key persists across container restarts).
- **Init service in compose**: runs `gen-hostkey.sh` only if
  `/keys/host_ed25519` is missing — first `docker compose up` Just Works.
- **Volume driver warning**: do **not** put `bbs-data` on NFS or some Docker
  volume drivers — SQLite filesystem locks aren't honoured everywhere, and
  a corrupt WAL is much worse than a 3s maintenance window.

`make compose-up` becomes the new "first run" command for contributors,
replacing the README's current 3-step `make hostkey && make run` setup.

---

## The Dockerfile + compose (final shape, not just sketch)

### `Dockerfile`

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -X github.com/daviddwlee84/sshbbs/internal/version.Version=${VERSION}" \
    -o /out/sshbbs ./cmd/sshbbs

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/sshbbs /sshbbs
USER nonroot
EXPOSE 2222
VOLUME ["/data", "/keys"]
ENTRYPOINT ["/sshbbs", "-addr=:2222", "-db=/data/bbs.db", "-hostkey=/keys/host_ed25519"]
```

### `docker-compose.yml`

```yaml
services:
  hostkey-init:
    image: alpine:3.20
    volumes:
      - bbs-keys:/keys
    entrypoint: ["/bin/sh", "-c"]
    command:
      - |
        if [ ! -f /keys/host_ed25519 ]; then
          apk add --no-cache openssh-keygen >/dev/null
          ssh-keygen -t ed25519 -f /keys/host_ed25519 -N "" -q
          chmod 600 /keys/host_ed25519
        fi
  sshbbs:
    build: .
    depends_on:
      hostkey-init:
        condition: service_completed_successfully
    ports:
      - "2222:2222"
    volumes:
      - bbs-data:/data
      - bbs-keys:/keys
    restart: unless-stopped
volumes:
  bbs-data:
  bbs-keys:
```

### `.dockerignore`

```
.git/
.github/
.specstory/
.claude/
data/
.ssh/
sshbbs
*.test
docs/demo.gif
docs/demo.webm
backlog/
pitfalls/
```

### `Makefile` additions

```makefile
compose-up:
	docker compose up --build -d
compose-down:
	docker compose down
docker-build:
	docker build -t sshbbs:dev .
```

---

## TODO.md additions (six entries via `scripts/add-todo.sh`)

Run from project root, in this order so the validator passes after each:

```bash
scripts/add-todo.sh --priority P3 --effort M \
  --title "Multi-stage Dockerfile + docker-compose.yml" \
  --description "Distroless final image; compose with hostkey-init + named volumes for /data and /keys. See docs/operations/04_docker.md."

scripts/add-todo.sh --priority P3 --effort S \
  --title "GitHub Actions CI: test-race + vet + staticcheck on PR" \
  --description "Gates M2. ~40-line workflow. See docs/operations/00_overview.md milestone M2 row."

scripts/add-todo.sh --priority P3 --effort M \
  --title "Migrate config from CLI flags to env vars (M2 prep)" \
  --description "Additive: env vars override flags. Touches cmd/sshbbs/main.go:22-26. See docs/operations/01_environments.md."

scripts/add-todo.sh --priority P3 --effort M --backlog \
  --title "Switch migration runner to pressly/goose" \
  --description "Gated on M2 entry (reversibility need). Keep the fs.FS / go:embed interface. See backlog/goose-migration-switch.md."

scripts/add-todo.sh --priority "P?" --effort L --backlog \
  --title "External chat broker (Redis pub/sub) for zero-downtime" \
  --description "Decouples chat.Broker from process. Spike on whether SO_REUSEPORT socket-handoff is cheaper. See backlog/external-chat-broker.md."

scripts/add-todo.sh --priority "P?" --effort XL --backlog \
  --title "Postgres migration plan" \
  --description "Only if M4 ever triggers. Captures SQLite-WAL → Postgres delta + the two-writer constraint that forces it. See backlog/postgres-migration-plan.md."
```

The `--backlog` flag scaffolds `backlog/<slug>.md` from the template. Three
backlog docs get authored (the three with `--backlog` above) following
`backlog/README.md`'s "options + trade-offs" template.

After all six entries, run `scripts/todo-kanban.sh --validate-only TODO.md`
once to confirm nothing drifted.

---

## Critical files to read / modify

**Read for style + invariants** (no edits):

- `docs/ptt_trace_code/00_overview.md` — table layout + cross-reference style
- `docs/testing.md` — concise English doc cadence to match
- `internal/store/migrate.go:13-103` — current migration runner that
  `02_migrations.md` documents and proposes evolving via goose
- `cmd/sshbbs/main.go:22-103` — config surface + graceful shutdown that
  `01_environments.md` and `03_deployment.md` reference
- `internal/chat/broker.go` — in-memory state that constrains zero-downtime
- `backlog/README.md` — companion-doc template + when-to-add rules
- `TODO.md` lines 30-49 — the P2/P3 schema the new entries must match

**Create**:

- 5 docs in `docs/operations/`
- `Dockerfile`, `docker-compose.yml`, `.dockerignore`
- 3 backlog docs in `backlog/`

**Modify**:

- `TODO.md` (via `scripts/add-todo.sh`, six times)
- `Makefile` (three new targets)
- `README.md` (one new "Local dev with Docker" subsection)

---

## Verification

End-to-end check before declaring done:

1. **Docs render correctly**:
   ```bash
   ls docs/operations/   # five files
   for f in docs/operations/*.md; do head -1 "$f"; done   # all start with "# NN ·"
   ```

2. **TODO validator passes**:
   ```bash
   scripts/todo-kanban.sh --validate-only TODO.md
   ```

3. **Backlog docs follow template**:
   ```bash
   for f in backlog/goose-migration-switch.md backlog/external-chat-broker.md backlog/postgres-migration-plan.md; do
     grep -q '^Status:' "$f" && grep -q '^## Options' "$f" || echo "BAD: $f"
   done
   ```

4. **Docker build works**:
   ```bash
   make docker-build   # builds without errors
   docker images sshbbs:dev   # final image < 20 MB (distroless static)
   ```

5. **Compose round-trip**: in a fresh checkout (or after `docker compose down -v`):
   ```bash
   make compose-up
   sleep 3
   ssh new@localhost -p 2222    # register form appears (proves hostkey + DB volumes work)
   make compose-down
   make compose-up              # second boot reuses the persisted hostkey + DB
   ssh -o StrictHostKeyChecking=accept-new alice@localhost -p 2222   # account from first boot still exists
   make compose-down
   ```

6. **No regressions in current dev workflow**:
   ```bash
   make test-race   # all existing tests still pass
   make run         # bare-metal run still works (no Docker required)
   ```

7. **Existing tests untouched**: `git diff --stat internal/` should show
   zero changes — this is a docs+packaging PR, not a code PR.
