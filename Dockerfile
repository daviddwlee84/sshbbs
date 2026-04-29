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
