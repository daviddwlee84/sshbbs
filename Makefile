BIN      := sshbbs
PKG      := ./cmd/sshbbs
ADDR     := :2222
DB       := data/bbs.db
MIGDIR   := internal/store/migrations
HOSTKEY  := .ssh/host_ed25519

.PHONY: build run watch hostkey db-reset test test-race cover demo tidy fmt vet clean compose-up compose-down docker-build

build:
	go build -o $(BIN) $(PKG)

run: hostkey
	go run $(PKG) -addr=$(ADDR) -db=$(DB) -hostkey=$(HOSTKEY)

watch: hostkey
	@mkdir -p tmp
	go tool air -c .air.toml

hostkey:
	@if [ ! -f $(HOSTKEY) ]; then \
		./scripts/gen-hostkey.sh $(HOSTKEY); \
	else \
		echo "host key already exists at $(HOSTKEY)"; \
	fi

db-reset:
	rm -f $(DB) $(DB)-wal $(DB)-shm $(DB)-journal
	@echo "removed $(DB) and journal files"

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -cover ./...

demo:
	./scripts/record-demo.sh

tidy:
	go mod tidy

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -f $(BIN)
	rm -rf dist/

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down

docker-build:
	docker build -t sshbbs:dev .
