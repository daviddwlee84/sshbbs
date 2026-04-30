package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// editFixture seeds a Test board with one article authored by alice and
// returns the model under test set up for `editor`.
func editFixture(t *testing.T, editor *store.User) (articleEditModel, *store.Article, *chat.Broker) {
	t.Helper()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	if editor == nil {
		editor = alice
	} else {
		// Insert the editor identity if not alice (so subsequent role-set works).
		if editor.UserID != "alice" {
			u := storetest.MustUser(t, st, editor.UserID, editor.Nickname)
			if editor.Role != "" && editor.Role != store.RoleUser {
				if err := st.Users().SetRole(t.Context(), u.ID, editor.Role); err != nil {
					t.Fatalf("SetRole: %v", err)
				}
				u.Role = editor.Role
			}
			editor = u
		}
	}
	board := storetest.MustBoard(t, st, "Test")
	a, err := st.Articles().Create(context.Background(), board.ID, alice.ID, alice.UserID, "old", "old body")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	br := chat.NewBroker()
	deps := Deps{Store: st, User: editor, Broker: br}
	m := newArticleEditModel(deps, a.ID)
	return m, a, br
}

func TestArticleEdit_PrefillsFromDB(t *testing.T) {
	m, _, _ := editFixture(t, nil)
	if m.title.Value() != "old" {
		t.Errorf("title prefill = %q, want 'old'", m.title.Value())
	}
	if m.body.Value() != "old body" {
		t.Errorf("body prefill = %q, want 'old body'", m.body.Value())
	}
	if m.loadErr != nil {
		t.Errorf("loadErr = %v, want nil", m.loadErr)
	}
}

func TestArticleEdit_NonOwnerNonModBlockedAtConstruction(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(context.Background(), board.ID, alice.ID, alice.UserID, "x", "y")

	deps := Deps{Store: st, User: bob, Broker: chat.NewBroker()}
	m := newArticleEditModel(deps, a.ID)
	if m.loadErr == nil {
		t.Fatal("expected loadErr for non-owner non-mod, got nil")
	}
}

func TestArticleEdit_ModCanEdit(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	mod := storetest.MustUser(t, st, "modder", "")
	if err := st.Users().SetRole(t.Context(), mod.ID, store.RoleMod); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	mod.Role = store.RoleMod
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(context.Background(), board.ID, alice.ID, alice.UserID, "old", "old body")

	deps := Deps{Store: st, User: mod, Broker: chat.NewBroker()}
	m := newArticleEditModel(deps, a.ID)
	if m.loadErr != nil {
		t.Fatalf("mod blocked: %v", m.loadErr)
	}
	if m.title.Value() != "old" {
		t.Errorf("title = %q", m.title.Value())
	}
}

func TestArticleEdit_HLNotBound(t *testing.T) {
	// h and l must reach the textinput / textarea, not navigate away.
	// Verify Update with `h` returns the same editor model (sub).
	m, _, _ := editFixture(t, nil)
	for _, key := range []string{"h", "l"} {
		var msg tea.KeyMsg
		switch key {
		case "h":
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")}
		case "l":
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}
		}
		got, _ := m.Update(msg)
		if _, ok := got.(articleEditModel); !ok {
			t.Errorf("key %q: model became %T, want articleEditModel (h/l must not navigate)", key, got)
		}
	}
}

func TestArticleEdit_CtrlSSavesAndBroadcasts(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Test")
	a, _ := st.Articles().Create(context.Background(), board.ID, alice.ID, alice.UserID, "old", "old body")

	br := chat.NewBroker()
	bobRec := &recordingSender{}
	br.Register(&chat.Session{UserID: bob.ID, UserIDStr: bob.UserID, Program: bobRec})

	deps := Deps{Store: st, User: alice, Broker: br}
	m := newArticleEditModel(deps, a.ID)
	m.title.SetValue("new title")
	m.body.SetValue("new body content")

	model, cmd := m.submit()
	got := model.(articleEditModel)
	if got.err != "" {
		t.Fatalf("submit err = %q", got.err)
	}
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenArticleView || nav.ArticleID != a.ID {
		t.Errorf("nav = %+v want article view %d", nav, a.ID)
	}

	// DB reflects the update.
	updated, _ := st.Articles().GetByID(context.Background(), a.ID)
	if updated.Title != "new title" || updated.Body != "new body content" {
		t.Errorf("DB title/body = %q/%q", updated.Title, updated.Body)
	}
	if !updated.UpdatedAt.Valid {
		t.Error("UpdatedAt not set")
	}

	// Bob (other session) must have got an ArticleUpdatedMsg.
	if len(bobRec.msgs) != 1 {
		t.Fatalf("bob got %d msgs, want 1", len(bobRec.msgs))
	}
	upd, ok := bobRec.msgs[0].(ArticleUpdatedMsg)
	if !ok || upd.ArticleID != a.ID {
		t.Errorf("got %+v, want ArticleUpdatedMsg{ArticleID: %d}", bobRec.msgs[0], a.ID)
	}
}

func TestArticleEdit_EscapeReturnsToArticleView(t *testing.T) {
	m, a, _ := editFixture(t, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenArticleView || nav.ArticleID != a.ID {
		t.Errorf("esc nav = %+v, want article view %d", nav, a.ID)
	}
}

func TestArticleEdit_SubmitRequiresFields(t *testing.T) {
	m, _, _ := editFixture(t, nil)
	m.title.SetValue("")
	m.body.SetValue("body")
	model, _ := m.submit()
	if model.(articleEditModel).err == "" {
		t.Error("expected err for empty title")
	}

	m, _, _ = editFixture(t, nil)
	m.title.SetValue("title")
	m.body.SetValue("")
	model, _ = m.submit()
	if model.(articleEditModel).err == "" {
		t.Error("expected err for empty body")
	}
}
