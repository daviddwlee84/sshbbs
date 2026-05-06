package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"

	"github.com/daviddwlee84/sshbbs/internal/auth"
	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/seed"
	"github.com/daviddwlee84/sshbbs/internal/server"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/version"
)

func main() {
	// Subcommand dispatch: anything that isn't a flag is treated as a
	// subcommand. Today only `import` exists, but the shape leaves room
	// for future ops tools (e.g. `export`, `dump`).
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") {
		switch os.Args[1] {
		case "import":
			os.Exit(runImport(os.Args[2:]))
		default:
			log.Fatalf("unknown subcommand: %s", os.Args[1])
		}
	}

	addr := flag.String("addr", ":2222", "SSH listen address")
	dbPath := flag.String("db", "data/bbs.db", "SQLite database path")
	hostkey := flag.String("hostkey", ".ssh/host_ed25519", "SSH host key path")
	adminPassword := flag.String("admin-password", "",
		"override admin password on first seed; empty uses the baked default 'admin' with must_change_password=1")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	if err := st.Boards().SeedDefaults(context.Background()); err != nil {
		log.Fatalf("seed boards: %v", err)
	}
	if err := auth.SeedSystemAccounts(context.Background(), st, auth.SeedOpts{
		AdminPassword: *adminPassword,
	}); err != nil {
		log.Fatalf("seed system accounts: %v", err)
	}
	if err := seed.Articles(context.Background(), st, auth.ReservedUsernameAdmin); err != nil {
		log.Fatalf("seed articles: %v", err)
	}
	if err := seed.Banners(context.Background(), st, auth.ReservedUsernameAdmin); err != nil {
		log.Fatalf("seed banners: %v", err)
	}
	log.Printf("storage ready at %s", *dbPath)

	broker := chat.NewBroker()

	srv, err := server.New(server.Config{Addr: *addr, HostKey: *hostkey}, st, broker)
	if err != nil {
		log.Fatalf("server.New: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("sshbbs %s listening on %s", version.Version, *addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	case <-ctx.Done():
		log.Println("shutdown signal received")
		shutdown(srv, broker)
	}
}

// shutdown closes the listener (refuses new connections), then asks every
// live bubbletea program to quit, then closes the SSH server hard. The DB
// closes on the deferred st.Close() in main.
func shutdown(srv *ssh.Server, broker *chat.Broker) {
	const drainTimeout = 3 * time.Second

	// 1. Stop accepting new connections.
	listenerCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	if err := srv.Shutdown(listenerCtx); err != nil {
		log.Printf("server.Shutdown: %v", err)
	}

	// 2. Tell every active session to quit.
	sessions := broker.SessionsSnapshot()
	if len(sessions) == 0 {
		return
	}
	log.Printf("draining %d active session(s)", len(sessions))
	for _, s := range sessions {
		s.Program.Send(tea.Quit())
	}

	// 3. Give them a moment to flush their final view, then force-close.
	timer := time.NewTimer(drainTimeout)
	defer timer.Stop()
	for {
		if len(broker.SessionsSnapshot()) == 0 {
			log.Println("all sessions drained")
			return
		}
		select {
		case <-timer.C:
			log.Printf("drain timeout; %d session(s) still attached", len(broker.SessionsSnapshot()))
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}
