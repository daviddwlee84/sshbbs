package tui

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

// boardBannerEditModel is the mod+ form for editing a board's banner.
// Body-only (banners have no title); ctrl+s saves via
// BoardRepo.UpdateBanner, esc cancels back to the board view.
//
// h/l are intentionally NOT bound at the model level so they reach the
// underlying textarea for left/right cursor motion (same convention as
// screen_post_compose / screen_article_edit).
type boardBannerEditModel struct {
	deps    Deps
	boardID int64
	body    textarea.Model
	width   int
	height  int
	err     string
	loadErr error
}

func newBoardBannerEditModel(deps Deps, boardID int64) boardBannerEditModel {
	ta := textarea.New()
	ta.Placeholder = i18n.T(localeOf(deps), i18n.ScreenBoardBannerEditPh)
	ta.CharLimit = 16000
	ta.SetWidth(72)
	ta.SetHeight(14)

	m := boardBannerEditModel{deps: deps, boardID: boardID, body: ta}

	if deps.User == nil {
		m.loadErr = errors.New("not logged in")
		return m
	}
	if !deps.User.Role.AtLeast(store.RoleMod) {
		m.loadErr = store.ErrPermissionDenied
		return m
	}
	board, err := deps.Store.Boards().GetByID(context.Background(), boardID)
	if err != nil {
		m.loadErr = err
		return m
	}
	m.body.SetValue(board.Banner)
	m.body.Focus()
	return m
}

func (m boardBannerEditModel) Init() tea.Cmd { return textarea.Blink }

func (m boardBannerEditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		bodyW := max(40, msg.Width-8)
		bodyH := max(8, msg.Height-12)
		m.body.SetWidth(bodyW)
		m.body.SetHeight(bodyH)
		return m, nil

	case tea.KeyMsg:
		if m.loadErr != nil {
			return m, m.cancelCmd()
		}
		switch msg.String() {
		case "esc":
			return m, m.cancelCmd()
		case "ctrl+s":
			return m.submit()
		}
	}

	if m.loadErr != nil {
		return m, nil
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

func (m boardBannerEditModel) cancelCmd() tea.Cmd {
	id := m.boardID
	return func() tea.Msg {
		if id == 0 {
			return NavigateMsg{To: ScreenBoardList}
		}
		return NavigateMsg{To: ScreenBoardView, BoardID: id}
	}
}

func (m boardBannerEditModel) submit() (tea.Model, tea.Cmd) {
	body := strings.TrimRight(m.body.Value(), "\n")
	ctx := context.Background()
	u := m.deps.User
	if err := m.deps.Store.Boards().UpdateBanner(ctx, m.boardID, u.ID, u.Role, body); err != nil {
		m.err = err.Error()
		return m, nil
	}
	id := m.boardID
	return m, func() tea.Msg {
		return NavigateMsg{To: ScreenBoardView, BoardID: id}
	}
}

func (m boardBannerEditModel) View() string {
	loc := localeOf(m.deps)
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(i18n.T(loc, i18n.ScreenBoardBannerEditTitle)))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		b.WriteString("\n  " + StyleHelp.Render("press any key to go back"))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString("  " + StyleDim.Render("Banner (ANSI / ASCII art):") + "\n")
	b.WriteString("  " + indent(m.body.View(), "  ") + "\n")

	if m.err != "" {
		b.WriteString("\n  " + StyleError.Render("⚠ "+m.err) + "\n")
	}
	b.WriteString("\n  " + StyleHelp.Render(i18n.T(loc, i18n.ScreenBoardBannerEditHelpLine)))
	b.WriteString("\n")
	return b.String()
}
