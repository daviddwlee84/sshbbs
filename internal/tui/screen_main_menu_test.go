package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

// keyOf builds a tea.KeyMsg whose .String() matches the wanted form.
// bubbletea exposes typed key constants; we use Runes for ASCII letters
// and named Type values for special keys.
func keyOf(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// runCmd invokes the tea.Cmd returned by Update and returns the resulting
// tea.Msg (or nil if cmd is nil).
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func newMainMenuFixture() (mainMenuModel, Deps) {
	deps := Deps{
		User: &store.User{ID: 1, UserID: "alice", Nickname: "Alice"},
	}
	return newMainMenuModel(deps), deps
}

func TestMainMenu_CursorMovement(t *testing.T) {
	m, _ := newMainMenuFixture()
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}
	for _, key := range []string{"j", "down"} {
		m, _ = updateMM(m, keyOf(key))
	}
	if m.cursor != 2 {
		t.Errorf("after 2x down: cursor = %d, want 2", m.cursor)
	}
	m, _ = updateMM(m, keyOf("k"))
	m, _ = updateMM(m, keyOf("up"))
	if m.cursor != 0 {
		t.Errorf("after 2x up: cursor = %d, want 0", m.cursor)
	}
}

func TestMainMenu_CursorClamps(t *testing.T) {
	m, _ := newMainMenuFixture()
	for i := 0; i < 100; i++ {
		m, _ = updateMM(m, keyOf("k")) // should not go below 0
	}
	if m.cursor != 0 {
		t.Errorf("cursor underflowed: %d", m.cursor)
	}
	for i := 0; i < 100; i++ {
		m, _ = updateMM(m, keyOf("j")) // should not go past last
	}
	if m.cursor != len(m.items)-1 {
		t.Errorf("cursor overflowed: %d, want %d", m.cursor, len(m.items)-1)
	}
}

// "l" / right / Enter all open the cursored item — verifies the new
// h/l navigation parity for forward-direction.
func TestMainMenu_ForwardKeys(t *testing.T) {
	for _, key := range []string{"enter", " ", "l", "right"} {
		t.Run(key, func(t *testing.T) {
			m, _ := newMainMenuFixture()
			// cursor=0 -> first item routes to ScreenBoardList
			_, cmd := updateMM(m, keyOf(key))
			msg := runCmd(cmd)
			nav, ok := msg.(NavigateMsg)
			if !ok {
				t.Fatalf("got %T, want NavigateMsg", msg)
			}
			if nav.To != ScreenBoardList {
				t.Errorf("To = %v, want ScreenBoardList", nav.To)
			}
		})
	}
}

func TestMainMenu_NumericShortcuts(t *testing.T) {
	cases := []struct {
		key  string
		want Screen
	}{
		{"1", ScreenBoardList},
		{"2", ScreenWBInbox},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			m, _ := newMainMenuFixture()
			_, cmd := updateMM(m, keyOf(tc.key))
			msg := runCmd(cmd)
			nav, ok := msg.(NavigateMsg)
			if !ok {
				t.Fatalf("got %T, want NavigateMsg", msg)
			}
			if nav.To != tc.want {
				t.Errorf("To = %v, want %v", nav.To, tc.want)
			}
		})
	}
}

// Quit shortcut: '3' jumps to "Quit" and emits tea.Quit, not a NavigateMsg.
func TestMainMenu_QuitShortcut(t *testing.T) {
	m, _ := newMainMenuFixture()
	_, cmd := updateMM(m, keyOf("3"))
	msg := runCmd(cmd)
	if _, isQuit := msg.(tea.QuitMsg); !isQuit {
		t.Errorf("got %T, want tea.QuitMsg", msg)
	}
}

// Helper to type-assert past the tea.Model interface back to mainMenuModel.
func updateMM(m mainMenuModel, msg tea.Msg) (mainMenuModel, tea.Cmd) {
	model, cmd := m.Update(msg)
	return model.(mainMenuModel), cmd
}
