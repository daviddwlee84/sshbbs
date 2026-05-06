package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func newBoardViewFixture(t *testing.T) (boardViewModel, *store.Board, []*store.Article) {
	t.Helper()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	var arts []*store.Article
	for _, title := range []string{"first", "second", "third"} {
		a, err := st.Articles().Create(context.Background(), board.ID, user.ID, user.UserID, title, "")
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		arts = append(arts, a)
	}
	deps := Deps{Store: st, User: user, Broker: chat.NewBroker()}
	return newBoardViewModel(deps, board.ID), board, arts
}

// `[` and `]` move the cursor up/down the article list; matches j/k aliases.
func TestBoardView_BracketCursorAliases(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}
	model, _ := m.Update(keyOf("]"))
	m = model.(boardViewModel)
	if m.cursor != 1 {
		t.Errorf("after ]: cursor = %d, want 1", m.cursor)
	}
	model, _ = m.Update(keyOf("["))
	m = model.(boardViewModel)
	if m.cursor != 0 {
		t.Errorf("after [: cursor = %d, want 0", m.cursor)
	}
}

// `Q` from board view jumps straight to main menu (skipping board list).
func TestBoardView_QuitToMenu(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	_, cmd := m.Update(keyOf("Q"))
	msg := runCmd(cmd)
	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("got %T, want NavigateMsg", msg)
	}
	if nav.To != ScreenMainMenu {
		t.Errorf("To = %v, want ScreenMainMenu", nav.To)
	}
}

// ArticleAddedMsg for THIS board re-fetches the article list from DB.
func TestBoardView_AcceptsArticleAddedForThisBoard(t *testing.T) {
	m, board, arts := newBoardViewFixture(t)
	if len(m.articles) != 3 {
		t.Fatalf("constructor loaded %d, want 3", len(m.articles))
	}
	// Insert a new article externally, then fire the broadcast.
	user := m.deps.User
	a4, err := m.deps.Store.Articles().Create(context.Background(), board.ID, user.ID, user.UserID, "fourth", "")
	if err != nil {
		t.Fatalf("create fourth: %v", err)
	}
	model, _ := m.Update(ArticleAddedMsg{
		BoardID:      board.ID,
		ArticleID:    a4.ID,
		AuthorUserID: "alice",
		Title:        "fourth",
	})
	got := model.(boardViewModel)
	if len(got.articles) != 4 {
		t.Errorf("after ArticleAddedMsg: %d articles, want 4", len(got.articles))
	}
	// Newest first — fourth should be at index 0.
	if got.articles[0].Title != "fourth" {
		t.Errorf("got[0].Title = %q, want fourth (existing arts: %d)", got.articles[0].Title, len(arts))
	}
}

// ArticleAddedMsg for a DIFFERENT board must NOT mutate this view.
func TestBoardView_IgnoresArticleAddedForOtherBoard(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	model, _ := m.Update(ArticleAddedMsg{
		BoardID:      m.board.ID + 999,
		ArticleID:    1,
		AuthorUserID: "bob",
		Title:        "irrelevant",
	})
	got := model.(boardViewModel)
	if len(got.articles) != 3 {
		t.Errorf("appended unrelated article: got %d, want 3", len(got.articles))
	}
}

// All four "back" keys (esc, backspace, left, h) navigate to board list.
func TestBoardView_BackKeys(t *testing.T) {
	for _, key := range []string{"esc", "backspace", "left", "h"} {
		t.Run(key, func(t *testing.T) {
			m, _, _ := newBoardViewFixture(t)
			_, cmd := m.Update(keyOf(key))
			nav := runCmd(cmd).(NavigateMsg)
			if nav.To != ScreenBoardList {
				t.Errorf("To = %v, want ScreenBoardList", nav.To)
			}
		})
	}
}

// Pressing 'b' with a non-empty banner navigates to the splash screen.
// With an empty banner it must no-op (no NavigateMsg).
func TestBoardView_BannerKey_NavigatesToSplash(t *testing.T) {
	m, board, _ := newBoardViewFixture(t)
	user := m.deps.User
	if err := m.deps.Store.Boards().UpdateBanner(t.Context(), board.ID, user.ID, store.RoleAdmin, "art"); err != nil {
		t.Fatalf("seed banner: %v", err)
	}
	// Reload model so it picks up the banner.
	m = newBoardViewModel(m.deps, board.ID)
	if m.board == nil || m.board.Banner == "" {
		t.Fatalf("test setup: banner not loaded; got board=%v", m.board)
	}

	_, cmd := m.Update(keyOf("b"))
	if cmd == nil {
		t.Fatal("b: got nil cmd, want NavigateMsg cmd")
	}
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok {
		t.Fatalf("b: got %T, want NavigateMsg", runCmd(cmd))
	}
	if nav.To != ScreenBoardSplash || nav.BoardID != board.ID {
		t.Errorf("nav = %+v, want splash for board %d", nav, board.ID)
	}
}

