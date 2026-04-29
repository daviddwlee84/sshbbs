package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

// Deps bundles everything every screen may need. Pass-by-value into
// constructors keeps screens self-contained.
type Deps struct {
	Store      *store.Store
	Broker     *chat.Broker // nil during register
	User       *store.User  // nil iff IsRegister
	IsRegister bool
}

// Root is the top-level tea.Model. It owns the active sub-model and
// handles cross-cutting concerns: window size, global hotkeys, navigation,
// and the toast footer.
type Root struct {
	deps   Deps
	sub    tea.Model
	width  int
	height int

	toast      string
	toastUntil time.Time
}

const toastDuration = 3 * time.Second

// NewRoot builds the Root with the right initial sub-screen.
func NewRoot(deps Deps) Root {
	r := Root{deps: deps}
	if deps.IsRegister {
		r.sub = newRegisterModel(deps)
	} else {
		r.sub = newMainMenuModel(deps)
	}
	return r
}

func (m Root) Init() tea.Cmd {
	return m.sub.Init()
}

func (m Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		var cmd tea.Cmd
		m.sub, cmd = m.sub.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		// Global ctrl+c always quits.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Global Ctrl+U opens water balloon inbox (logged-in only).
		if msg.String() == "ctrl+u" && !m.deps.IsRegister {
			return m.navigate(NavigateMsg{To: ScreenWBInbox})
		}

	case NavigateMsg:
		return m.navigate(msg)

	case ErrorMsg:
		if msg.Err != nil {
			m.setToast("⚠ " + msg.Err.Error())
		}
		return m, nil

	case WBIncomingMsg:
		m.setToast("💧 " + msg.FromUserID + ": " + msg.Body)
		// Mark this WB as read so it doesn't replay on reconnect.
		if m.deps.Store != nil && msg.ID != 0 {
			go func(id int64) {
				_ = m.deps.Store.WaterBalloons().MarkRead(context.Background(), id)
			}(msg.ID)
		}
		// fall through so the active screen can also react
	}

	var cmd tea.Cmd
	m.sub, cmd = m.sub.Update(msg)
	return m, cmd
}

func (m Root) View() string {
	v := m.sub.View()
	if m.toast != "" && time.Now().Before(m.toastUntil) {
		v += "\n" + StyleToast.Render(m.toast)
	}
	return v
}

func (m *Root) setToast(s string) {
	m.toast = s
	m.toastUntil = time.Now().Add(toastDuration)
}

func (m Root) navigate(n NavigateMsg) (tea.Model, tea.Cmd) {
	var sub tea.Model
	switch n.To {
	case ScreenMainMenu:
		sub = newMainMenuModel(m.deps)
	case ScreenBoardList:
		sub = newBoardListModel(m.deps)
	case ScreenBoardView:
		sub = newBoardViewModel(m.deps, n.BoardID)
	case ScreenArticleView:
		sub = newArticleViewModel(m.deps, n.ArticleID)
	case ScreenPostCompose:
		sub = newPostComposeModel(m.deps, n.BoardID)
	case ScreenWBInbox:
		sub = newWBInboxModel(m.deps)
	case ScreenWBCompose:
		sub = newWBComposeModel(m.deps, n.Recipient)
	default:
		// Unknown screen — leave sub untouched. Later steps add cases.
		return m, nil
	}
	m.sub = sub
	cmd := sub.Init()
	if m.width > 0 {
		// Forward current size to the new sub so it can lay out immediately.
		var cmd2 tea.Cmd
		m.sub, cmd2 = m.sub.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		if cmd2 != nil {
			cmd = tea.Batch(cmd, cmd2)
		}
	}
	return m, cmd
}
