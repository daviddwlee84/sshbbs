package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// nullSender is a chat.Sender that swallows messages — sufficient for the
// online screen which only reads OnlineList(), never Sends.
type nullSender struct{}

func (nullSender) Send(tea.Msg) {}

func newOnlineFixture(t *testing.T, registered ...string) onlineModel {
	t.Helper()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	br := chat.NewBroker()
	for i, name := range registered {
		br.Register(&chat.Session{
			UserID:    int64(i + 1),
			UserIDStr: name,
			Program:   nullSender{},
		})
	}
	deps := Deps{Store: st, User: user, Broker: br}
	return newOnlineModel(deps)
}

func TestOnline_EmptyList(t *testing.T) {
	m := newOnlineFixture(t)
	if len(m.users) != 0 {
		t.Errorf("got %d users, want 0", len(m.users))
	}
	out := m.View()
	if !contains(out, "nobody else online") {
		t.Errorf("View missing empty-state hint, got: %q", out)
	}
}

func TestOnline_RendersRegisteredUsers(t *testing.T) {
	m := newOnlineFixture(t, "alice", "bob")
	if len(m.users) != 2 {
		t.Fatalf("got %d users, want 2", len(m.users))
	}
	out := m.View()
	for _, want := range []string{"alice", "bob", "線上使用者"} {
		if !contains(out, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func TestOnline_BackKeys(t *testing.T) {
	for _, key := range []string{"esc", "backspace", "left", "h", "Q"} {
		t.Run(key, func(t *testing.T) {
			m := newOnlineFixture(t, "alice")
			_, cmd := m.Update(keyOf(key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok {
				t.Fatalf("want NavigateMsg")
			}
			if nav.To != ScreenMainMenu {
				t.Errorf("To = %v, want ScreenMainMenu", nav.To)
			}
		})
	}
}

func TestOnline_BracketCursorAliases(t *testing.T) {
	m := newOnlineFixture(t, "alice", "bob")
	model, _ := m.Update(keyOf("]"))
	m = model.(onlineModel)
	if m.cursor != 1 {
		t.Errorf("after ]: cursor = %d, want 1", m.cursor)
	}
	model, _ = m.Update(keyOf("["))
	m = model.(onlineModel)
	if m.cursor != 0 {
		t.Errorf("after [: cursor = %d, want 0", m.cursor)
	}
}

// Forward keys (enter / space / right / l) open WB compose with the
// cursored user pre-filled as recipient.
func TestOnline_ForwardKeysOpenWBCompose(t *testing.T) {
	for _, key := range []string{"enter", " ", "right", "l"} {
		t.Run(key, func(t *testing.T) {
			m := newOnlineFixture(t, "alice", "bob")
			// Move cursor to bob.
			model, _ := m.Update(keyOf("j"))
			m = model.(onlineModel)
			_, cmd := m.Update(keyOf(key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok {
				t.Fatalf("want NavigateMsg")
			}
			if nav.To != ScreenWBCompose {
				t.Errorf("To = %v, want ScreenWBCompose", nav.To)
			}
			if nav.Recipient != "bob" {
				t.Errorf("Recipient = %q, want bob", nav.Recipient)
			}
		})
	}
}

// On an empty list, the forward keys must be no-ops (don't index out of range).
func TestOnline_ForwardKeysNoopWhenEmpty(t *testing.T) {
	m := newOnlineFixture(t)
	_, cmd := m.Update(keyOf("enter"))
	if runCmd(cmd) != nil {
		t.Errorf("enter on empty list should be no-op")
	}
}

var _ tea.Model = onlineModel{}
var _ = (*store.Store)(nil) // keep import live for future expansion
