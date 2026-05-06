package tui

import (
	"context"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestBioEdit_PrefillsFromUser(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	_ = st.Users().SetBio(context.Background(), u.ID, "existing bio")
	fresh, _ := st.Users().GetByID(context.Background(), u.ID)
	deps := Deps{Store: st, User: fresh}

	m := newBioEditModel(deps)
	if got := m.body.Value(); got != "existing bio" {
		t.Errorf("body prefilled = %q, want %q", got, "existing bio")
	}
}

func TestBioEdit_SubmitPersistsAndRefreshesUser(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}

	m := newBioEditModel(deps)
	m.body.SetValue("hello\nworld")

	model, cmd := m.submit()
	got := model.(bioEditModel)
	if got.err != "" {
		t.Fatalf("submit err = %q", got.err)
	}
	if !got.success {
		t.Errorf("success = false")
	}
	// Cmd should be a NavigateMsg back to settings.
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenUserSettings {
		t.Errorf("nav = %+v, want ScreenUserSettings", nav)
	}
	// In-memory User pointer is updated.
	if u.Bio != "hello\nworld" {
		t.Errorf("in-memory bio = %q, want refreshed", u.Bio)
	}
	// DB is updated.
	fresh, _ := st.Users().GetByID(context.Background(), u.ID)
	if fresh.Bio != "hello\nworld" {
		t.Errorf("DB bio = %q", fresh.Bio)
	}
}

func TestBioEdit_EscReturnsToSettings(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}
	m := newBioEditModel(deps)
	_, cmd := m.Update(keyOf("esc"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenUserSettings {
		t.Errorf("nav = %+v, want ScreenUserSettings", nav)
	}
}
