package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/markdown"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func exportFixture(t *testing.T) (articleExportModel, *store.Article, *store.User, func()) {
	t.Helper()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	board := storetest.MustBoard(t, st, "Welcome")
	a, _ := st.Articles().Create(context.Background(), board.ID, alice.ID, alice.UserID, "Hello", "body line one\nbody line two")
	if _, err := st.Pushes().Create(context.Background(), a.ID, bob.ID, bob.UserID, store.PushKindPush, "great"); err != nil {
		t.Fatalf("seed push: %v", err)
	}
	deps := Deps{Store: st, User: alice, Broker: chat.NewBroker()}
	m := newArticleExportModel(deps, a.ID)
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	cleanup := func() { _ = os.Chdir(cwd) }
	return m, a, alice, cleanup
}

func TestArticleExport_DefaultBodyOnly(t *testing.T) {
	m, _, _, cleanup := exportFixture(t)
	defer cleanup()
	if strings.Contains(m.rendered, markdown.PushesSentinel) {
		t.Errorf("default mode should NOT include pushes sentinel:\n%s", m.rendered)
	}
	if !strings.Contains(m.rendered, "title: Hello") {
		t.Errorf("missing title in render:\n%s", m.rendered)
	}
}

func TestArticleExport_Toggle2IncludesPushes(t *testing.T) {
	m, _, _, cleanup := exportFixture(t)
	defer cleanup()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	got := model.(articleExportModel)
	if !strings.Contains(got.rendered, markdown.PushesSentinel) {
		t.Errorf("after `2`, pushes sentinel missing:\n%s", got.rendered)
	}
	if !strings.Contains(got.rendered, "[bob]") {
		t.Errorf("after `2`, push author missing:\n%s", got.rendered)
	}
}

func TestArticleExport_RoundTripParsesBack(t *testing.T) {
	m, art, _, cleanup := exportFixture(t)
	defer cleanup()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	got := model.(articleExportModel)

	parsed, err := markdown.Parse(got.rendered)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Title != art.Title {
		t.Errorf("title round-trip: %q != %q", parsed.Title, art.Title)
	}
	if parsed.BoardName != "Welcome" {
		t.Errorf("board: %q", parsed.BoardName)
	}
	if len(parsed.Pushes) != 1 || parsed.Pushes[0].Author != "bob" {
		t.Errorf("pushes: %+v", parsed.Pushes)
	}
}

func TestArticleExport_3WritesFile(t *testing.T) {
	m, art, alice, cleanup := exportFixture(t)
	defer cleanup()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	got := model.(articleExportModel)
	if got.statusLine == "" || !strings.Contains(got.statusLine, "data/exports/alice/") {
		t.Errorf("statusLine = %q, want a path under data/exports/alice/", got.statusLine)
	}
	// File must exist on disk.
	matches, err := filepath.Glob(filepath.Join("data", "exports", alice.UserID, "*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("glob: %v matches=%v", err, matches)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	parsed, _ := markdown.Parse(string(body))
	if parsed.Title != art.Title {
		t.Errorf("file title: %q != %q", parsed.Title, art.Title)
	}
}

func TestArticleExport_3GuestBlocked(t *testing.T) {
	st := storetest.New(t)
	guest := storetest.MustUser(t, st, "guest1", "")
	if err := st.Users().SetRole(t.Context(), guest.ID, store.RoleGuest); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	guest.Role = store.RoleGuest
	alice := storetest.MustUser(t, st, "alice", "")
	board := storetest.MustBoard(t, st, "Welcome")
	a, _ := st.Articles().Create(context.Background(), board.ID, alice.ID, alice.UserID, "T", "B")

	deps := Deps{Store: st, User: guest, Broker: chat.NewBroker()}
	m := newArticleExportModel(deps, a.ID)
	tmp := t.TempDir()
	cwd, _ := os.Getwd()
	_ = os.Chdir(tmp)
	defer os.Chdir(cwd)

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	got := model.(articleExportModel)
	if !strings.Contains(got.statusLine, "guest") {
		t.Errorf("statusLine = %q, want guest-blocked message", got.statusLine)
	}
	// Should NOT have written the file.
	if _, err := os.Stat(filepath.Join("data", "exports", "guest1")); err == nil {
		t.Error("guest export dir was created despite refusal")
	}
}

func TestArticleExport_EscReturnsToView(t *testing.T) {
	m, art, _, cleanup := exportFixture(t)
	defer cleanup()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenArticleView || nav.ArticleID != art.ID {
		t.Errorf("nav = %+v, want article view %d", nav, art.ID)
	}
}
