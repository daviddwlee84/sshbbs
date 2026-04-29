# 04 · Docker — multi-stage build + one-command compose

The project ships a `Dockerfile` and `docker-compose.yml` even at M0
because the marginal cost is low (pure-Go SQLite means no cgo, so the
build is trivial and the final image is ~5 MB) and the payoff — a
reproducible local-dev environment and a clean artifact for the
eventual CI/CD step at M2 — is high.

This doc walks through the shipped files and explains the choices.

## The Dockerfile

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

### Why these choices

- **`golang:1.23-alpine`** for the builder: smaller download than the
  Debian variant; alpine is fine for build because the artifact is a
  static binary that goes into the final stage.
- **`CGO_ENABLED=0`**: `modernc.org/sqlite` is pure Go. Static binary
  works in any final image — no glibc, no `ld-musl` headaches.
- **`-trimpath`**: removes local filesystem paths from binary error
  output. Reproducible builds.
- **`-ldflags="-s -w"`**: strips debug info (~30% smaller binary).
- **`-X .../version.Version=${VERSION}`**: the existing `internal/version`
  package picks this up; pass via `--build-arg` in CI.
- **`gcr.io/distroless/static-debian12:nonroot`** over `scratch`:
  ~2 MB larger but provides `/etc/passwd` (so `USER nonroot` works) and
  CA certs (for any future outbound TLS, e.g. Prometheus push). The
  `:nonroot` tag bakes in UID 65532 — no user setup needed.
- **`VOLUME ["/data", "/keys"]`**: declares the persistent paths so
  `docker run` warns if you bind-mount one but not the other.

## The docker-compose.yml

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

### Why the init service

The host key is an SSH identity. If it changes between runs, every
client that connected before sees `Host key verification failed` and
refuses to log in until they `ssh-keygen -R` it. So we want it
**generated once and persisted** — not regenerated on every container
boot.

The `hostkey-init` service runs *only on first boot* (the `if [ ! -f ]`
check skips it once the key exists), generates an ed25519 key into the
named volume, and exits. The main `sshbbs` service `depends_on` it
with `condition: service_completed_successfully`, so the boot order is
guaranteed.

This replaces the current `make hostkey` step for contributors who
prefer Docker.

## Volume driver warning

**Do not put `bbs-data` on NFS.** SQLite filesystem locks rely on POSIX
`flock`/`fcntl` semantics that NFS implementations honour
inconsistently. A corrupted WAL is a much worse failure than a
3-second restart blip. Some Docker volume drivers (vboxsf, certain
CSI plugins) have similar issues. Stick to the local `bbs-data` named
volume in dev; in prod use a real local disk or AWS EBS / GCP
persistent disk.

## Make targets

Three new targets in the `Makefile`:

```
make compose-up       # docker compose up --build -d
make compose-down     # docker compose down
make docker-build     # docker build -t sshbbs:dev .
```

`make compose-up` is the new "first run" command for contributors who
don't want to set up Go locally. The bare-metal `make run` flow still
works unchanged.

## Cross-references

- `cmd/sshbbs/main.go:22-26` — the three flags the ENTRYPOINT pins
- `01_environments.md` — env-var migration that will eventually replace those flags
- `00_overview.md` ceiling #1 — the SQLite-on-NFS warning, also flagged here
- `Dockerfile`, `docker-compose.yml`, `.dockerignore` — the actual artifacts
