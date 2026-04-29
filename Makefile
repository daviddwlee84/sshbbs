BIN      := sshbbs
PKG      := ./cmd/sshbbs
ADDR     := :2222
DB       := data/bbs.db
MIGDIR   := internal/store/migrations
HOSTKEY  := .ssh/host_ed25519

.PHONY: build run hostkey db-reset test tidy fmt vet clean

build:
	go build -o $(BIN) $(PKG)

run: hostkey
	go run $(PKG) -addr=$(ADDR) -db=$(DB) -hostkey=$(HOSTKEY)

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
