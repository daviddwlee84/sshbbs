package tui

import (
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestUserSettings_NavigatesPerEntry(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}

	cases := []struct {
		key  string
		want Screen
	}{
		{"1", ScreenPasswordChange},
		{"2", ScreenBioEdit},
		{"3", ScreenLocaleSettings},
		{"4", ScreenNotifySettings},
		{"5", ScreenMainMenu}, // 返回
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			m := newUserSettingsModel(deps)
			_, cmd := m.Update(keyOf(tc.key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok {
				t.Fatalf("got non-NavigateMsg")
			}
			if nav.To != tc.want {
				t.Errorf("To = %v, want %v", nav.To, tc.want)
			}
		})
	}
}

func TestUserSettings_EscReturnsToMainMenu(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}
	m := newUserSettingsModel(deps)
	_, cmd := m.Update(keyOf("esc"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenMainMenu {
		t.Errorf("nav = %+v, want ScreenMainMenu", nav)
	}
}

func TestUserSettings_JKMovesCursor(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}
	m := newUserSettingsModel(deps)
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d", m.cursor)
	}
	model, _ := m.Update(keyOf("j"))
	got := model.(userSettingsModel)
	if got.cursor != 1 {
		t.Errorf("after j cursor = %d, want 1", got.cursor)
	}
	model, _ = got.Update(keyOf("k"))
	got = model.(userSettingsModel)
	if got.cursor != 0 {
		t.Errorf("after k cursor = %d, want 0", got.cursor)
	}
}
