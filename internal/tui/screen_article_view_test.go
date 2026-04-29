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