func TestBoardView_BannerKey_NoOpWhenEmpty(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	if m.board.Banner != "" {
		t.Fatalf("test setup: expected empty banner; got %q", m.board.Banner)
	}
	_, cmd := m.Update(keyOf("b"))
	if cmd != nil {
		t.Errorf("b with empty banner: got cmd, want nil (no navigation)")
	}
}

// 'B' opens the edit screen only for mod+. A regular user pressing B must
// not navigate.
func TestBoardView_BannerEditKey_GatedByRole(t *testing.T) {
	cases := []struct {
		role       store.Role
		shouldNav  bool
	}{
		{store.RoleUser, false},
		{store.RoleGuest, false},
		{store.RoleMod, true},
		{store.RoleAdmin, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			m, board, _ := newBoardViewFixture(t)
			// Promote the fixture user. Guest is special — it's the
			// reserved guest account; we can just override the in-memory
			// role for the predicate check (DB role doesn't matter for
			// canEditBanner).
			m.deps.User.Role = tc.role

			_, cmd := m.Update(keyOf("B"))
			if tc.shouldNav {
				if cmd == nil {
					t.Fatalf("%s: got nil cmd, want NavigateMsg", tc.role)
				}
				nav := runCmd(cmd).(NavigateMsg)
				if nav.To != ScreenBoardBannerEdit || nav.BoardID != board.ID {
					t.Errorf("%s: nav = %+v, want banner edit for board %d", tc.role, nav, board.ID)
				}
			} else if cmd != nil {
				t.Errorf("%s: got cmd %v, want nil (denied)", tc.role, cmd)
			}
		})
	}
}

// 'M' toggles pin state on the cursor article. Mod+ only.
func TestBoardView_PinKey_GatedByRole(t *testing.T) {
	cases := []struct {
		role        store.Role
		shouldPin   bool
	}{
		{store.RoleGuest, false},
		{store.RoleUser, false},
		{store.RoleMod, true},
		{store.RoleAdmin, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			m, _, arts := newBoardViewFixture(t)
			m.deps.User.Role = tc.role
			// Cursor at 0 (newest, "third"). Pin it.
			_, _ = m.Update(keyOf("M"))

			got, _ := m.deps.Store.Articles().GetByID(context.Background(), arts[2].ID)
			if got.PinnedAt.Valid != tc.shouldPin {
				t.Errorf("%s: pinned = %v, want %v", tc.role, got.PinnedAt.Valid, tc.shouldPin)
			}
		})
	}
}

// 'M' toggles: pinning then pressing M again unpins.
func TestBoardView_PinKey_Toggles(t *testing.T) {
	m, _, arts := newBoardViewFixture(t)
	m.deps.User.Role = store.RoleMod

	// Pin the cursor article (index 0 = "third", the newest).
	model, _ := m.Update(keyOf("M"))
	m = model.(boardViewModel)
	got, _ := m.deps.Store.Articles().GetByID(context.Background(), arts[2].ID)
	if !got.PinnedAt.Valid {
		t.Fatal("first M press: not pinned")
	}

	// Press M again — should unpin.
	model, _ = m.Update(keyOf("M"))
	m = model.(boardViewModel)
	got, _ = m.deps.Store.Articles().GetByID(context.Background(), arts[2].ID)
	if got.PinnedAt.Valid {
		t.Errorf("second M press: still pinned, want unpinned")
	}
}

// Pinning a NON-first article reorders it to the top, and the cursor
// follows the same article (re-anchored by ID).
func TestBoardView_PinKey_ReordersAndPreservesCursor(t *testing.T) {
	m, _, arts := newBoardViewFixture(t)
	m.deps.User.Role = store.RoleMod

	// Move cursor to the 3rd row (oldest, "first").
	model, _ := m.Update(keyOf("j"))
	m = model.(boardViewModel)
	model, _ = m.Update(keyOf("j"))
	m = model.(boardViewModel)
	if m.cursor != 2 || m.articles[m.cursor].ID != arts[0].ID {
		t.Fatalf("setup: cursor=%d on article %d, want cursor=2 on article %d",
			m.cursor, m.articles[m.cursor].ID, arts[0].ID)
	}

	model, _ = m.Update(keyOf("M"))
	m = model.(boardViewModel)
	if m.articles[0].ID != arts[0].ID {
		t.Errorf("pinned article not at index 0: got %d, want %d", m.articles[0].ID, arts[0].ID)
	}
	if !m.articles[0].PinnedAt.Valid {
		t.Error("articles[0] not marked pinned in reloaded slice")
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (followed the pinned article)", m.cursor)
	}
}

