package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// Build an admin-screen fixture with one admin (alice) and N other users
// in default role 'user'. Returns the model + the admin's User pointer.
func newAdminUsersFixture(t *testing.T, others int) (adminUsersModel, *store.Store, *store.User) {
	t.Helper()
	st := storetest.New(t)
	ctx := context.Background()
	admin := storetest.MustUser(t, st, "alice", "Alice")
	if err := st.Users().SetRole(ctx, admin.ID, store.RoleAdmin); err != nil {
		t.Fatalf("SetRole admin: %v", err)
	}
	admin.Role = store.RoleAdmin
	for i := 0; i < others; i++ {
		_ = storetest.MustUser(t, st, peerName(i), "")
	}
	deps := Deps{Store: st, User: admin}
	return newAdminUsersModel(deps), st, admin
}

func peerName(i int) string {
	switch i {
	case 0:
		return "bob"
	case 1:
		return "carol"
	case 2:
		return "dave"
	}
	return "user_" + string(rune('a'+i))
}

func TestAdminUsers_LoadsListing(t *testing.T) {
	m, _, _ := newAdminUsersFixture(t, 2)
	if m.loadErr != nil {
		t.Fatalf("loadErr = %v", m.loadErr)
	}
	if len(m.users) != 3 {
		t.Errorf("got %d users, want 3 (alice + 2 peers)", len(m.users))
	}
}

func TestAdminUsers_PromoteToMod(t *testing.T) {
	m, st, admin := newAdminUsersFixture(t, 1)
	// Cursor onto bob (the second user; alice is first by id).
	m.cursor = 1
	if m.users[m.cursor].UserID != "bob" {
		t.Fatalf("cursor target = %q, want bob", m.users[m.cursor].UserID)
	}
	model, _ := m.Update(keyOf("M"))
	got := model.(adminUsersModel)
	bob, _ := st.Users().GetByUserID(context.Background(), "bob")
	if bob.Role != store.RoleMod {
		t.Errorf("bob.Role = %q, want mod", bob.Role)
	}
	if !strings.Contains(got.toast, "mod") {
		t.Errorf("toast = %q, want one mentioning 'mod'", got.toast)
	}
	_ = admin
}

func TestAdminUsers_LastAdminGuard(t *testing.T) {
	m, st, admin := newAdminUsersFixture(t, 1)
	// Cursor onto admin (alice).
	for i, u := range m.users {
		if u.ID == admin.ID {
			m.cursor = i
		}
	}
	// Try to demote alice from admin → user. Refused: she's the only admin.
	model, _ := m.Update(keyOf("u"))
	got := model.(adminUsersModel)
	stillAdmin, _ := st.Users().GetByID(context.Background(), admin.ID)
	if stillAdmin.Role != store.RoleAdmin {
		t.Errorf("alice was demoted despite being the last admin: now %q", stillAdmin.Role)
	}
	if !strings.Contains(got.toast, "管理員") && !strings.Contains(got.toast, "admin") {
		t.Errorf("toast = %q, want refusal message", got.toast)
	}

	// Promote bob to admin first, then alice→user must succeed.
	bob, _ := st.Users().GetByUserID(context.Background(), "bob")
	if err := st.Users().SetRole(context.Background(), bob.ID, store.RoleAdmin); err != nil {
		t.Fatalf("seed bob admin: %v", err)
	}
	got.reload()
	model, _ = got.Update(keyOf("u"))
	got = model.(adminUsersModel)
	demoted, _ := st.Users().GetByID(context.Background(), admin.ID)
	if demoted.Role != store.RoleUser {
		t.Errorf("alice not demoted after seeding second admin: now %q", demoted.Role)
	}
}

func TestAdminUsers_BackKeys(t *testing.T) {
	m, _, _ := newAdminUsersFixture(t, 0)
	for _, key := range []string{"esc", "backspace", "left", "h", "Q"} {
		t.Run(key, func(t *testing.T) {
			_, cmd := m.Update(keyOf(key))
			msg := runCmd(cmd)
			nav, ok := msg.(NavigateMsg)
			if !ok {
				t.Fatalf("got %T, want NavigateMsg", msg)
			}
			if nav.To != ScreenMainMenu {
				t.Errorf("To = %v, want ScreenMainMenu", nav.To)
			}
		})
	}
}

func TestAdminUsers_NoOpWhenSameRole(t *testing.T) {
	m, _, _ := newAdminUsersFixture(t, 1)
	// Cursor onto bob (a regular user); pressing 'u' (the role he already has) toasts no-op.
	m.cursor = 1
	model, _ := m.Update(keyOf("u"))
	got := model.(adminUsersModel)
	if !strings.Contains(got.toast, "已是") && !strings.Contains(got.toast, "user") {
		t.Errorf("toast = %q, want no-op message mentioning the role", got.toast)
	}
}

// MainMenu admin entry presence is conditional on the user's role.
func TestMainMenu_AdminEntryConditional(t *testing.T) {
	regular := &store.User{ID: 1, UserID: "alice", Role: store.RoleUser}
	admin := &store.User{ID: 2, UserID: "admin0", Role: store.RoleAdmin}

	mRegular := newMainMenuModel(Deps{User: regular})
	for _, it := range mRegular.items {
		if it.to == ScreenAdminUsers {
			t.Error("regular user's main menu should not include the admin entry")
		}
	}

	mAdmin := newMainMenuModel(Deps{User: admin})
	found := false
	for _, it := range mAdmin.items {
		if it.to == ScreenAdminUsers {
			found = true
			break
		}
	}
	if !found {
		t.Error("admin's main menu should include the admin entry")
	}
}
