package chat_test

import (
	"sync"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
)

// fakeSender is a chat.Sender that records every message for assertion.
type fakeSender struct {
	mu     sync.Mutex
	got    []tea.Msg
	id     int // for distinguishing "雙開" sessions
}

func newFake(id int) *fakeSender { return &fakeSender{id: id} }

func (f *fakeSender) Send(msg tea.Msg) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, msg)
}

func (f *fakeSender) Got() []tea.Msg {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tea.Msg, len(f.got))
	copy(out, f.got)
	return out
}

func TestBroker_RegisterAppearsOnline(t *testing.T) {
	b := chat.NewBroker()
	s := newFake(1)
	b.Register(&chat.Session{UserID: 1, UserIDStr: "alice", Program: s})

	online := b.OnlineList()
	if len(online) != 1 {
		t.Fatalf("got %d online, want 1", len(online))
	}
	if online[0].UserIDStr != "alice" || online[0].Sessions != 1 {
		t.Errorf("online[0] = %+v", online[0])
	}
}

func TestBroker_UnregisterRemoves(t *testing.T) {
	b := chat.NewBroker()
	s := newFake(1)
	b.Register(&chat.Session{UserID: 1, UserIDStr: "alice", Program: s})
	b.Unregister(1, s)
	if len(b.OnlineList()) != 0 {
		t.Errorf("OnlineList not empty after Unregister")
	}
}

// 雙開: same user, two sessions. Send should reach both. Unregister one
// must keep the other online.
func TestBroker_MultipleSessionsSameUser(t *testing.T) {
	b := chat.NewBroker()
	s1 := newFake(1)
	s2 := newFake(2)
	b.Register(&chat.Session{UserID: 1, UserIDStr: "alice", Program: s1})
	b.Register(&chat.Session{UserID: 1, UserIDStr: "alice", Program: s2})

	online := b.OnlineList()
	if len(online) != 1 {
		t.Fatalf("OnlineList: got %d entries, want 1 (one user, two sessions)", len(online))
	}
	if online[0].Sessions != 2 {
		t.Errorf("Sessions = %d, want 2", online[0].Sessions)
	}

	if !b.Send(1, "hello") {
		t.Error("Send returned false but recipient is online")
	}
	for i, s := range []*fakeSender{s1, s2} {
		if g := s.Got(); len(g) != 1 || g[0] != "hello" {
			t.Errorf("session %d got %v, want [\"hello\"]", i, g)
		}
	}

	// Drop s1; s2 must remain.
	b.Unregister(1, s1)
	if !b.Send(1, "again") {
		t.Error("Send returned false; s2 should still be online")
	}
	if len(s1.Got()) != 1 {
		t.Errorf("s1 received message after Unregister; len=%d", len(s1.Got()))
	}
	if len(s2.Got()) != 2 {
		t.Errorf("s2.Got = %v, want length 2", s2.Got())
	}
}

func TestBroker_SendToOfflineReturnsFalse(t *testing.T) {
	b := chat.NewBroker()
	if b.Send(99, "x") {
		t.Error("Send to nobody returned true")
	}
}

func TestBroker_SendToAllExcludesSender(t *testing.T) {
	b := chat.NewBroker()
	alice := newFake(1)
	bob := newFake(2)
	b.Register(&chat.Session{UserID: 1, UserIDStr: "alice", Program: alice})
	b.Register(&chat.Session{UserID: 2, UserIDStr: "bob", Program: bob})

	b.SendToAll(1, "broadcast")

	if g := alice.Got(); len(g) != 0 {
		t.Errorf("alice (sender) received broadcast: %v", g)
	}
	if g := bob.Got(); len(g) != 1 || g[0] != "broadcast" {
		t.Errorf("bob got %v, want [\"broadcast\"]", g)
	}
}

func TestBroker_LookupByUserID(t *testing.T) {
	b := chat.NewBroker()
	b.Register(&chat.Session{UserID: 42, UserIDStr: "alice", Program: newFake(1)})
	if got := b.LookupByUserID("alice"); got != 42 {
		t.Errorf("LookupByUserID(alice) = %d, want 42", got)
	}
	if got := b.LookupByUserID("ALICE"); got != 42 {
		t.Errorf("LookupByUserID(ALICE) (case-insensitive) = %d, want 42", got)
	}
	if got := b.LookupByUserID("ghost"); got != 0 {
		t.Errorf("LookupByUserID(ghost) = %d, want 0", got)
	}
}

// Concurrency stress: many Register / Unregister / Send running together.
// Run with `go test -race` to surface data races.
func TestBroker_Concurrency(t *testing.T) {
	b := chat.NewBroker()
	const workers = 16
	const iters = 100

	var sent atomic.Int64

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			s := newFake(int(id))
			for i := 0; i < iters; i++ {
				b.Register(&chat.Session{UserID: id, UserIDStr: "u", Program: s})
				b.Send(id, "x")
				sent.Add(1)
				b.Unregister(id, s)
			}
		}(int64(w + 1))
	}
	wg.Wait()

	if got := len(b.OnlineList()); got != 0 {
		t.Errorf("OnlineList not empty after all unregisters: %d", got)
	}
	if sent.Load() != int64(workers*iters) {
		t.Errorf("sent = %d, want %d", sent.Load(), workers*iters)
	}
}
