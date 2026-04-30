package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// recordingSender records every Send for assertion.
type recordingSender struct{ msgs []tea.Msg }

func (r *recordingSender) Send(m tea.Msg) { r.msgs = append(r.msgs, m) }

// Submitting a valid post sends an ArticleAddedMsg to other sessions and
// returns a NavigateMsg back to ScreenBoardView. The originator is excluded.
func TestPostCompose_SubmitBroadcastsArticleAdded(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")

	br := chat.NewBroker()
	bobRec := &recordingSender{}
	aliceRec := &recordingSender{}
	br.Register(&chat.Session{UserID: alice.ID, UserIDStr: alice.UserID, Program: aliceRec})
	br.Register(&chat.Session{UserID: bob.ID, UserIDStr: bob.UserID, Program: bobRec})

	deps := Deps{Store: st, User: alice, Broker: br}
	m := newPostComposeModel(deps, board.ID)
	m.title.SetValue("hello")
	m.body.SetValue("body line")

	model, cmd := m.submit()
	got := model.(postComposeModel)
	if got.err != "" {
		t.Fatalf("submit err = %q", got.err)
	}
	// The NavigateMsg cmd must point back to board view.
	nav, ok := runCmd(cmd).(NavigateMsg)
	if !ok || nav.To != ScreenBoardView || nav.BoardID != board.ID {
		t.Errorf("nav = %+v", nav)
	}

	// Bob (the other session) must have received an ArticleAddedMsg.
	if len(bobRec.msgs) != 1 {
		t.Fatalf("bob got %d msgs, want 1", len(bobRec.msgs))
	}
	added, ok := bobRec.msgs[0].(ArticleAddedMsg)
	if !ok {
		t.Fatalf("bob got %T, want ArticleAddedMsg", bobRec.msgs[0])
	}
	if added.BoardID != board.ID || added.AuthorUserID != "alice" || added.Title != "hello" {
		t.Errorf("ArticleAddedMsg = %+v", added)
	}

	// Alice (the originator) must NOT receive the broadcast.
	if len(aliceRec.msgs) != 0 {
		t.Errorf("alice (originator) got %d msgs, want 0", len(aliceRec.msgs))
	}

	// The article must actually exist in the DB.
	arts, err := st.Articles().ListByBoard(context.Background(), board.ID, 10)
	if err != nil || len(arts) != 1 || arts[0].Title != "hello" {
		t.Errorf("DB articles: %+v err=%v", arts, err)
	}
}

// Ctrl+I extracts frontmatter from a pasted markdown blob and prefills
// title + body. Pushes section in the input is silently dropped.
func TestPostCompose_CtrlIImportsFrontmatter(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	deps := Deps{Store: st, User: alice, Broker: chat.NewBroker()}
	m := newPostComposeModel(deps, board.ID)
	m.body.SetValue("---\ntitle: Imported\nboard: Welcome\n---\n\nhello world\n")

	m.importFromBody()

	if m.title.Value() != "Imported" {
		t.Errorf("title = %q, want 'Imported'", m.title.Value())
	}
	if m.body.Value() != "hello world" {
		t.Errorf("body = %q, want 'hello world'", m.body.Value())
	}
	if m.err != "" {
		t.Errorf("unexpected err = %q", m.err)
	}
}

func TestPostCompose_CtrlIWithoutFrontmatterErrors(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	deps := Deps{Store: st, User: alice, Broker: chat.NewBroker()}
	m := newPostComposeModel(deps, board.ID)
	m.body.SetValue("just plain text without any frontmatter")

	m.importFromBody()

	if m.err == "" {
		t.Error("expected err for body without frontmatter")
	}
	if m.title.Value() != "" {
		t.Errorf("title was changed despite parse failure: %q", m.title.Value())
	}
}

// Empty title or empty body must not insert and must not broadcast.
func TestPostCompose_EmptyFieldsDoNotSubmit(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Test")
	br := chat.NewBroker()
	rec := &recordingSender{}
	br.Register(&chat.Session{UserID: 99, UserIDStr: "carol", Program: rec})

	deps := Deps{Store: st, User: alice, Broker: br}
	m := newPostComposeModel(deps, board.ID)
	m.title.SetValue("")
	m.body.SetValue("body")

	model, _ := m.submit()
	if model.(postComposeModel).err == "" {
		t.Errorf("expected err for empty title")
	}
	if len(rec.msgs) != 0 {
		t.Errorf("broadcast on failed submit: %d msgs", len(rec.msgs))
	}
}
