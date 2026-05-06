package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func bannerEditFixture(t *testing.T, role store.Role) (boardBannerEditModel, *store.Board) {
	t.Helper()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	user.Role = role
	board := storetest.MustBoard(t, st, "Test")
	deps := Deps{Store: st, User: user, Broker: chat.NewBroker()}
	m := newBoardBannerEditModel(deps, board.ID)
	return m, board
}

func TestBannerEdit_NonModBlockedAtConstruction(t *testing.T) {
	for _, role := range []store.Role{store.RoleGuest, store.RoleUser} {
		t.Run(string(role), func(t *testing.T) {
			m, _ := bannerEditFixture(t, role)
			if m.loadErr == nil {
				t.Errorf("%s: expected loadErr, got nil", role)
			}
		})
	}
}

func TestBannerEdit_ModCanLoad(t *testing.T) {
	m, _ := bannerEditFixture(t, store.RoleMod)
	if m.loadErr != nil {
		t.Fatalf("loadErr = %v", m.loadErr)
	}
}

func TestBannerEdit_PrefillsFromDB(t *testing.T) {
	m, board := bannerEditFixture(t, store.RoleAdmin)
	// ASCII-only intentionally: bubbles' textarea filters raw ESC bytes
	// (0x1b) on SetValue, so a "round-trip raw ANSI through the form"
	// test would fail not on our code but on the upstream input filter.
	// Mods writing colored banners should use plain-ASCII art for now;
	// adding raw-ANSI editing is tracked as a separate TODO.
	const want = "=== HELLO BANNER ==="
	if err := m.deps.Store.Boards().UpdateBanner(t.Context(), board.ID, m.deps.User.ID, store.RoleAdmin, want); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m = newBoardBannerEditModel(m.deps, board.ID)
	if got := m.body.Value(); got != want {
		t.Errorf("body prefill = %q, want %q", got, want)
	}
}

func TestBannerEdit_CtrlSSavesAndNavigatesBack(t *testing.T) {
	m, board := bannerEditFixture(t, store.RoleAdmin)
	m.body.SetValue("new banner art")

	model, cmd := m.submit()
	got := model.(boardBannerEditModel)
	if got.err != "" {
		t.Fatalf("submit err = %q", got.err)
	}
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenBoardView || nav.BoardID != board.ID {
		t.Errorf("nav = %+v, want board view %d", nav, board.ID)
	}

	updated, _ := m.deps.Store.Boards().GetByID(context.Background(), board.ID)
	if updated.Banner != "new banner art" {
		t.Errorf("DB banner = %q, want 'new banner art'", updated.Banner)
	}
}

func TestBannerEdit_EscapeReturnsToBoardView(t *testing.T) {
	m, board := bannerEditFixture(t, store.RoleAdmin)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenBoardView || nav.BoardID != board.ID {
		t.Errorf("esc nav = %+v, want board view %d", nav, board.ID)
	}
}

// h and l must reach the textarea (cursor motion), not navigate. This
// matches the convention used by screen_post_compose / screen_article_edit.
func TestBannerEdit_HLNotBound(t *testing.T) {
	m, _ := bannerEditFixture(t, store.RoleAdmin)
	for _, key := range []string{"h", "l"} {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		got, _ := m.Update(msg)
		if _, ok := got.(boardBannerEditModel); !ok {
			t.Errorf("key %q: model became %T, want boardBannerEditModel", key, got)
		}
	}
}

func TestBannerEdit_EmptyBodyClearsBanner(t *testing.T) {
	m, board := bannerEditFixture(t, store.RoleAdmin)
	// Pre-seed something then clear.
	_ = m.deps.Store.Boards().UpdateBanner(context.Background(), board.ID, m.deps.User.ID, store.RoleAdmin, "old")
	m.body.SetValue("")
	model, _ := m.submit()
	if got := model.(boardBannerEditModel); got.err != "" {
		t.Fatalf("submit err = %q", got.err)
	}
	updated, _ := m.deps.Store.Boards().GetByID(context.Background(), board.ID)
	if updated.Banner != "" {
		t.Errorf("Banner = %q, want empty after clear", updated.Banner)
	}
}
