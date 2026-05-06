package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func newBoardListFixture(t *testing.T) (boardListModel, *store.Store) {
	t.Helper()
	st := storetest.New(t)
	deps := Deps{
		Store: st,
		User:  &store.User{ID: 1, UserID: "alice"},
	}
	return newBoardListModel(deps), st
}

func TestBoardList_LoadsDefaultBoards(t *testing.T) {
	m, _ := newBoardListFixture(t)
	if m.loadErr != nil {
		t.Fatalf("loadErr = %v", m.loadErr)
	}
	if len(m.boards) != 3 {
		t.Errorf("loaded %d boards, want 3", len(m.boards))
	}
}

// All four "back" keys (esc, backspace, left, h) must navigate to main menu —
// this is the new h/l parity from Step 12.
func TestBoardList_BackKeys(t *testing.T) {
	for _, key := range []string{"esc", "backspace", "left", "h"} {
		t.Run(key, func(t *testing.T) {
			m, _ := newBoardListFixture(t)
			_, cmd := m.Update(keyOf(key))
			msg := runCmd(cmd)
			nav, ok := msg.(NavigateMsg)
			if !ok {
				t.Fatalf("got %T, want NavigateMsg", msg)
			}
			if nav.To != ScreenMainMenu {
				t.Errorf("To = %v, want ScreenMainMenu", nav.To)
			}
		})
	}
}

// All four "forward" keys (enter, space, right, l) must open the cursored board.
func TestBoardList_ForwardKeys(t *testing.T) {
	for _, key := range []string{"enter", " ", "right", "l"} {
		t.Run(key, func(t *testing.T) {
			m, _ := newBoardListFixture(t)
			_, cmd := m.Update(keyOf(key))
			msg := runCmd(cmd)
			nav, ok := msg.(NavigateMsg)
			if !ok {
				t.Fatalf("got %T, want NavigateMsg", msg)
			}
			if nav.To != ScreenBoardView {
				t.Errorf("To = %v, want ScreenBoardView", nav.To)
			}
			// cursor=0 → ChitChat (alphabetically first by COLLATE NOCASE)
			if nav.BoardID != m.boards[0].ID {
				t.Errorf("BoardID = %d, want %d (ChitChat)", nav.BoardID, m.boards[0].ID)
			}
		})
	}
}

// `[` and `]` are PTT-style aliases for k/j (cursor up/down).
func TestBoardList_BracketCursorAliases(t *testing.T) {
	m, _ := newBoardListFixture(t)
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}
	model, _ := m.Update(keyOf("]"))
	m = model.(boardListModel)
	if m.cursor != 1 {
		t.Errorf("after ]: cursor = %d, want 1", m.cursor)
	}
	model, _ = m.Update(keyOf("["))
	m = model.(boardListModel)
	if m.cursor != 0 {
		t.Errorf("after [: cursor = %d, want 0", m.cursor)
	}
}

// `Q` quits the list back to main menu.
func TestBoardList_QuitToMenu(t *testing.T) {
	m, _ := newBoardListFixture(t)
	_, cmd := m.Update(keyOf("Q"))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenMainMenu {
		t.Errorf("To = %v, want ScreenMainMenu", nav.To)
	}
}

func TestBoardList_CursorTracksOpen(t *testing.T) {
	m, _ := newBoardListFixture(t)
	// Move cursor to "Test" (index 1).
	model, _ := m.Update(keyOf("j"))
	m = model.(boardListModel)

	_, cmd := m.Update(keyOf("enter"))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.BoardID != m.boards[1].ID {
		t.Errorf("BoardID = %d, want %d (Test)", nav.BoardID, m.boards[1].ID)
	}
}

