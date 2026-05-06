package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/notify"
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

func TestNotifySettings_TKeyTestsTarget(t *testing.T) {
	// Spin up a sink that returns 200 so SendTest succeeds.
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	_, _ = st.Notify().AddTarget(context.Background(), u.ID, "test", srv.URL)

	mgr := notify.New(st, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	deps := Deps{Store: st, User: u, Notify: mgr}
	m := newNotifySettingsModel(deps)
	// Cursor on the first target row.
	m.cursor = notifPrefCount

	// T returns a tea.Cmd that does the HTTP call. Run it inline.
	model, cmd := m.Update(keyOf("T"))
	got := model.(notifySettingsModel)
	if cmd == nil {
		t.Fatal("T returned nil cmd; expected HTTP-bearing tea.Cmd")
	}
	if got.flash == "" {
		t.Errorf("flash empty during in-flight test; expected '🔔 testing…' message")
	}
	// Execute the cmd (synchronously runs the HTTP call) — tea.Cmd is
	// just a func() tea.Msg.
	resultMsg := cmd()
	res, ok := resultMsg.(notifyTestResultMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want notifyTestResultMsg", resultMsg)
	}
	if res.err != nil {
		t.Errorf("test failed: %v", res.err)
	}
	if hits != 1 {
		t.Errorf("sink hits = %d, want 1", hits)
	}

	// Feed the result back into the model, verify flash updates.
	model, _ = got.Update(res)
	got = model.(notifySettingsModel)
	if got.err != "" {
		t.Errorf("err set on success: %q", got.err)
	}
	if got.flash == "" {
		t.Errorf("flash empty after success result; expected ✓ message")
	}
}

func TestNotifySettings_TKeyOnNonTargetRowIsNoop(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}
	m := newNotifySettingsModel(deps)
	// Cursor on a pref toggle row (0..notifPrefCount-1) — T should no-op.
	m.cursor = 0
	_, cmd := m.Update(keyOf("T"))
	if cmd != nil {
		t.Errorf("T on toggle row returned cmd; should no-op")
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
