// Package notify is the per-user webhook fan-out for BBS events
// (push / 水球 / mail / Re:). The dispatcher reads each user's prefs +
// enabled targets from the store and POSTs `{title, body}` JSON to every
// target — compatible with caronc/apprise-api's /notify/<key> endpoint
// and any other simple webhook receiver (Discord, Slack, ntfy.sh, etc.).
//
// The BBS deliberately does NOT parse apprise:// URLs or vendor an apprise
// library. That choice keeps the BBS dependency-light and lets users plug
// in any webhook stack. See docs/notifications.md for the architectural
// rationale.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/charmbracelet/log"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

// Kind names the four event types user_notif_prefs gates on.
type Kind string

const (
	KindPush  Kind = "push"  // someone pushed/booed/arrowed my article
	KindWB    Kind = "wb"    // someone sent me a 水球
	KindMail  Kind = "mail"  // someone sent me a 站內信
	KindReply Kind = "reply" // someone posted a Re: reply targeting my article
)

// Event is one notification request handed to Dispatch. Title and Body are
// pre-formatted display strings — the dispatcher does not own message
// formatting; call sites do, so the user sees consistent text whether the
// event lands as a 水球 toast or a webhook delivery.
type Event struct {
	Kind       Kind
	ToUserID   int64
	FromUserID string // for logging only
	Title      string
	Body       string
}

// OnlineChecker is the slice of *chat.Broker the dispatcher needs for
// the only_when_offline pref. Decoupled so tests can substitute a fake.
type OnlineChecker interface {
	IsOnline(uid int64) bool
}

// Manager is the dispatcher. Construct with New, start workers with Start,
// and call Dispatch from event call sites. Dispatch is non-blocking — a
// full queue drops the event and logs (we never want notification I/O to
// stall an Update path).
type Manager struct {
	store  *store.Store
	online OnlineChecker

	queue   chan Event
	client  *http.Client
	workers int

	wg     sync.WaitGroup
	stopMu sync.Mutex
	closed bool
}

// payload is the JSON body POSTed to every target. Matches apprise-api's
// /notify/<key> contract; non-apprise webhook receivers either consume
// these fields directly (most tooling does) or template against them.
type payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

const (
	defaultQueue   = 256
	defaultWorkers = 4
	defaultTimeout = 5 * time.Second
)

// New builds a Manager. online may be nil for tests that don't care about
// the only_when_offline pref.
func New(st *store.Store, online OnlineChecker) *Manager {
	return &Manager{
		store:   st,
		online:  online,
		queue:   make(chan Event, defaultQueue),
		client:  &http.Client{Timeout: defaultTimeout},
		workers: defaultWorkers,
	}
}

// Start spawns N worker goroutines that read from the queue and POST
// notifications until ctx is cancelled. Returns immediately. Call Stop()
// from the server's shutdown path to drain in-flight events.
func (m *Manager) Start(ctx context.Context) {
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker(ctx)
	}
}

// Stop closes the queue and waits for workers to drain. Safe to call
// multiple times. Does NOT cancel in-flight HTTP requests — the http.Client
// timeout is the upper bound on how long Stop blocks per worker.
func (m *Manager) Stop() {
	m.stopMu.Lock()
	if m.closed {
		m.stopMu.Unlock()
		return
	}
	m.closed = true
	close(m.queue)
	m.stopMu.Unlock()
	m.wg.Wait()
}

// Dispatch enqueues an event for asynchronous delivery. Non-blocking: on a
// full queue (slow webhook upstream + many concurrent events) we drop the
// event and log a warning. We deliberately prefer drop-with-log over
// blocking the caller because the call sites are inside bubbletea Update
// paths — blocking there would freeze the SSH session.
func (m *Manager) Dispatch(ev Event) {
	m.stopMu.Lock()
	closed := m.closed
	m.stopMu.Unlock()
	if closed {
		return
	}
	select {
	case m.queue <- ev:
	default:
		log.Warn("notify: queue full, dropping event",
			"kind", ev.Kind, "to", ev.ToUserID, "from", ev.FromUserID)
	}
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-m.queue:
			if !ok {
				return
			}
			m.deliver(ctx, ev)
		}
	}
}

func (m *Manager) deliver(ctx context.Context, ev Event) {
	prefs, err := m.store.Notify().GetPrefs(ctx, ev.ToUserID)
	if err != nil {
		log.Warn("notify: load prefs", "to", ev.ToUserID, "err", err)
		return
	}
	if !prefsAllow(prefs, ev.Kind) {
		return
	}
	if prefs.OnlyWhenOffline && m.online != nil && m.online.IsOnline(ev.ToUserID) {
		return
	}
	targets, err := m.store.Notify().ListEnabledTargets(ctx, ev.ToUserID)
	if err != nil {
		log.Warn("notify: load targets", "to", ev.ToUserID, "err", err)
		return
	}
	if len(targets) == 0 {
		return
	}
	body, err := json.Marshal(payload{Title: ev.Title, Body: ev.Body})
	if err != nil {
		log.Warn("notify: marshal", "err", err)
		return
	}
	for _, t := range targets {
		m.post(ctx, t, body, ev)
	}
}

func (m *Manager) post(ctx context.Context, t *store.NotifyTarget, body []byte, ev Event) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		log.Warn("notify: build request", "target", t.ID, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sshbbs-notify/1")
	resp, err := m.client.Do(req)
	if err != nil {
		log.Warn("notify: post failed",
			"target", t.ID, "url", t.URL, "kind", ev.Kind, "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Warn("notify: target rejected",
			"target", t.ID, "url", t.URL, "status", resp.StatusCode, "kind", ev.Kind)
	}
}

func prefsAllow(p store.NotifyPrefs, k Kind) bool {
	switch k {
	case KindPush:
		return p.OnPush
	case KindWB:
		return p.OnWB
	case KindMail:
		return p.OnMail
	case KindReply:
		return p.OnReply
	}
	return false
}
