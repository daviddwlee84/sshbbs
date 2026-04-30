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

// `D` from article view as the author opens the confirm overlay; `y`
// hard-deletes and navigates back to the board view; `n` cancels.
func TestArticleView_DeleteOwn_RequiresConfirm(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)

	model, _ := m.Update(keyOf("D"))
	got := model.(articleViewModel)
	if !got.pendingDelete {
		t.Fatal("D did not enter pending-delete state")
	}
	// Cancel.
	model, _ = got.Update(keyOf("n"))
	got = model.(articleViewModel)
	if got.pendingDelete {
		t.Error("n did not cancel pending-delete")
	}
	// Article must still exist.
	if _, err := deps.Store.Articles().GetByID(context.Background(), art.ID); err != nil {
		t.Errorf("article was deleted on cancel: %v", err)
	}

	// Re-enter and confirm.
	model, _ = got.Update(keyOf("D"))
	got = model.(articleViewModel)
	_, cmd := got.Update(keyOf("y"))
	msg := runCmd(cmd)
	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("got %T, want NavigateMsg after y", msg)
	}
	if nav.To != ScreenBoardView || nav.BoardID != art.BoardID {
		t.Errorf("nav after y = %+v, want ScreenBoardView BoardID=%d", nav, art.BoardID)
	}
	if _, err := deps.Store.Articles().GetByID(context.Background(), art.ID); err == nil {
		t.Error("article still exists after y confirm")
	}
}

// `D` from a non-author non-mod must NOT enter pending-delete state.
// (Renders an inline error string and stays on the screen.)
func TestArticleView_DeleteRefusedForNonOwnerNonMod(t *testing.T) {
	deps, art := seedArticle(t)
	// Replace deps.User with a different non-mod user.
	bob := storetest.MustUser(t, deps.Store, "bob", "Bob")
	deps2 := deps
	deps2.User = bob
	m := newArticleViewModel(deps2, art.ID)

	model, _ := m.Update(keyOf("D"))
	got := model.(articleViewModel)
	if got.pendingDelete {
		t.Error("D should not enter pending-delete for non-owner non-mod")
	}
	if got.err == "" {
		t.Error("expected an err message explaining the refusal")
	}
}

// `D` as a mod (not author) must enter pending-delete and successful
// confirmation deletes the article.
func TestArticleView_ModCanDeleteAnyones(t *testing.T) {
	deps, art := seedArticle(t)
	bob := storetest.MustUser(t, deps.Store, "bob", "Bob")
	bob.Role = store.RoleMod
	if err := deps.Store.Users().SetRole(context.Background(), bob.ID, store.RoleMod); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	deps2 := deps
	deps2.User = bob
	m := newArticleViewModel(deps2, art.ID)

	model, _ := m.Update(keyOf("D"))
	got := model.(articleViewModel)
	if !got.pendingDelete {
		t.Fatal("mod should enter pending-delete on D")
	}
	_, cmd := got.Update(keyOf("y"))
	if _, ok := runCmd(cmd).(NavigateMsg); !ok {
		t.Error("mod confirm did not produce NavigateMsg")
	}
	if _, err := deps.Store.Articles().GetByID(context.Background(), art.ID); err == nil {
		t.Error("article still exists after mod delete")
	}
}

// `p` advances the push cursor; `P` reverses; the cycle wraps via the
// "no selection" sentinel state (cursor=-1) so D defaults to article delete.
func TestArticleView_PushCursor_pPCycles(t *testing.T) {
	deps, art := seedArticle(t)
	ctx := context.Background()
	// Seed three pushes so the cursor has somewhere to go.
	for i := 0; i < 3; i++ {
		if _, err := deps.Store.Pushes().Create(ctx, art.ID, deps.User.ID, deps.User.UserID, store.PushKindPush, "msg"); err != nil {
			t.Fatalf("seed push: %v", err)
		}
	}
	m := newArticleViewModel(deps, art.ID)
	if m.pushCursor != -1 {
		t.Fatalf("initial pushCursor = %d, want -1", m.pushCursor)
	}

	// p p p p → 0, 1, 2, -1
	wantSeq := []int{0, 1, 2, -1}
	for i, want := range wantSeq {
		model, _ := m.Update(keyOf("p"))
		m = model.(articleViewModel)
		if m.pushCursor != want {
			t.Errorf("after %d×p: cursor = %d, want %d", i+1, m.pushCursor, want)
		}
	}

	// From -1: P → 2, P → 1, P → 0, P → -1
	wantSeq = []int{2, 1, 0, -1}
	for i, want := range wantSeq {
		model, _ := m.Update(keyOf("P"))
		m = model.(articleViewModel)
		if m.pushCursor != want {
			t.Errorf("after %d×P: cursor = %d, want %d", i+1, m.pushCursor, want)
		}
	}
}

// `p` is a no-op when there are no pushes — cursor stays at -1.
func TestArticleView_PushCursor_NoPushesIsNoop(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)
	for _, key := range []string{"p", "P"} {
		model, _ := m.Update(keyOf(key))
		got := model.(articleViewModel)
		if got.pushCursor != -1 {
			t.Errorf("%s with no pushes: cursor = %d, want -1", key, got.pushCursor)
		}
	}
}

