// Package chat is the in-memory hub that connects live SSH sessions for
// real-time features (water balloons, push broadcasts). It mirrors PTT's
// `userinfo_t` shared-memory table conceptually, but as a process-local
// map[user_id][]*Session — no SHM, no daemons.
package chat

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// Sender is the minimal slice of *tea.Program the broker actually needs.
// Decoupling lets tests substitute a recording fake without spinning up a
// real bubbletea program.
type Sender interface {
	Send(tea.Msg)
}

// Session is one live SSH connection. The same user may have multiple
// Sessions (PTT calls this "雙開"); the broker keeps them all and
// broadcasts to each.
type Session struct {
	UserID    int64
	UserIDStr string
	Program   Sender
}

// OnlineUser is the externally-visible view of a logged-in user.
type OnlineUser struct {
	UserID    int64
	UserIDStr string
	Sessions  int
}

type Broker struct {
	mu       sync.RWMutex
	sessions map[int64][]*Session
}

func NewBroker() *Broker {
	return &Broker{sessions: make(map[int64][]*Session)}
}

func (b *Broker) Register(s *Session) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions[s.UserID] = append(b.sessions[s.UserID], s)
}

// Unregister removes a single session for the given user (matched by
// Sender identity). If it was the last session, the user disappears from
// the online list.
func (b *Broker) Unregister(userID int64, prog Sender) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.sessions[userID]
	for i, s := range list {
		if s.Program == prog {
			b.sessions[userID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(b.sessions[userID]) == 0 {
		delete(b.sessions, userID)
	}
}

// Send delivers msg to every live session of toUID. Returns true if at
// least one session received it (i.e. the recipient is online).
//
// IMPORTANT: do NOT call from inside an Update path with toUID equal to
// the caller's own UserID. bubbletea's tea.Program.Send writes to an
// unbuffered msgs channel, so a self-targeted Send while the program
// loop is blocked in Update will deadlock the SSH session. Application
// code must guard self-targeting at the call site (see
// pitfalls/water-balloon-self-send-hangs-server.md).
func (b *Broker) Send(toUID int64, msg tea.Msg) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	list := b.sessions[toUID]
	for _, s := range list {
		s.Program.Send(msg)
	}
	return len(list) > 0
}

// SendToAll broadcasts to every online session. excludeUID lets the
// originator skip themselves (pass 0 to broadcast to everyone).
func (b *Broker) SendToAll(excludeUID int64, msg tea.Msg) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for uid, list := range b.sessions {
		if uid == excludeUID {
			continue
		}
		for _, s := range list {
			s.Program.Send(msg)
		}
	}
}

// IsOnline reports whether the user has at least one live session. Read-only;
// uses the same RLock as Send so this is safe to call from the notify
// dispatcher's worker goroutines while Update loops are running.
func (b *Broker) IsOnline(uid int64) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.sessions[uid]) > 0
}

// LookupByUserID returns the numeric user_id for a logged-in account name,
// or 0 if nobody by that name is currently online. Case-insensitive.
func (b *Broker) LookupByUserID(name string) int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for uid, list := range b.sessions {
		for _, s := range list {
			if equalFold(s.UserIDStr, name) {
				return uid
			}
		}
	}
	return 0
}

func (b *Broker) OnlineList() []OnlineUser {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]OnlineUser, 0, len(b.sessions))
	for uid, list := range b.sessions {
		if len(list) == 0 {
			continue
		}
		out = append(out, OnlineUser{
			UserID:    uid,
			UserIDStr: list[0].UserIDStr,
			Sessions:  len(list),
		})
	}
	return out
}

// SessionsSnapshot returns a defensive copy of every session pointer for
// graceful shutdown (so the caller can iterate without holding the lock
// while blocking on per-program quits).
func (b *Broker) SessionsSnapshot() []*Session {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []*Session
	for _, list := range b.sessions {
		out = append(out, list...)
	}
	return out
}

// equalFold is strings.EqualFold inlined to avoid the import in this tiny package.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
