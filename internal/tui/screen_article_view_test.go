package tui

import (
	"context"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func seedArticle(t *testing.T) (Deps, *store.Article) {
	t.Helper()
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "Alice")
	board := storetest.MustBoard(t, st, "Test")
	a, err := st.Articles().Create(context.Background(), board.ID, user.ID, user.UserID, "title", "line1\nline2\nline3")
	if err != nil {
		t.Fatalf("Create article: %v", err)
	}
	return Deps{Store: st, User: user, Broker: chat.NewBroker()}, a
}

func TestArticleView_LoadsArticle(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)
	if m.loadErr != nil {
		t.Fatalf("loadErr = %v", m.loadErr)
	}
	if m.article.ID != art.ID {
		t.Errorf("article.ID = %d, want %d", m.article.ID, art.ID)
	}
}

func TestArticleView_BackKeys(t *testing.T) {
	deps, art := seedArticle(t)
	for _, key := range []string{"esc", "backspace", "left", "h"} {
		t.Run(key, func(t *testing.T) {
			m := newArticleViewModel(deps, art.ID)
			_, cmd := m.Update(keyOf(key))
			msg := runCmd(cmd)
			nav, ok := msg.(NavigateMsg)
			if !ok {
				t.Fatalf("got %T, want NavigateMsg", msg)
			}
			if nav.To != ScreenBoardView {
				t.Errorf("To = %v, want ScreenBoardView", nav.To)
			}
			if nav.BoardID != art.BoardID {
				t.Errorf("BoardID = %d, want %d", nav.BoardID, art.BoardID)
			}
		})
	}
}

// '+' / '-' / '=' open the inline push input with the right kind.
func TestArticleView_OpenPushKinds(t *testing.T) {
	cases := []struct {
		key  string
		kind store.PushKind
	}{
		{"+", store.PushKindPush},
		{"-", store.PushKindBoo},
		{"=", store.PushKindArrow},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			deps, art := seedArticle(t)
			m := newArticleViewModel(deps, art.ID)
			model, _ := m.Update(keyOf(tc.key))
			got := model.(articleViewModel)
			if !got.pushing {
				t.Errorf("pushing=false after %q", tc.key)
			}
			if got.pushKind != tc.kind {
				t.Errorf("pushKind = %s, want %s", got.pushKind, tc.kind)
			}
		})
	}
}

// `Q` from article view jumps straight to main menu.
func TestArticleView_QuitToMenu(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)
	_, cmd := m.Update(keyOf("Q"))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenMainMenu {
		t.Errorf("To = %v, want ScreenMainMenu", nav.To)
	}
}

// `g` / `G` jump scroll to top / bottom of the body. Body has N lines;
// after `G`, scroll equals the canonical "show last viewport" offset.
func TestArticleView_ScrollGGoToTopBottom(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)
	// Force a known terminal size so viewportLines is deterministic.
	m.height = 24 // viewportLines() = max(24-16, 5) = 8
	m.scroll = 1

	model, _ := m.Update(keyOf("g"))
	got := model.(articleViewModel)
	if got.scroll != 0 {
		t.Errorf("after g: scroll = %d, want 0", got.scroll)
	}

	got.height = 24
	model, _ = got.Update(keyOf("G"))
	got = model.(articleViewModel)
	// fixture has 3 body lines, viewport=8 → max(0, 3-8) = 0
	if got.scroll != 0 {
		t.Errorf("after G (3 lines, viewport=8): scroll = %d, want 0", got.scroll)
	}

	// Long body → G should land on totalLines - viewportLines.
	st := deps.Store
	user := deps.User
	long := ""
	for i := 0; i < 30; i++ {
		long += "line\n"
	}
	a, err := st.Articles().Create(context.Background(), art.BoardID, user.ID, user.UserID, "long", long)
	if err != nil {
		t.Fatalf("create long: %v", err)
	}
	mLong := newArticleViewModel(deps, a.ID)
	mLong.height = 24
	model, _ = mLong.Update(keyOf("G"))
	gotLong := model.(articleViewModel)
	wantScroll := gotLong.bodyLineCount() - gotLong.viewportLines()
	if gotLong.scroll != wantScroll {
		t.Errorf("after G (long body): scroll = %d, want %d", gotLong.scroll, wantScroll)
	}
}

