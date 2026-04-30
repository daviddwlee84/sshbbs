package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/auth"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// Build a fresh store, seed `admin` with a known password and
// must_change_password=1, return a password-change model bound to it.
func newPasswordChangeFixture(t *testing.T, current string) (passwordChangeModel, *store.Store, *store.User) {
	t.Helper()
	st := storetest.New(t)
	ctx := context.Background()
	hash, err := auth.HashPassword(current)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	u, err := st.Users().InsertSystemAccount(ctx, "admin", hash, store.RoleAdmin, true)
	if err != nil {
		t.Fatalf("InsertSystemAccount: %v", err)
	}
	deps := Deps{Store: st, User: u, MustChangePassword: true}
	m := newPasswordChangeModel(deps)
	return m, st, u
}

func setPwField(m passwordChangeModel, idx int, value string) passwordChangeModel {
	m.inputs[idx].SetValue(value)
	return m
}

func TestPasswordChange_RejectsWrongCurrent(t *testing.T) {
	m, _, _ := newPasswordChangeFixture(t, "old-password")
	m = setPwField(m, pwFieldCurrent, "totally-wrong")
	m = setPwField(m, pwFieldNew, "new-pw-1")
	m = setPwField(m, pwFieldConfirm, "new-pw-1")

	model, _ := m.submit()
	got := model.(passwordChangeModel)
	if got.success {
		t.Error("submit succeeded with wrong current password")
	}
	if got.err == "" {
		t.Error("expected error message; got empty")
	}
}

func TestPasswordChange_RejectsConfirmMismatch(t *testing.T) {
	m, _, _ := newPasswordChangeFixture(t, "old-password")
	m = setPwField(m, pwFieldCurrent, "old-password")
	m = setPwField(m, pwFieldNew, "new-pw-1")
	m = setPwField(m, pwFieldConfirm, "different")

	model, _ := m.submit()
	got := model.(passwordChangeModel)
	if got.success {
		t.Error("submit succeeded with confirm mismatch")
	}
}

func TestPasswordChange_RejectsTooShort(t *testing.T) {
	m, _, _ := newPasswordChangeFixture(t, "old-password")
	m = setPwField(m, pwFieldCurrent, "old-password")
	m = setPwField(m, pwFieldNew, "x")
	m = setPwField(m, pwFieldConfirm, "x")

	model, _ := m.submit()
	got := model.(passwordChangeModel)
	if got.success {
		t.Error("submit succeeded with too-short new password")
	}
}

func TestPasswordChange_RejectsSameAsCurrent(t *testing.T) {
	m, _, _ := newPasswordChangeFixture(t, "old-pw-12")
	m = setPwField(m, pwFieldCurrent, "old-pw-12")
	m = setPwField(m, pwFieldNew, "old-pw-12")
	m = setPwField(m, pwFieldConfirm, "old-pw-12")

	model, _ := m.submit()
	got := model.(passwordChangeModel)
	if got.success {
		t.Error("submit succeeded with new == current")
	}
}

func TestPasswordChange_Success_ClearsFlagAndNavigates(t *testing.T) {
	m, st, u := newPasswordChangeFixture(t, "old-password")
	m = setPwField(m, pwFieldCurrent, "old-password")
	m = setPwField(m, pwFieldNew, "fresh-pw-9")
	m = setPwField(m, pwFieldConfirm, "fresh-pw-9")

	model, cmd := m.submit()
	got := model.(passwordChangeModel)
	if !got.success {
		t.Fatalf("submit failed: %s", got.err)
	}
	msg := runCmd(cmd)
	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("got %T, want NavigateMsg", msg)
	}
	if nav.To != ScreenMainMenu {
		t.Errorf("To = %v, want ScreenMainMenu", nav.To)
	}

	// Verify flag is cleared in DB and the in-memory User pointer is refreshed.
	fresh, err := st.Users().GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("re-fetch: %v", err)
	}
	if fresh.MustChangePassword {
		t.Error("must_change_password not cleared in DB")
	}
	if u.MustChangePassword {
		t.Error("in-memory User.MustChangePassword not cleared (Deps.User pointer not refreshed)")
	}
	// And the new password must verify.
	if err := auth.VerifyPasswordHash(fresh.PasswordHash, "fresh-pw-9"); err != nil {
		t.Errorf("new password does not verify against stored hash: %v", err)
	}
}

func TestPasswordChange_EscQuits(t *testing.T) {
	m, _, _ := newPasswordChangeFixture(t, "old-password")
	_, cmd := m.Update(keyOf("esc"))
	msg := runCmd(cmd)
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("got %T, want tea.QuitMsg", msg)
	}
}
