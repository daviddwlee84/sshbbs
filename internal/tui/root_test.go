package tui

import (
	"strings"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// Layer 1: Root.navigate intercepts compose-screen NavigateMsgs from
// guests with an ErrorMsg toast and leaves the current sub mounted.
func TestNavigate_GuestComposeBlocked(t *testing.T) {
	st := storetest.New(t)
	guest := storetest.MustUser(t, st, "guest", "Guest")
	if err := st.Users().SetRole(t.Context(), guest.ID, store.RoleGuest); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	guest.Role = store.RoleGuest
	deps := Deps{Store: st, User: guest, Broker: chat.NewBroker()}
	r := NewRoot(deps)
	r.sub = newBoardListModel(deps)

	for _, target := range []Screen{ScreenPostCompose, ScreenWBCompose, ScreenMailCompose, ScreenAdminUsers} {
		t.Run(screenName(target), func(t *testing.T) {
			model, cmd := r.Update(NavigateMsg{To: target})
			r2 := model.(Root)
			// The guard means r2.sub stays as boardListModel — verify by type.
			if _, ok := r2.sub.(boardListModel); !ok {
				t.Errorf("sub type = %T, want boardListModel (guard should leave sub unchanged)", r2.sub)
			}
			msg := runCmd(cmd)
			err, ok := msg.(ErrorMsg)
			if !ok {
				t.Fatalf("got %T, want ErrorMsg", msg)
			}
			if err.Err == nil || !strings.Contains(err.Err.Error(), "唯讀") {
				t.Errorf("ErrorMsg = %v, want one mentioning 唯讀 (read-only)", err.Err)
			}
		})
	}
}

// Layer 1 / admin-only: navigate to ScreenAdminUsers as a non-admin
// (regular user) is rejected with a toast.
func TestNavigate_NonAdminAdminUsersBlocked(t *testing.T) {
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "Alice")
	deps := Deps{Store: st, User: user, Broker: chat.NewBroker()}
	r := NewRoot(deps)
	r.sub = newBoardListModel(deps)

	model, cmd := r.Update(NavigateMsg{To: ScreenAdminUsers})
	r2 := model.(Root)
	if _, ok := r2.sub.(boardListModel); !ok {
		t.Errorf("sub type = %T, want boardListModel (non-admin should not enter admin screen)", r2.sub)
	}
	msg := runCmd(cmd)
	err, ok := msg.(ErrorMsg)
	if !ok {
		t.Fatalf("got %T, want ErrorMsg", msg)
	}
	if err.Err == nil || !strings.Contains(err.Err.Error(), "管理員") {
		t.Errorf("ErrorMsg = %v, want one mentioning 管理員", err.Err)
	}
}

// Admins can navigate to ScreenAdminUsers freely. (Sanity: the guard
// should NOT fire for admins.) The screen itself doesn't exist yet — we
// just verify navigate doesn't emit an ErrorMsg here.
func TestNavigate_AdminCanReachAdminScreen(t *testing.T) {
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "admin0", "Admin")
	if err := st.Users().SetRole(t.Context(), user.ID, store.RoleAdmin); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	user.Role = store.RoleAdmin
	deps := Deps{Store: st, User: user, Broker: chat.NewBroker()}
	r := NewRoot(deps)
	r.sub = newBoardListModel(deps)

	_, cmd := r.Update(NavigateMsg{To: ScreenAdminUsers})
	msg := runCmd(cmd)
	if e, ok := msg.(ErrorMsg); ok {
		t.Errorf("admin nav to admin screen produced ErrorMsg: %v", e.Err)
	}
}

func screenName(s Screen) string {
	switch s {
	case ScreenPostCompose:
		return "PostCompose"
	case ScreenWBCompose:
		return "WBCompose"
	case ScreenMailCompose:
		return "MailCompose"
	case ScreenAdminUsers:
		return "AdminUsers"
	}
	return "?"
}
