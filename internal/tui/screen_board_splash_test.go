package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func splashFixture(t *testing.T, banner string) (boardSplashModel, *store.Board) {
	t.Helper()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	if banner != "" {
		if err := st.Boards().UpdateBanner(t.Context(), board.ID, user.ID, store.RoleAdmin, banner); err != nil {
			t.Fatalf("seed banner: %v", err)
		}
	}
	deps := Deps{Store: st, User: user, Broker: chat.NewBroker()}
	return newBoardSplashModel(deps, board.ID), board
}

// Any key returns to the board view with the right BoardID.
func TestBoardSplash_AnyKeyReturns(t *testing.T) {
	for _, key := range []string{"esc", "enter", "q", " ", "x", "b"} {
		t.Run(key, func(t *testing.T) {
			m, board := splashFixture(t, "art")
			_, cmd := m.Update(keyOf(key))
			if cmd == nil {
				t.Fatalf("key %q: nil cmd", key)
			}
			nav := runCmd(cmd).(NavigateMsg)
			if nav.To != ScreenBoardView || nav.BoardID != board.ID {
				t.Errorf("key %q: nav = %+v, want board view %d", key, nav, board.ID)
			}
		})
	}
}

func TestBoardSplash_RendersBannerContent(t *testing.T) {
	m, _ := splashFixture(t, "MARKER-XYZ")
	m, _ = mustModel(m.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))

	view := m.View()
	if !strings.Contains(view, "MARKER-XYZ") {
		t.Errorf("view missing banner marker; got:\n%s", view)
	}
}

func TestBoardSplash_NoBannerHint(t *testing.T) {
	m, _ := splashFixture(t, "")
	view := m.View()
	if !strings.Contains(view, "no banner") {
		t.Errorf("view missing 'no banner' hint; got:\n%s", view)
	}
}

func mustModel(model tea.Model, cmd tea.Cmd) (boardSplashModel, tea.Cmd) {
	return model.(boardSplashModel), cmd
}
