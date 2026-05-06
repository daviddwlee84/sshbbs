// bbsmoke is a one-shot end-to-end check that the notify dispatcher's
// HTTP path actually reaches a running caronc/apprise-api sidecar and
// gets fanned out the configured downstream URL.
//
// Usage (with the docker-compose.example.yml apprise sidecar already up):
//
//   curl -X POST http://localhost:8000/add/smoketest \
//     -H 'Content-Type: application/json' \
//     -d '{"urls":"json://host.docker.internal:9999"}'
//   go run ./cmd/bbsmoke
//
// Run from any directory in this module. Exits 0 on round-trip success,
// non-zero on timeout. Prints the JSON body the sink received.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/notify"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

func main() {
	const sinkAddr = "0.0.0.0:9999"
	const appriseURL = "http://localhost:8000/notify/smoketest"

	received := make(chan []byte, 4)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		select {
		case received <- body:
		default:
		}
		w.WriteHeader(http.StatusOK)
	})
	ln, err := net.Listen("tcp", sinkAddr)
	if err != nil {
		fatal("listen %s: %v", sinkAddr, err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	fmt.Printf("[sink] listening on %s\n", sinkAddr)

	dir, err := os.MkdirTemp("", "bbsmoke-")
	if err != nil {
		fatal("mktemp: %v", err)
	}
	defer os.RemoveAll(dir)
	st, err := store.Open(filepath.Join(dir, "smoke.db"))
	if err != nil {
		fatal("store.Open: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	alice, err := st.Users().Create(ctx, "alice",
		"$2a$12$placeholderplaceholderplaceholderplaceholderplaceholder.", "", "")
	if err != nil {
		fatal("create user: %v", err)
	}
	tid, err := st.Notify().AddTarget(ctx, alice.ID, "apprise-smoke", appriseURL)
	if err != nil {
		fatal("AddTarget: %v", err)
	}
	fmt.Printf("[db] alice id=%d, target id=%d → %s\n", alice.ID, tid, appriseURL)

	broker := chat.NewBroker()
	mgr := notify.New(st, broker)
	mctx, cancel := context.WithCancel(ctx)
	defer cancel()
	mgr.Start(mctx)
	defer mgr.Stop()

	ev := notify.Event{
		Kind:       notify.KindPush,
		ToUserID:   alice.ID,
		FromUserID: "bob",
		Title:      "[BBS-smoke] bob 推了你的文章 «Hello»",
		Body:       "→ 推 nice post (round-trip test)",
	}
	mgr.Dispatch(ev)
	fmt.Printf("[dispatch] kind=%s to=%d from=%s\n", ev.Kind, ev.ToUserID, ev.FromUserID)

	select {
	case body := <-received:
		fmt.Printf("[sink] received %d bytes:\n%s\n", len(body), string(body))
		fmt.Println("OK")
	case <-time.After(8 * time.Second):
		fmt.Println("TIMEOUT — sink got nothing in 8s")
		os.Exit(1)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "fatal: "+format+"\n", args...)
	os.Exit(2)
}