// `[` and `]` navigate to the prev/next sibling article in the same board.
// At the edges (no neighbour), the keys are no-ops (no NavigateMsg emitted).
func TestArticleView_BracketSiblingNavigation(t *testing.T) {
	st := storetest.New(t)
	user := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	ctx := context.Background()
	a, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "first", "")
	b, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "second", "")
	c, _ := st.Articles().Create(ctx, board.ID, user.ID, user.UserID, "third", "")
	deps := Deps{Store: st, User: user, Broker: chat.NewBroker()}

	// Middle article: `[` → first, `]` → third.
	m := newArticleViewModel(deps, b.ID)
	_, cmd := m.Update(keyOf("["))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenArticleView || nav.ArticleID != a.ID {
		t.Errorf("[ from middle: nav = %+v, want ArticleID=%d", nav, a.ID)
	}
	_, cmd = m.Update(keyOf("]"))
	nav = runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenArticleView || nav.ArticleID != c.ID {
		t.Errorf("] from middle: nav = %+v, want ArticleID=%d", nav, c.ID)
	}

	// First article: `[` is a no-op; `]` → second.
	m = newArticleViewModel(deps, a.ID)
	_, cmd = m.Update(keyOf("["))
	if runCmd(cmd) != nil {
		t.Errorf("[ from first should be no-op")
	}
	_, cmd = m.Update(keyOf("]"))
	nav = runCmd(cmd).(NavigateMsg)
	if nav.ArticleID != b.ID {
		t.Errorf("] from first: ArticleID = %d, want %d", nav.ArticleID, b.ID)
	}

	// Last article: `]` is a no-op; `[` → second.
	m = newArticleViewModel(deps, c.ID)
	_, cmd = m.Update(keyOf("]"))
	if runCmd(cmd) != nil {
		t.Errorf("] from last should be no-op")
	}
	_, cmd = m.Update(keyOf("["))
	nav = runCmd(cmd).(NavigateMsg)
	if nav.ArticleID != b.ID {
		t.Errorf("[ from last: ArticleID = %d, want %d", nav.ArticleID, b.ID)
	}
}

// PushAddedMsg for a different article must NOT mutate this view —
// guards against the broadcast-to-all + filter-by-id pattern from Step 7.
func TestArticleView_IgnoresUnrelatedPush(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)

	model, _ := m.Update(PushAddedMsg{
		ArticleID:  art.ID + 999, // some other article
		UserUserID: "bob",
		Kind:       string(store.PushKindPush),
		Body:       "irrelevant",
	})
	got := model.(articleViewModel)
	if len(got.pushes) != 0 {
		t.Errorf("appended unrelated push: got %d, want 0", len(got.pushes))
	}
}

// PushAddedMsg for THIS article triggers a re-fetch so timestamps and
// recommend_score reflect canonical DB state.
func TestArticleView_AcceptsRelatedPush(t *testing.T) {
	deps, art := seedArticle(t)
	// Insert a push directly so the re-fetch sees something.
	if _, err := deps.Store.Pushes().Create(context.Background(), art.ID, deps.User.ID, deps.User.UserID, store.PushKindPush, "from outside"); err != nil {
		t.Fatalf("seed push: %v", err)
	}
	m := newArticleViewModel(deps, art.ID)
	// At this point m.pushes has 1 from the constructor's load.
	if len(m.pushes) != 1 {
		t.Fatalf("constructor loaded %d pushes, want 1", len(m.pushes))
	}

	// Insert a SECOND push externally, then fire PushAddedMsg.
	if _, err := deps.Store.Pushes().Create(context.Background(), art.ID, deps.User.ID, deps.User.UserID, store.PushKindBoo, "another"); err != nil {
		t.Fatalf("seed push 2: %v", err)
	}
	model, _ := m.Update(PushAddedMsg{ArticleID: art.ID, UserUserID: "bob", Kind: "boo", Body: "another"})
	got := model.(articleViewModel)

	if len(got.pushes) != 2 {
		t.Errorf("after PushAddedMsg: %d pushes, want 2", len(got.pushes))
	}
	if got.article.RecommendScore != 0 { // +1 push then -1 boo
		t.Errorf("RecommendScore = %d, want 0", got.article.RecommendScore)
	}
}