// 'M' on an empty article list is a silent no-op (no panic).
func TestBoardView_PinKey_NoOpWhenEmpty(t *testing.T) {
	st := storetest.New(t)
	mod := storetest.MustUser(t, st, "mod", "")
	board := storetest.MustBoard(t, st, "Test")
	deps := Deps{Store: st, User: mod, Broker: chat.NewBroker()}
	deps.User.Role = store.RoleMod
	m := newBoardViewModel(deps, board.ID)
	if len(m.articles) != 0 {
		t.Fatalf("setup: expected 0 articles, got %d", len(m.articles))
	}
	// Must not panic.
	_, _ = m.Update(keyOf("M"))
}

// ArticlePinChangedMsg for THIS board re-fetches the list and re-anchors
// the cursor by article ID across the reorder.
func TestBoardView_AcceptsPinChangedForThisBoard(t *testing.T) {
	m, board, arts := newBoardViewFixture(t)
	m.deps.User.Role = store.RoleMod

	// Position cursor on "second" (index 1).
	model, _ := m.Update(keyOf("j"))
	m = model.(boardViewModel)
	if m.articles[m.cursor].ID != arts[1].ID {
		t.Fatalf("setup: cursor on %d, want %d", m.articles[m.cursor].ID, arts[1].ID)
	}

	// External actor pins "first" (oldest), then broadcasts.
	if err := m.deps.Store.Articles().SetPinned(context.Background(), arts[0].ID, m.deps.User.ID, store.RoleAdmin, true); err != nil {
		t.Fatalf("external pin: %v", err)
	}
	model, _ = m.Update(ArticlePinChangedMsg{
		BoardID: board.ID, ArticleID: arts[0].ID, Pinned: true,
	})
	m = model.(boardViewModel)

	if m.articles[0].ID != arts[0].ID {
		t.Errorf("pinned article not at index 0 after broadcast: got %d", m.articles[0].ID)
	}
	if m.articles[m.cursor].ID != arts[1].ID {
		t.Errorf("cursor moved off 'second': now on %d, want %d", m.articles[m.cursor].ID, arts[1].ID)
	}
}

// ArticlePinChangedMsg for a DIFFERENT board must NOT mutate this view.
func TestBoardView_IgnoresPinChangedForOtherBoard(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	before := len(m.articles)
	model, _ := m.Update(ArticlePinChangedMsg{
		BoardID: m.board.ID + 999, ArticleID: 1, Pinned: true,
	})
	got := model.(boardViewModel)
	if len(got.articles) != before {
		t.Errorf("got %d articles, want %d (unchanged)", len(got.articles), before)
	}
}

// View() prefixes pinned rows with "[M] ".
func TestBoardView_RendersPinnedMarker(t *testing.T) {
	m, _, arts := newBoardViewFixture(t)
	if err := m.deps.Store.Articles().SetPinned(context.Background(), arts[0].ID, m.deps.User.ID, store.RoleAdmin, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	m = newBoardViewModel(m.deps, m.board.ID) // reload
	m.width = 80
	out := m.View()
	if !strings.Contains(out, "[M]") {
		t.Errorf("View() lacks [M] marker for pinned article:\n%s", out)
	}
}

// renderBanner produces no output when the board has no banner (won't
// affect existing tests / layout).
func TestBoardView_RenderBanner_Empty(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	if got := m.renderBanner(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// renderBanner caps to maxInlineBannerLines and tags overflow.
func TestBoardView_RenderBanner_LongBannerTruncated(t *testing.T) {
	m, board, _ := newBoardViewFixture(t)
	user := m.deps.User
	long := ""
	for i := 0; i < maxInlineBannerLines+5; i++ {
		long += "line\n"
	}
	if err := m.deps.Store.Boards().UpdateBanner(t.Context(), board.ID, user.ID, store.RoleAdmin, long); err != nil {
		t.Fatalf("seed banner: %v", err)
	}
	m = newBoardViewModel(m.deps, board.ID)
	m.width = 80

	got := m.renderBanner()
	// Must contain the overflow hint.
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected truncation hint; got:\n%s", got)
	}
}
