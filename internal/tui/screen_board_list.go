package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

type boardListModel struct {
	deps         Deps
	boards       []*store.Board // canonical full list from the store
	filtered     []*store.Board // boards visible after applying m.filter
	cursor       int            // index into filtered
	width        int
	height       int
	loadErr      error
	search       textinput.Model // input for the active search term
	searchActive bool            // true while focused & accepting keys
	filter       string          // confirmed (non-empty) substring; "" = no filter
}

func newBoardListModel(deps Deps) boardListModel {
	boards, err := deps.Store.Boards().List(context.Background())
	ti := textinput.New()
	ti.Placeholder = i18n.T(localeOf(deps), i18n.ScreenBoardListSearchPlaceholder)
	ti.CharLimit = 32
	ti.Width = 40
	m := boardListModel{deps: deps, boards: boards, loadErr: err, search: ti}
	m.recomputeFilter()
	return m
}

func (m boardListModel) Init() tea.Cmd { return nil }

// recomputeFilter rebuilds m.filtered from m.boards using the current
// m.filter (case-insensitive substring against name/title/description) and
// clamps the cursor so it never points past the end of the new slice.
// strings.ToLower is a no-op for CJK runes, which still match by byte
// substring — fine for the boards we ship today.
func (m *boardListModel) recomputeFilter() {
	if m.filter == "" {
		m.filtered = m.boards
	} else {
		lq := strings.ToLower(m.filter)
		out := make([]*store.Board, 0, len(m.boards))
		for _, b := range m.boards {
			if strings.Contains(strings.ToLower(b.Name), lq) ||
				strings.Contains(strings.ToLower(b.Title), lq) ||
				strings.Contains(strings.ToLower(b.Description), lq) {
				out = append(out, b)
			}
		}
		m.filtered = out
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m boardListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = max(20, msg.Width-12)
		return m, nil
	case tea.KeyMsg:
		// While the search input is focused every printable key types into
		// it; only enter (confirm), esc (cancel), and ctrl+c (handled at
		// Root) escape that. This mirrors the form-screen rule from
		// CLAUDE.md so vim navigation keys (h/j/k/l) and action keys (Q)
		// remain available as text characters.
		if m.searchActive {
			switch msg.String() {
			case "enter":
				m.filter = strings.TrimSpace(m.search.Value())
				m.searchActive = false
				m.search.Blur()
				m.cursor = 0
				m.recomputeFilter()
				return m, nil
			case "esc":
				// Drop the in-flight value AND any prior confirmed filter.
				m.search.SetValue("")
				m.filter = ""
				m.searchActive = false
				m.search.Blur()
				m.cursor = 0
				m.recomputeFilter()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "/":
			m.searchActive = true
			m.search.SetValue(m.filter)
			m.search.CursorEnd()
			m.search.Focus()
			return m, textinput.Blink
		case "esc":
			// While idle, esc clears an active filter first; otherwise it
			// returns to the main menu (matches existing back-navigation).
			if m.filter != "" {
				m.filter = ""
				m.search.SetValue("")
				m.cursor = 0
				m.recomputeFilter()
				return m, nil
			}
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "backspace", "left", "h":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "Q":
			return m, func() tea.Msg { return NavigateMsg{To: ScreenMainMenu} }
		case "up", "k", "[":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j", "]":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			if len(m.filtered) > 0 {
				m.cursor = len(m.filtered) - 1
			}
		case "enter", " ", "right", "l":
			if len(m.filtered) == 0 {
				return m, nil
			}
			b := m.filtered[m.cursor]
			return m, func() tea.Msg { return NavigateMsg{To: ScreenBoardView, BoardID: b.ID} }
		}
	}
	return m, nil
}

func (m boardListModel) View() string {
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenBoardListTitle)))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render(i18n.Tf(loc, i18n.ScreenBoardListLoadFailed, m.loadErr.Error())) + "\n")
		return b.String()
	}

	// Search row: live input above the list while focused; passive
	// "[search: foo · N match …]" indicator when a confirmed filter is active.
	switch {
	case m.searchActive:
		b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenBoardListSearchPrompt)) + m.search.View() + "\n")
		b.WriteString("  " + StyleDim.Render(i18n.Tf(loc, i18n.ScreenBoardListSearchInProgress, len(m.filtered))) + "\n\n")
	case m.filter != "":
		b.WriteString("  " + StyleDim.Render(i18n.Tf(loc, i18n.ScreenBoardListSearchActive, m.filter, len(m.filtered))) + "\n\n")
	}

	if len(m.filtered) == 0 {
		if m.filter != "" {
			b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenBoardListNoMatch)) + "\n")
		} else {
			b.WriteString("  " + StyleDim.Render(i18n.T(loc, i18n.ScreenBoardListNoBoards)) + "\n")
		}
		b.WriteString("\n  " + StyleHelp.Render(i18n.T(loc, i18n.ScreenBoardListHelpLine)) + "\n")
		return b.String()
	}

	const (
		nameW  = 14
		titleW = 32
	)
	header := fmt.Sprintf("  %s  %s  %s",
		PadRight("Name", nameW),
		PadRight("Title", titleW),
		"Description",
	)
	b.WriteString(StyleDim.Render(header) + "\n")

	for i, br := range m.filtered {
		row := fmt.Sprintf("  %s  %s  %s",
			PadRight(br.Name, nameW),
			PadRight(br.Title, titleW),
			Truncate(br.Description, max(20, m.width-nameW-titleW-8)),
		)
		if i == m.cursor {
			b.WriteString(StyleHighlight.Render("▸"+row[1:]) + "\n")
		} else {
			b.WriteString(" " + row + "\n")
		}
	}

	b.WriteString("\n  " + StyleHelp.Render(i18n.T(loc, i18n.ScreenBoardListHelpLine)))
	b.WriteString("\n")
	return b.String()
}
