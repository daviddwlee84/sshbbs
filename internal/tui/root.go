package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

// Deps bundles everything every screen may need. Pass-by-value into
// constructors keeps screens self-contained.
type Deps struct {
	Store              *store.Store
	Broker             *chat.Broker // nil during register
	User               *store.User  // nil iff IsRegister
	IsRegister         bool
	MustChangePassword bool // true iff user must rotate password before reaching the main menu
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

	// activeScreen tracks which Screen the current sub corresponds to, so
	// the help overlay can render a context-aware keymap. Set in NewRoot
	// for the bootstrap sub and in navigate for every subsequent swap.
	activeScreen Screen
	// helpVisible is true while the help overlay should render in place of
	// the sub view. Toggled on '?' and dismissed by any key.
	helpVisible bool
}

const toastDuration = 3 * time.Second

// NewRoot builds the Root with the right initial sub-screen.
//
// Priority: IsRegister wins over MustChangePassword wins over the regular
// authenticated main menu — these flags are mutually exclusive in practice
// (set by the SSH password callback / makeProgramHandler).
func NewRoot(deps Deps) Root {
	r := Root{deps: deps}
	switch {
	case deps.IsRegister:
		r.sub = newRegisterModel(deps)
		// No Screen const for register; help is suppressed via deps.IsRegister.
	case deps.MustChangePassword:
		r.sub = newPasswordChangeModel(deps)
		r.activeScreen = ScreenPasswordChange
	default:
		r.sub = newMainMenuModel(deps)
		r.activeScreen = ScreenMainMenu
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
		// Help overlay: any key dismisses, swallow the keypress so the
		// sub doesn't act on it. ctrl+c above is the deliberate exception.
		if m.helpVisible {
			m.helpVisible = false
			return m, nil
		}
		// '?' opens the help overlay on screens that aren't typing-forms.
		if msg.String() == "?" && helpAvailable(m.deps, m.activeScreen) {
			m.helpVisible = true
			return m, nil
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
	if m.helpVisible {
		// Replace the sub view entirely so the keymap reads cleanly. Toast
		// suppressed too — would distract from a help screen.
		return renderHelp(m.activeScreen)
	}
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

// guestWriteBlocked returns an ErrorMsg cmd if a guest is trying to enter
// a write-capable screen, otherwise nil. Layer 1 of the read-only path.
func (m Root) guestWriteBlocked(to Screen) tea.Cmd {
	if m.deps.User == nil || m.deps.User.Role != store.RoleGuest {
		return nil
	}
	switch to {
	case ScreenPostCompose, ScreenWBCompose, ScreenMailCompose, ScreenAdminUsers, ScreenArticleEdit:
		return func() tea.Msg { return ErrorMsg{Err: errors.New("guest 為唯讀帳號 (read-only)")} }
	}
	return nil
}

// navigateNonAdminBlocked returns an ErrorMsg cmd if a non-admin tries
// to reach the admin user-management screen.
func (m Root) navigateNonAdminBlocked(to Screen) tea.Cmd {
	if to != ScreenAdminUsers {
		return nil
	}
	if m.deps.User != nil && m.deps.User.Role == store.RoleAdmin {
		return nil
	}
	return func() tea.Msg { return ErrorMsg{Err: errors.New("管理員專用 (admin only)")} }
}

func (m Root) navigate(n NavigateMsg) (tea.Model, tea.Cmd) {
	if cmd := m.guestWriteBlocked(n.To); cmd != nil {
		return m, cmd
	}
	if cmd := m.navigateNonAdminBlocked(n.To); cmd != nil {
		return m, cmd
	}
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
	case ScreenWBThread:
		sub = newWBThreadModel(m.deps, n.CounterpartyUserID)
	case ScreenOnline:
		sub = newOnlineModel(m.deps)
	case ScreenMailInbox:
		sub = newMailInboxModel(m.deps)
	case ScreenMailThread:
		sub = newMailThreadModel(m.deps, n.MailThreadID)
	case ScreenMailCompose:
		sub = newMailComposeModel(m.deps, n.Recipient, n.MailID)
	case ScreenPasswordChange:
		sub = newPasswordChangeModel(m.deps)
	case ScreenAdminUsers:
		sub = newAdminUsersModel(m.deps)
	case ScreenArticleEdit:
		sub = newArticleEditModel(m.deps, n.ArticleID)
	case ScreenArticleExport:
		sub = newArticleExportModel(m.deps, n.ArticleID)
	case ScreenBoardSplash:
		sub = newBoardSplashModel(m.deps, n.BoardID)
	case ScreenBoardBannerEdit:
		sub = newBoardBannerEditModel(m.deps, n.BoardID)
	default:
		// Unknown screen — leave sub untouched. Later steps add cases.
		return m, nil
	}
	m.sub = sub
	m.activeScreen = n.To
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