// View must render board names with CJK titles intact and not panic on
// zero-width terminal (some test runners report 0,0 for size).
func TestBoardList_ViewRendersWithoutPanic(t *testing.T) {
	m, _ := newBoardListFixture(t)
	out := m.View()
	for _, want := range []string{"看板列表", "ChitChat", "Test", "Welcome"} {
		if !contains(out, want) {
			t.Errorf("View missing %q", want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var _ tea.Model = boardListModel{}

// updateBL is a tiny helper that runs Update and returns the typed model so
// callers can chain key presses without re-asserting the type each time.
func updateBL(m boardListModel, key string) boardListModel {
	model, _ := m.Update(keyOf(key))
	return model.(boardListModel)
}

// typeQuery feeds a literal string into the search textinput one rune at a
// time. We use single-rune KeyMsgs because that's the shape the SSH terminal
// actually emits, and it exercises Update for each keystroke.
func typeQuery(m boardListModel, query string) boardListModel {
	for _, r := range query {
		m = updateBL(m, string(r))
	}
	return m
}

func TestBoardList_SlashEntersSearchMode(t *testing.T) {
	m, _ := newBoardListFixture(t)
	m = updateBL(m, "/")
	if !m.searchActive {
		t.Fatalf("after /: searchActive = false, want true")
	}
	if !m.search.Focused() {
		t.Errorf("textinput must be focused while searching")
	}
}

func TestBoardList_SearchConfirmedFiltersList(t *testing.T) {
	m, _ := newBoardListFixture(t)
	m = updateBL(m, "/")
	m = typeQuery(m, "Test")
	m = updateBL(m, "enter")

	if m.searchActive {
		t.Errorf("after enter: searchActive should be false")
	}
	if m.filter != "Test" {
		t.Errorf("filter = %q, want %q", m.filter, "Test")
	}
	if len(m.filtered) != 1 || m.filtered[0].Name != "Test" {
		t.Errorf("filtered = %v, want [Test]", boardNames(m.filtered))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (reset on confirm)", m.cursor)
	}
}

// While searchActive, every printable key — including the vim navigation
// keys h/j/k/l and the back-keys Q/esc-already-handled — must type into the
// textinput, NOT trigger their normal action. This mirrors the form-screen
// rule from CLAUDE.md.
func TestBoardList_SearchSuspendsHJKLNav(t *testing.T) {
	for _, key := range []string{"h", "j", "k", "l", "Q"} {
		t.Run(key, func(t *testing.T) {
			m, _ := newBoardListFixture(t)
			m = updateBL(m, "/")
			beforeCursor := m.cursor
			m = updateBL(m, key)
			if !m.searchActive {
				t.Fatalf("key %q exited search mode unexpectedly", key)
			}
			if m.cursor != beforeCursor {
				t.Errorf("key %q moved cursor (%d → %d) instead of typing", key, beforeCursor, m.cursor)
			}
			if m.search.Value() != key {
				t.Errorf("textinput value = %q, want %q", m.search.Value(), key)
			}
		})
	}
}

func TestBoardList_EscFromSearchingClearsBoth(t *testing.T) {
	m, _ := newBoardListFixture(t)

	// Confirm a filter first so we can prove esc also wipes the prior state.
	m = updateBL(m, "/")
	m = typeQuery(m, "Test")
	m = updateBL(m, "enter")

	// Re-enter search and bail with esc.
	m = updateBL(m, "/")
	m = typeQuery(m, "abc")
	m = updateBL(m, "esc")

	if m.searchActive {
		t.Errorf("esc must exit search mode")
	}
	if m.filter != "" {
		t.Errorf("filter = %q, want empty (esc-from-searching also clears prior filter)", m.filter)
	}
	if len(m.filtered) != 3 {
		t.Errorf("filtered length = %d, want 3 (full list restored)", len(m.filtered))
	}
}

func TestBoardList_EscFromConfirmedClearsFilter(t *testing.T) {
	m, _ := newBoardListFixture(t)
	m = updateBL(m, "/")
	m = typeQuery(m, "Test")
	m = updateBL(m, "enter")

	// Confirmed state: filter active, search not focused.
	if m.searchActive || m.filter != "Test" {
		t.Fatalf("precondition failed: searchActive=%v filter=%q", m.searchActive, m.filter)
	}

	// esc from idle-but-filtered must clear the filter, not navigate back.
	model, cmd := m.Update(keyOf("esc"))
	m = model.(boardListModel)
	if cmd != nil {
		// If a command came back, it must NOT be a NavigateMsg.
		if msg, ok := runCmd(cmd).(NavigateMsg); ok {
			t.Errorf("esc on filtered list returned NavigateMsg(%v); should clear filter only", msg)
		}
	}
	if m.filter != "" {
		t.Errorf("filter = %q, want empty after esc", m.filter)
	}
	if len(m.filtered) != 3 {
		t.Errorf("filtered length = %d, want 3 (full list restored)", len(m.filtered))
	}
}

// Cursor at index 2 (last board) must reclamp to a valid position when the
// filter narrows the list to fewer rows.
func TestBoardList_FilterCursorClamps(t *testing.T) {
	m, _ := newBoardListFixture(t)
	m = updateBL(m, "G") // jump to last (cursor=2)
	if m.cursor != 2 {
		t.Fatalf("precondition: cursor=%d, want 2", m.cursor)
	}

	m = updateBL(m, "/")
	m = typeQuery(m, "Test")
	m = updateBL(m, "enter")

	if m.cursor != 0 {
		t.Errorf("after narrowing filter, cursor = %d, want 0", m.cursor)
	}
}

// The filter searches Description as well — a query that doesn't appear in
// any Name or Title but does appear in a Description must still match.
func TestBoardList_FilterByDescription(t *testing.T) {
	m, _ := newBoardListFixture(t)
	m = updateBL(m, "/")
	m = typeQuery(m, "kind") // Welcome's description has "Be kind." (ChitChat)
	m = updateBL(m, "enter")

	if len(m.filtered) != 1 || m.filtered[0].Name != "ChitChat" {
		t.Errorf("filter %q matched %v, want [ChitChat] (matched via description)", "kind", boardNames(m.filtered))
	}
}

// CJK-substring matching: a query containing the character 「閒」 should hit
// ChitChat board whose Title is 「閒聊」.
func TestBoardList_FilterCJK(t *testing.T) {
	m, _ := newBoardListFixture(t)
	m = updateBL(m, "/")
	m = typeQuery(m, "閒")
	m = updateBL(m, "enter")

	if len(m.filtered) != 1 || m.filtered[0].Name != "ChitChat" {
		t.Errorf("CJK filter matched %v, want [ChitChat]", boardNames(m.filtered))
	}
}

// View must surface the search indicator when a filter is active, and the
// textinput when searchActive — so users always see what state they're in.
func TestBoardList_ViewShowsSearchIndicator(t *testing.T) {
	m, _ := newBoardListFixture(t)
	m = updateBL(m, "/")
	out := m.View()
	if !contains(out, "搜尋") {
		t.Errorf("active-search view missing 搜尋 prompt:\n%s", out)
	}

	m = typeQuery(m, "Test")
	m = updateBL(m, "enter")
	out = m.View()
	if !contains(out, "搜尋: Test") {
		t.Errorf("confirmed-filter view missing search indicator:\n%s", out)
	}
}

func boardNames(bs []*store.Board) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Name
	}
	return out
}
