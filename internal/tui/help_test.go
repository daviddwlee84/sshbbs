package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// rootForHelp builds a minimal Root logged in as alice on the main menu.
// Used by every help-overlay test below.
func rootForHelp(t *testing.T) Root {
	t.Helper()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	deps := Deps{Store: st, User: alice, Broker: chat.NewBroker()}
	return NewRoot(deps)
}

// TestHelp_QuestionMarkOpensOverlay asserts '?' on the main menu flips
// helpVisible and that View() returns the help block instead of the menu.
func TestHelp_QuestionMarkOpensOverlay(t *testing.T) {
	r := rootForHelp(t)
	if r.helpVisible {
		t.Fatal("helpVisible should start false")
	}
	updated, _ := r.Update(keyOf("?"))
	r = updated.(Root)
	if !r.helpVisible {
		t.Errorf("? did not flip helpVisible")
	}
	v := r.View()
	if !strings.Contains(v, "Help") {
		t.Errorf("View missing help title; got: %q", v)
	}
	if !strings.Contains(v, "Main menu") {
		t.Errorf("View missing main-menu section heading; got: %q", v)
	}
	if !strings.Contains(v, "? ") || !strings.Contains(v, "Global") {
		t.Errorf("View missing global section / '?' entry; got: %q", v)
	}
}

// TestHelp_AnyKeyDismisses fires a non-'?' key while help is open and
// expects helpVisible to flip back off without triggering navigation.
func TestHelp_AnyKeyDismisses(t *testing.T) {
	r := rootForHelp(t)
	updated, _ := r.Update(keyOf("?"))
	r = updated.(Root)
	if !r.helpVisible {
		t.Fatal("setup: ? should open help")
	}

	for _, key := range []string{"j", "esc", "enter", " ", "x"} {
		t.Run(key, func(t *testing.T) {
			r2 := r // copy
			r2.helpVisible = true
			updated, cmd := r2.Update(keyOf(key))
			r2 = updated.(Root)
			if r2.helpVisible {
				t.Errorf("key %q did not dismiss help overlay", key)
			}
			if cmd != nil {
				if msg := runCmd(cmd); msg != nil {
					t.Errorf("dismissing key %q produced %T %+v, want nil", key, msg, msg)
				}
			}
		})
	}
}

// TestHelp_CtrlCStillQuitsWhileOpen guards against a regression where the
// help overlay swallows Ctrl+C. Ctrl+C must always quit.
func TestHelp_CtrlCStillQuitsWhileOpen(t *testing.T) {
	r := rootForHelp(t)
	updated, _ := r.Update(keyOf("?"))
	r = updated.(Root)
	if !r.helpVisible {
		t.Fatal("setup")
	}
	_, cmd := r.Update(keyOf("ctrl+c"))
	if cmd == nil {
		t.Fatal("ctrl+c returned no cmd")
	}
	got := runCmd(cmd)
	if _, ok := got.(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c → %T %+v, want tea.QuitMsg", got, got)
	}
}

// TestHelp_ActiveScreenTrackedAcrossNavigate asserts navigate updates the
// activeScreen field so subsequent '?' shows the correct page.
func TestHelp_ActiveScreenTrackedAcrossNavigate(t *testing.T) {
	r := rootForHelp(t)
	if r.activeScreen != ScreenMainMenu {
		t.Errorf("initial activeScreen = %v, want ScreenMainMenu", r.activeScreen)
	}
	updated, _ := r.navigate(NavigateMsg{To: ScreenWBInbox})
	r = updated.(Root)
	if r.activeScreen != ScreenWBInbox {
		t.Errorf("after navigate to ScreenWBInbox: activeScreen = %v", r.activeScreen)
	}

	// '?' on the WB inbox should mention water balloons.
	updated, _ = r.Update(keyOf("?"))
	r = updated.(Root)
	if !strings.Contains(r.View(), "水球") {
		t.Errorf("WB inbox help missing 水球 heading; got: %q", r.View())
	}
}

// TestHelp_FormScreensSuppressOverlay verifies '?' is forwarded to the
// sub-screen (not intercepted) when a form screen is active. We assert
// helpVisible stays false; the textinput within the sub will receive the
// keypress and treat it as input.
func TestHelp_FormScreensSuppressOverlay(t *testing.T) {
	formScreens := []Screen{
		ScreenPostCompose,
		ScreenWBCompose,
		ScreenMailCompose,
		ScreenArticleEdit,
		ScreenBoardBannerEdit,
		ScreenPasswordChange,
	}
	for _, s := range formScreens {
		t.Run(fmt.Sprintf("screen-%d", s), func(t *testing.T) {
			r := rootForHelp(t)
			r.activeScreen = s
			updated, _ := r.Update(keyOf("?"))
			r2 := updated.(Root)
			if r2.helpVisible {
				t.Errorf("form screen %v: '?' should not open help overlay", s)
			}
		})
	}
}

// TestHelp_RegisterFlowSuppresses ensures '?' during register doesn't
// pop a help overlay (the user isn't logged in; help table is irrelevant).
func TestHelp_RegisterFlowSuppresses(t *testing.T) {
	st := storetest.New(t)
	deps := Deps{Store: st, IsRegister: true}
	r := NewRoot(deps)
	updated, _ := r.Update(keyOf("?"))
	r = updated.(Root)
	if r.helpVisible {
		t.Errorf("'?' in register flow should not open help overlay")
	}
}

// TestHelp_NonFormScreensAllHaveEntries is a coverage check: every
// non-form, user-facing screen const should appear in the screenHelp map
// so the overlay never falls back to the "no keymap registered" message.
func TestHelp_NonFormScreensAllHaveEntries(t *testing.T) {
	want := []Screen{
		ScreenMainMenu,
		ScreenBoardList,
		ScreenBoardView,
		ScreenArticleView,
		ScreenWBInbox,
		ScreenWBThread,
		ScreenOnline,
		ScreenMailInbox,
		ScreenMailThread,
		ScreenAdminUsers,
		ScreenArticleExport,
		ScreenBoardSplash,
	}
	for _, s := range want {
		if len(screenHelpFor(i18n.Default, s)) == 0 {
			t.Errorf("missing screenHelp entry for %v", s)
		}
	}
}

// TestHelp_BoardListAndViewAdvertiseSearchSort guards the keymap entries
// for the new / and s shortcuts. If someone removes a key from the
// implementation, the inline help advertising it will stop matching the
// real behaviour — this test catches that drift.
func TestHelp_BoardListAndViewAdvertiseSearchSort(t *testing.T) {
	requireKey(t, ScreenBoardList, "/")
	requireKey(t, ScreenBoardView, "/")
	requireKey(t, ScreenBoardView, "s")
}

// requireKey fails the test unless at least one HelpEntry on the given
// screen begins with the wanted key (substring match — entries like
// "/ search by title" still satisfy `requireKey(.., "/")`).
func requireKey(t *testing.T, s Screen, key string) {
	t.Helper()
	for _, sec := range screenHelpFor(i18n.Default, s) {
		for _, e := range sec.Entries {
			if strings.HasPrefix(e.Keys, key) {
				return
			}
		}
	}
	t.Errorf("screen %v: no HelpEntry advertises key %q", s, key)
}
