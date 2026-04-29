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
