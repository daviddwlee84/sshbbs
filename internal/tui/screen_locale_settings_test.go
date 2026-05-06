package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func newLocaleSettingsFixture(t *testing.T) (localeSettingsModel, Deps) {
	t.Helper()
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: u}
	return newLocaleSettingsModel(deps), deps
}

func updateLocale(m localeSettingsModel, msg tea.Msg) (localeSettingsModel, tea.Cmd) {
	model, cmd := m.Update(msg)
	return model.(localeSettingsModel), cmd
}

// Default locale is zh-TW; cursor and selected both land on the zh-TW row.
func TestLocaleSettings_InitialState(t *testing.T) {
	m, _ := newLocaleSettingsFixture(t)
	if m.current != i18n.LocaleZHTW {
		t.Errorf("current = %q, want LocaleZHTW", m.current)
	}
	if m.selected != i18n.LocaleZHTW {
		t.Errorf("selected = %q, want LocaleZHTW", m.selected)
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

// Pressing Enter / Space / →  / l on the second row stages "en" as the
// selection but does NOT persist — that requires Ctrl+S.
func TestLocaleSettings_SelectStagesWithoutPersisting(t *testing.T) {
	for _, key := range []string{"enter", " ", "right", "l"} {
		t.Run(key, func(t *testing.T) {
			m, deps := newLocaleSettingsFixture(t)
			m, _ = updateLocale(m, keyOf("j")) // cursor onto en
			m, _ = updateLocale(m, keyOf(key))
			if m.selected != i18n.LocaleEN {
				t.Errorf("selected = %q, want LocaleEN", m.selected)
			}
			if m.current != i18n.LocaleZHTW {
				t.Errorf("current = %q, want LocaleZHTW (no save yet)", m.current)
			}
			fresh, _ := deps.Store.Users().GetByID(context.Background(), deps.User.ID)
			if fresh.Locale != "zh-TW" {
				t.Errorf("DB locale = %q, want zh-TW (no save yet)", fresh.Locale)
			}
		})
	}
}

// Numeric shortcut: '2' jumps to and stages the en row.
func TestLocaleSettings_NumericShortcut(t *testing.T) {
	m, _ := newLocaleSettingsFixture(t)
	m, _ = updateLocale(m, keyOf("2"))
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 after '2'", m.cursor)
	}
	if m.selected != i18n.LocaleEN {
		t.Errorf("selected = %q, want LocaleEN", m.selected)
	}
}

// Ctrl+S persists the staged selection AND mutates the in-memory User
// pointer in place — so the next render of any screen sharing this Deps
// sees the new locale without a re-login.
func TestLocaleSettings_SavePersistsAndRefreshesUser(t *testing.T) {
	m, deps := newLocaleSettingsFixture(t)
	m, _ = updateLocale(m, keyOf("j"))
	m, _ = updateLocale(m, keyOf("enter"))   // stage en
	m, _ = updateLocale(m, keyOf("ctrl+s"))  // commit

	if m.err != "" {
		t.Fatalf("save err = %q", m.err)
	}
	if m.flash == "" {
		t.Errorf("flash empty after save — want a saved-toast")
	}
	// In-place User refresh.
	if deps.User.Locale != "en" {
		t.Errorf("in-memory user locale = %q, want en", deps.User.Locale)
	}
	// DB row updated.
	fresh, _ := deps.Store.Users().GetByID(context.Background(), deps.User.ID)
	if fresh.Locale != "en" {
		t.Errorf("DB locale = %q, want en", fresh.Locale)
	}
}

// Esc returns to user-settings WITHOUT persisting a staged change.
func TestLocaleSettings_EscDiscards(t *testing.T) {
	m, deps := newLocaleSettingsFixture(t)
	m, _ = updateLocale(m, keyOf("j"))
	m, _ = updateLocale(m, keyOf("enter")) // stage en
	_, cmd := updateLocale(m, keyOf("esc"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenUserSettings {
		t.Errorf("nav = %+v, want ScreenUserSettings", nav)
	}
	fresh, _ := deps.Store.Users().GetByID(context.Background(), deps.User.ID)
	if fresh.Locale != "zh-TW" {
		t.Errorf("DB locale = %q, want zh-TW (Esc must not persist)", fresh.Locale)
	}
}

// h / left / backspace are also "back to user-settings" (parity with
// every other vim-style nav screen in the codebase).
func TestLocaleSettings_BackKeys(t *testing.T) {
	for _, key := range []string{"h", "left", "backspace"} {
		t.Run(key, func(t *testing.T) {
			m, _ := newLocaleSettingsFixture(t)
			_, cmd := updateLocale(m, keyOf(key))
			nav, ok := runCmd(cmd).(NavigateMsg)
			if !ok || nav.To != ScreenUserSettings {
				t.Errorf("nav = %+v, want ScreenUserSettings", nav)
			}
		})
	}
}

// Q (capital) jumps straight to main menu.
func TestLocaleSettings_QuitToMainMenu(t *testing.T) {
	m, _ := newLocaleSettingsFixture(t)
	_, cmd := updateLocale(m, keyOf("Q"))
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenMainMenu {
		t.Errorf("nav = %+v, want ScreenMainMenu", nav)
	}
}

// Re-saving the same locale shouldn't error and shouldn't touch the DB
// in any visible way (the row already had this value).
func TestLocaleSettings_SaveSameLocaleIsIdempotent(t *testing.T) {
	m, deps := newLocaleSettingsFixture(t)
	m, _ = updateLocale(m, keyOf("ctrl+s"))
	if m.err != "" {
		t.Fatalf("save err = %q on no-op save", m.err)
	}
	if deps.User.Locale != "zh-TW" {
		t.Errorf("user locale = %q, want unchanged zh-TW", deps.User.Locale)
	}
}

// User who already has en stored: the screen opens with cursor / current
// / selected all landed on the en row so j/k navigation is symmetric.
func TestLocaleSettings_PreExistingEnUser(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	if err := st.Users().SetLocale(context.Background(), u.ID, "en"); err != nil {
		t.Fatalf("SetLocale: %v", err)
	}
	fresh, _ := st.Users().GetByID(context.Background(), u.ID)
	deps := Deps{Store: st, User: fresh}
	m := newLocaleSettingsModel(deps)
	if m.current != i18n.LocaleEN || m.selected != i18n.LocaleEN || m.cursor != 1 {
		t.Errorf("current=%q selected=%q cursor=%d; want en/en/1", m.current, m.selected, m.cursor)
	}
}