// `D` while cursor is on a push deletes that push and reverts the score.
// The article remains, the push is gone, and the cursor clamps to -1
// when the last push is removed.
func TestArticleView_DeleteCursoredPush(t *testing.T) {
	deps, art := seedArticle(t)
	ctx := context.Background()
	// Seed one push of kind=push so the score becomes +1.
	p, err := deps.Store.Pushes().Create(ctx, art.ID, deps.User.ID, deps.User.UserID, store.PushKindPush, "+1 vote")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	pre, _ := deps.Store.Articles().GetByID(ctx, art.ID)
	if pre.RecommendScore != 1 {
		t.Fatalf("post-seed score = %d, want 1", pre.RecommendScore)
	}

	m := newArticleViewModel(deps, art.ID)
	model, _ := m.Update(keyOf("p"))
	m = model.(articleViewModel)
	if m.pushCursor != 0 {
		t.Fatalf("p should land cursor=0, got %d", m.pushCursor)
	}
	model, _ = m.Update(keyOf("D"))
	m = model.(articleViewModel)
	if !m.pendingDelete {
		t.Fatal("D should arm the confirm overlay")
	}
	model, cmd := m.Update(keyOf("y"))
	m = model.(articleViewModel)
	// y on push delete must not navigate away — the article view stays mounted.
	if cmd != nil {
		if msg := runCmd(cmd); msg != nil {
			if _, isNav := msg.(NavigateMsg); isNav {
				t.Errorf("push delete should not produce NavigateMsg; got %+v", msg)
			}
		}
	}
	if m.pushCursor != -1 {
		t.Errorf("after deleting last push: cursor = %d, want -1 (clamped)", m.pushCursor)
	}
	if len(m.pushes) != 0 {
		t.Errorf("after delete: %d pushes remain, want 0", len(m.pushes))
	}
	if m.article.RecommendScore != 0 {
		t.Errorf("score after revert = %d, want 0", m.article.RecommendScore)
	}
	// And the article itself must still exist (not cascade-deleted).
	if _, err := deps.Store.Articles().GetByID(ctx, art.ID); err != nil {
		t.Errorf("article gone after push delete: %v", err)
	}
	// Push truly gone from DB.
	pushes, _ := deps.Store.Pushes().ListByArticle(ctx, art.ID)
	for _, q := range pushes {
		if q.ID == p.ID {
			t.Errorf("push %d still exists in DB", p.ID)
		}
	}
}

// `D` on someone else's push is refused for a non-mod, even if the user
// owns the article. This is the strict-ownership case (mod can override).
func TestArticleView_DeleteCursoredPush_NonOwnerNonModRefused(t *testing.T) {
	deps, art := seedArticle(t)
	ctx := context.Background()
	// alice owns the article. bob (also non-mod) leaves a push.
	bob := storetest.MustUser(t, deps.Store, "bob", "")
	if _, err := deps.Store.Pushes().Create(ctx, art.ID, bob.ID, bob.UserID, store.PushKindBoo, "-1"); err != nil {
		t.Fatalf("seed bob push: %v", err)
	}
	m := newArticleViewModel(deps, art.ID)
	// cursor=0 → bob's push.
	model, _ := m.Update(keyOf("p"))
	m = model.(articleViewModel)
	model, _ = m.Update(keyOf("D"))
	got := model.(articleViewModel)
	if got.pendingDelete {
		t.Error("alice should not be able to delete bob's push (non-mod)")
	}
	if got.err == "" {
		t.Error("expected refusal message")
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

func TestArticleView_EOpensEdit_AsAuthor(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)
	_, cmd := m.Update(keyOf("E"))
	msg := runCmd(cmd)
	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("got %T, want NavigateMsg", msg)
	}
	if nav.To != ScreenArticleEdit || nav.ArticleID != art.ID {
		t.Errorf("nav = %+v, want article-edit %d", nav, art.ID)
	}
}

func TestArticleView_ERefusedForNonOwnerNonMod(t *testing.T) {
	deps, art := seedArticle(t)
	// Switch the user to bob (not the author).
	bob := storetest.MustUser(t, deps.Store, "bob", "")
	deps.User = bob
	m := newArticleViewModel(deps, art.ID)
	model, cmd := m.Update(keyOf("E"))
	if cmd != nil {
		// If something IS returned, it must NOT be a NavigateMsg to ArticleEdit.
		if nav, ok := runCmd(cmd).(NavigateMsg); ok && nav.To == ScreenArticleEdit {
			t.Errorf("non-owner non-mod was allowed to navigate to edit screen")
		}
	}
	if got := model.(articleViewModel); got.err == "" {
		t.Errorf("expected err set after refused E key, got %q", got.err)
	}
}

func TestArticleView_YOpensExport(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)
	_, cmd := m.Update(keyOf("y"))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenArticleExport || nav.ArticleID != art.ID {
		t.Errorf("nav = %+v, want article-export %d", nav, art.ID)
	}
}

func TestArticleView_AcceptsArticleUpdated(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)

	// Externally update the article (simulating another session's edit).
	if err := deps.Store.Articles().Update(context.Background(), art.ID, deps.User.ID, deps.User.Role, "edited title", "edited body"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Fire ArticleUpdatedMsg for THIS article — model should refetch.
	model, _ := m.Update(ArticleUpdatedMsg{ArticleID: art.ID})
	got := model.(articleViewModel)
	if got.article.Title != "edited title" || got.article.Body != "edited body" {
		t.Errorf("after ArticleUpdatedMsg: title/body = %q/%q want edited",
			got.article.Title, got.article.Body)
	}
}

func TestArticleView_IgnoresUnrelatedArticleUpdated(t *testing.T) {
	deps, art := seedArticle(t)
	m := newArticleViewModel(deps, art.ID)
	originalTitle := m.article.Title

	model, _ := m.Update(ArticleUpdatedMsg{ArticleID: art.ID + 999})
	got := model.(articleViewModel)
	if got.article.Title != originalTitle {
		t.Errorf("title changed for unrelated update: %q", got.article.Title)
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
