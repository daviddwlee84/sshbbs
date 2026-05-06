package tui

import (
	"context"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestNotifySettings_FlipsAndPersistsPrefs(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}

	m := newNotifySettingsModel(deps)
	// Cursor starts at row 0 (OnPush). Space flips it to false.
	model, _ := m.Update(keyOf(" "))
	got := model.(notifySettingsModel)
	if got.prefs.OnPush {
		t.Errorf("OnPush still true after space-flip")
	}
	if !got.prefsDirty {
		t.Errorf("prefsDirty = false after flip")
	}
	// Ctrl+S persists.
	model, _ = got.Update(keyOf("ctrl+s"))
	got = model.(notifySettingsModel)
	if got.prefsDirty {
		t.Errorf("prefsDirty still true after save")
	}
	// DB row exists with the flipped value.
	persisted, _ := st.Notify().GetPrefs(context.Background(), u.ID)
	if persisted.OnPush {
		t.Errorf("DB OnPush = true after flip+save: %+v", persisted)
	}
	want := store.NotifyPrefs{OnPush: false, OnWB: true, OnMail: true, OnReply: true, OnlyWhenOffline: false}
	if persisted != want {
		t.Errorf("persisted prefs = %+v, want %+v", persisted, want)
	}
}

func TestNotifySettings_AddTargetRoundTrip(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}

	m := newNotifySettingsModel(deps)
	// Move cursor to the "+ add" row (notifPrefCount + 0 targets = 5).
	for i := 0; i < notifPrefCount; i++ {
		model, _ := m.Update(keyOf("j"))
		m = model.(notifySettingsModel)
	}
	if m.cursor != m.addRowIndex() {
		t.Fatalf("cursor = %d, want add-row %d", m.cursor, m.addRowIndex())
	}
	// Enter starts the add flow.
	model, _ := m.Update(keyOf("enter"))
	m = model.(notifySettingsModel)
	if m.mode != modeEdit {
		t.Fatalf("mode = %d, want modeEdit", m.mode)
	}
	m.editLabel.SetValue("discord")
	m.editURL.SetValue("https://example.com/hook")
	model, _ = m.submitEdit()
	m = model.(notifySettingsModel)
	if m.mode != modeList {
		t.Errorf("mode after submit = %d, want modeList", m.mode)
	}
	if len(m.targets) != 1 || m.targets[0].URL != "https://example.com/hook" {
		t.Errorf("targets = %+v", m.targets)
	}
	// Verify in DB.
	rows, _ := st.Notify().ListTargets(context.Background(), u.ID)
	if len(rows) != 1 {
		t.Errorf("DB rows = %d, want 1", len(rows))
	}
}

func TestNotifySettings_RejectsInvalidURL(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}
	m := newNotifySettingsModel(deps)
	m.startAdd()
	m.editLabel.SetValue("discord")
	m.editURL.SetValue("discord://abc") // not http(s)
	model, _ := m.submitEdit()
	got := model.(notifySettingsModel)
	if got.err == "" {
		t.Errorf("expected validation error for non-http URL")
	}
	if got.mode != modeEdit {
		t.Errorf("mode dropped to %d after validation error; should stay in edit", got.mode)
	}
}

func TestNotifySettings_ToggleAndDeleteTarget(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	id, _ := st.Notify().AddTarget(context.Background(), u.ID, "x", "https://example.com/x")

	deps := Deps{Store: st, User: u}
	m := newNotifySettingsModel(deps)
	// Move to the first target row.
	m.cursor = notifPrefCount
	// `t` toggles enabled to false.
	model, _ := m.Update(keyOf("t"))
	m = model.(notifySettingsModel)
	rows, _ := st.Notify().ListTargets(context.Background(), u.ID)
	if rows[0].ID != id || rows[0].Enabled {
		t.Errorf("after t: %+v", rows[0])
	}
	// `d` deletes.
	model, _ = m.Update(keyOf("d"))
	m = model.(notifySettingsModel)
	rows, _ = st.Notify().ListTargets(context.Background(), u.ID)
	if len(rows) != 0 {
		t.Errorf("after d: %d rows", len(rows))
	}
}

func TestNotifySettings_EscReturnsToSettings(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}
	m := newNotifySettingsModel(deps)
	_, cmd := m.Update(keyOf("esc"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenUserSettings {
		t.Errorf("nav = %+v, want ScreenUserSettings", nav)
	}
}
