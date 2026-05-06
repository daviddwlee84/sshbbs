package tui

import (
	"context"
	"fmt"
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

// View() prefixes locked / arrows-only rows with "[鎖]" / "[箭]".
func TestBoardView_RendersCommentsModeBadges(t *testing.T) {
	for _, tc := range []struct {
		mode       store.CommentsMode
		wantSubstr string
	}{
		{store.CommentsModeLocked, "[鎖]"},
		{store.CommentsModeArrowsOnly, "[箭]"},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			m, _, arts := newBoardViewFixture(t)
			if err := m.deps.Store.Articles().SetCommentsMode(context.Background(),
				arts[0].ID, m.deps.User.ID, store.RoleAdmin, tc.mode); err != nil {
				t.Fatalf("SetCommentsMode: %v", err)
			}
			m = newBoardViewModel(m.deps, m.board.ID)
			m.width = 80
			out := m.View()
			if !strings.Contains(out, tc.wantSubstr) {
				t.Errorf("View() lacks %q for mode %q:\n%s", tc.wantSubstr, tc.mode, out)
			}
		})
	}
}

// Pinned + locked stack: "[M][鎖] " in the rendered title.
func TestBoardView_StacksPinAndLockBadges(t *testing.T) {
	m, _, arts := newBoardViewFixture(t)
	if err := m.deps.Store.Articles().SetPinned(context.Background(),
		arts[0].ID, m.deps.User.ID, store.RoleAdmin, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if err := m.deps.Store.Articles().SetCommentsMode(context.Background(),
		arts[0].ID, m.deps.User.ID, store.RoleAdmin, store.CommentsModeLocked); err != nil {
		t.Fatalf("lock: %v", err)
	}
	m = newBoardViewModel(m.deps, m.board.ID)
	m.width = 80
	out := m.View()
	if !strings.Contains(out, "[M][鎖]") {
		t.Errorf("View() lacks stacked [M][鎖] prefix:\n%s", out)
	}
}

// ArticleCommentsModeChangedMsg for THIS board re-fetches the list so the
// badge appears without a manual reload.
func TestBoardView_AcceptsCommentsModeChangedForThisBoard(t *testing.T) {
	m, board, arts := newBoardViewFixture(t)
	if err := m.deps.Store.Articles().SetCommentsMode(context.Background(),
		arts[0].ID, m.deps.User.ID, store.RoleAdmin, store.CommentsModeLocked); err != nil {
		t.Fatalf("external lock: %v", err)
	}
	model, _ := m.Update(ArticleCommentsModeChangedMsg{
		BoardID: board.ID, ArticleID: arts[0].ID, Mode: string(store.CommentsModeLocked),
	})
	got := model.(boardViewModel)

	// Find the article in the reloaded slice.
	var found *store.Article
	for _, a := range got.articles {
		if a.ID == arts[0].ID {
			found = a
			break
		}
	}
	if found == nil {
		t.Fatal("article missing from reloaded list")
	}
	if found.CommentsMode != store.CommentsModeLocked {
		t.Errorf("CommentsMode = %q, want locked (refetch missed?)", found.CommentsMode)
	}
}

// ArticleCommentsModeChangedMsg for a DIFFERENT board must NOT refetch.
func TestBoardView_IgnoresCommentsModeChangedForOtherBoard(t *testing.T) {
	m, _, arts := newBoardViewFixture(t)
	// Mutate the DB *after* the initial load, then send a msg for a
	// different board. The model should still see the stale cached state.
	if err := m.deps.Store.Articles().SetCommentsMode(context.Background(),
		arts[0].ID, m.deps.User.ID, store.RoleAdmin, store.CommentsModeLocked); err != nil {
		t.Fatalf("background lock: %v", err)
	}
	model, _ := m.Update(ArticleCommentsModeChangedMsg{
		BoardID: m.board.ID + 999, ArticleID: arts[0].ID, Mode: string(store.CommentsModeLocked),
	})
	got := model.(boardViewModel)
	for _, a := range got.articles {
		if a.ID == arts[0].ID && a.CommentsMode == store.CommentsModeLocked {
			t.Error("model refetched for unrelated-board msg")
		}
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

// updateBV runs Update on a boardViewModel and returns the typed model so
// tests can chain key presses without re-asserting the type each time.
func updateBV(m boardViewModel, key string) boardViewModel {
	model, _ := m.Update(keyOf(key))
	return model.(boardViewModel)
}

func typeBVQuery(m boardViewModel, query string) boardViewModel {
	for _, r := range query {
		m = updateBV(m, string(r))
	}
	return m
}

// pushArticleN registers n fresh helper users and has each push the given
// article once with the given kind. Each push contributes |delta| to the
// cached recommend_score, which is what SortByScoreDesc orders on.
func pushArticleN(t *testing.T, st *store.Store, articleID int64, kind store.PushKind, n int) {
	t.Helper()
	ctx := context.Background()
	for i := range n {
		nick := fmt.Sprintf("bv_p%d_%s_%d", articleID, kind, i)
		pusher := storetest.MustUser(t, st, nick, "")
		if _, err := st.Pushes().Create(ctx, articleID, pusher.ID, pusher.UserID, kind, "."); err != nil {
			t.Fatalf("push %d on article %d: %v", i, articleID, err)
		}
	}
}

func TestBoardView_SlashEntersSearchMode(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	m = updateBV(m, "/")
	if !m.searchActive {
		t.Fatalf("after /: searchActive = false, want true")
	}
	if !m.search.Focused() {
		t.Errorf("textinput must be focused while searching")
	}
}

// While searchActive every action key must type into the input rather than
// trigger its normal behaviour. Mirrors the form-screen rule.
func TestBoardView_SearchSuspendsActionKeys(t *testing.T) {
	for _, key := range []string{"s", "M", "p", "b", "B", "h", "l", "j", "k"} {
		t.Run(key, func(t *testing.T) {
			m, _, _ := newBoardViewFixture(t)
			m = updateBV(m, "/")
			beforeCursor := m.cursor
			beforeSort := m.sort
			m = updateBV(m, key)
			if !m.searchActive {
				t.Fatalf("key %q exited search mode unexpectedly", key)
			}
			if m.cursor != beforeCursor {
				t.Errorf("key %q moved cursor (%d → %d)", key, beforeCursor, m.cursor)
			}
			if m.sort != beforeSort {
				t.Errorf("key %q changed sort (%v → %v)", key, beforeSort, m.sort)
			}
			if m.search.Value() != key {
				t.Errorf("textinput value = %q, want %q (key did not type)", m.search.Value(), key)
			}
		})
	}
}

func TestBoardView_FilterByTitle(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	// Fixture seeds three titles: first, second, third.
	m = updateBV(m, "/")
	m = typeBVQuery(m, "irst") // matches "first" only
	m = updateBV(m, "enter")

	if m.filter != "irst" {
		t.Errorf("filter = %q, want irst", m.filter)
	}
	if len(m.articles) != 1 || m.articles[0].Title != "first" {
		got := make([]string, len(m.articles))
		for i, a := range m.articles {
			got[i] = a.Title
		}
		t.Errorf("filtered articles = %v, want [first]", got)
	}
}

func TestBoardView_EscFromConfirmedClearsFilter(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	m = updateBV(m, "/")
	m = typeBVQuery(m, "irst")
	m = updateBV(m, "enter")
	if len(m.articles) != 1 {
		t.Fatalf("precondition: filtered len=%d, want 1", len(m.articles))
	}

	model, cmd := m.Update(keyOf("esc"))
	m = model.(boardViewModel)
	if cmd != nil {
		if msg, ok := runCmd(cmd).(NavigateMsg); ok {
			t.Errorf("esc on filtered board returned NavigateMsg(%v); should clear filter only", msg)
		}
	}
	if m.filter != "" {
		t.Errorf("filter = %q, want empty", m.filter)
	}
	if len(m.articles) != 3 {
		t.Errorf("len(articles) = %d, want 3 (full list restored)", len(m.articles))
	}
}

// Three articles, distinct scores 5/3/-1. Press s and the list reorders
// hottest-first. Press s again and it returns to newest-first.
func TestBoardView_SortToggle_ByScoreThenBack(t *testing.T) {
	m, _, arts := newBoardViewFixture(t)
	st := m.deps.Store
	// Note: fixture order in arts is creation order [first, second, third];
	// newest-first the visible order is third, second, first.
	pushArticleN(t, st, arts[0].ID, store.PushKindPush, 5)  // first → +5
	pushArticleN(t, st, arts[1].ID, store.PushKindPush, 3)  // second → +3
	pushArticleN(t, st, arts[2].ID, store.PushKindBoo, 1)   // third → -1

	// First press: enter score sort. Expected order first(5), second(3), third(-1).
	m = updateBV(m, "s")
	if m.sort != store.SortByScoreDesc {
		t.Fatalf("after s: sort = %v, want SortByScoreDesc", m.sort)
	}
	wantTitles := []string{"first", "second", "third"}
	if len(m.articles) != 3 {
		t.Fatalf("after s: %d articles, want 3", len(m.articles))
	}
	for i, w := range wantTitles {
		if m.articles[i].Title != w {
			gotTitles := make([]string, len(m.articles))
			for j, a := range m.articles {
				gotTitles[j] = a.Title
			}
			t.Errorf("score-sort [%d] = %q, want %q (full: %v)", i, m.articles[i].Title, w, gotTitles)
			break
		}
	}

	// Second press: back to newest-first. Expected order third, second, first.
	m = updateBV(m, "s")
	if m.sort != store.SortNewestFirst {
		t.Fatalf("after second s: sort = %v, want SortNewestFirst", m.sort)
	}
	wantNewest := []string{"third", "second", "first"}
	for i, w := range wantNewest {
		if m.articles[i].Title != w {
			t.Errorf("newest-sort [%d] = %q, want %q", i, m.articles[i].Title, w)
			break
		}
	}
}

// Sort key while searchActive must NOT cycle the sort — covered by the
// table test above, but assert the post-confirm state explicitly so a
// regression in the gate is caught precisely.
func TestBoardView_SortNotTriggeredDuringSearch(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	m = updateBV(m, "/")
	if m.sort != store.SortNewestFirst {
		t.Fatalf("precondition: sort=%v, want SortNewestFirst", m.sort)
	}
	m = updateBV(m, "s")
	if m.sort != store.SortNewestFirst {
		t.Errorf("s during search changed sort to %v", m.sort)
	}
	if m.search.Value() != "s" {
		t.Errorf("textinput value = %q, want %q", m.search.Value(), "s")
	}
}

// A pinned article must precede higher-scored unpinned articles even in
// score sort — pin is an explicit moderation override.
func TestBoardView_SortPinnedStillFirst(t *testing.T) {
	m, _, arts := newBoardViewFixture(t)
	st := m.deps.Store
	// Make "first" hot (score 10); pin "third" (newest, score 0).
	pushArticleN(t, st, arts[0].ID, store.PushKindPush, 10)
	m.deps.User.Role = store.RoleMod
	if err := st.Articles().SetPinned(context.Background(),
		arts[2].ID, m.deps.User.ID, store.RoleMod, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	m = updateBV(m, "s")
	if len(m.articles) != 3 || m.articles[0].Title != "third" {
		gotTitles := make([]string, len(m.articles))
		for i, a := range m.articles {
			gotTitles[i] = a.Title
		}
		t.Errorf("score sort with pin: got %v, want [third, ...] (pinned leads)", gotTitles)
	}
}

// Sort selection must survive an ArticleAddedMsg broadcast — the new
// article should slot into the score-ordered list, not reset the sort.
func TestBoardView_SortPersistsAcrossArticleAddedMsg(t *testing.T) {
	m, board, arts := newBoardViewFixture(t)
	st := m.deps.Store
	pushArticleN(t, st, arts[0].ID, store.PushKindPush, 5) // first → 5

	m = updateBV(m, "s")
	if m.sort != store.SortByScoreDesc {
		t.Fatalf("precondition: sort=%v", m.sort)
	}

	// Inject a new article with score 0.
	user := m.deps.User
	a4, err := st.Articles().Create(context.Background(), board.ID, user.ID, user.UserID, "fourth", "")
	if err != nil {
		t.Fatalf("create fourth: %v", err)
	}
	model, _ := m.Update(ArticleAddedMsg{
		BoardID: board.ID, ArticleID: a4.ID, AuthorUserID: user.UserID, Title: "fourth",
	})
	m = model.(boardViewModel)

	if m.sort != store.SortByScoreDesc {
		t.Errorf("sort flipped to %v after ArticleAddedMsg", m.sort)
	}
	if len(m.articles) != 4 || m.articles[0].Title != "first" {
		gotTitles := make([]string, len(m.articles))
		for i, a := range m.articles {
			gotTitles[i] = a.Title
		}
		t.Errorf("score order after add: %v, want [first, …] (first still has score 5)", gotTitles)
	}
}

// Combined filter + sort: only articles matching the title query, ordered
// by score within that subset, with pinned-first preserved.
func TestBoardView_FilterAndSortCombined(t *testing.T) {
	m, board, _ := newBoardViewFixture(t)
	st := m.deps.Store

	// Add two more articles whose titles share "match" so we have a
	// non-trivial filtered subset.
	user := m.deps.User
	hot, _ := st.Articles().Create(context.Background(), board.ID, user.ID, user.UserID, "hot match", "")
	cold, _ := st.Articles().Create(context.Background(), board.ID, user.ID, user.UserID, "cold match", "")
	pushArticleN(t, st, hot.ID, store.PushKindPush, 7)
	pushArticleN(t, st, cold.ID, store.PushKindBoo, 2)

	// Reload model so the new articles are visible. The fixture's
	// constructor ran before Create, so we re-build to pick up the new rows.
	m = newBoardViewModel(m.deps, board.ID)

	m = updateBV(m, "/")
	m = typeBVQuery(m, "match")
	m = updateBV(m, "enter")
	m = updateBV(m, "s") // toggle to score sort

	if len(m.articles) != 2 {
		gotTitles := make([]string, len(m.articles))
		for i, a := range m.articles {
			gotTitles[i] = a.Title
		}
		t.Fatalf("filtered+sorted len = %d, want 2 (got: %v)", len(m.articles), gotTitles)
	}
	if m.articles[0].Title != "hot match" || m.articles[1].Title != "cold match" {
		t.Errorf("[%q, %q], want [hot match, cold match]", m.articles[0].Title, m.articles[1].Title)
	}
}

// Cursor anchor: filter narrows the list but the previously-selected
// article is still in the result — the highlight should follow it rather
// than reset to 0.
func TestBoardView_FilterCursorReanchorsByID(t *testing.T) {
	m, _, arts := newBoardViewFixture(t)
	// Move cursor to "first" (the oldest, at the bottom of newest-first).
	m = updateBV(m, "G")
	if m.articles[m.cursor].ID != arts[0].ID {
		t.Fatalf("precondition: cursor on arts[0]=%d, got cursor=%d on id=%d",
			arts[0].ID, m.cursor, m.articles[m.cursor].ID)
	}

	// Filter to "irst" — only "first" matches. After confirm, cursor=0
	// (the test guarantees the user lands on the single match without
	// stale-index dangling-pointer behaviour).
	m = updateBV(m, "/")
	m = typeBVQuery(m, "irst")
	m = updateBV(m, "enter")

	if len(m.articles) != 1 {
		t.Fatalf("filtered len = %d, want 1", len(m.articles))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (only one row)", m.cursor)
	}
	if m.articles[0].ID != arts[0].ID {
		t.Errorf("filtered[0].ID = %d, want %d (first)", m.articles[0].ID, arts[0].ID)
	}
}

// View must surface the search indicator and the sort-mode tag so users
// always see what state they're in.
func TestBoardView_ViewShowsSearchAndSortIndicators(t *testing.T) {
	m, _, _ := newBoardViewFixture(t)
	m.width = 80

	// While searchActive: 搜尋 prompt visible.
	m1 := updateBV(m, "/")
	if !strings.Contains(m1.View(), "搜尋") {
		t.Errorf("active-search view missing 搜尋 prompt")
	}

	// After confirming a filter: indicator with query.
	m2 := typeBVQuery(m1, "first")
	m2 = updateBV(m2, "enter")
	if !strings.Contains(m2.View(), "搜尋: first") {
		t.Errorf("confirmed-filter view missing search indicator")
	}

	// After s: sort indicator.
	m3 := updateBV(m, "s")
	if !strings.Contains(m3.View(), "推文量") {
		t.Errorf("score-sort view missing 推文量 indicator")
	}
}
