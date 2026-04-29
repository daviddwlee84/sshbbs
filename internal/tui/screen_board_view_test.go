package tui

import (
	"context"
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
