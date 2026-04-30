package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/daviddwlee84/sshbbs/internal/markdown"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

// articleExportModel renders an article (with optional pushes) as
// markdown for the user to copy. Three exits:
//   - mouse-select in the terminal scrollback (always works; just View)
//   - `c` → emit OSC 52 to push into the local terminal clipboard
//   - `3` → write to data/exports/<userid>/<id>-<unix>.md (registered users)
//
// Read-only screen — no permission gates beyond "guest can't write to disk".
type articleExportModel struct {
	deps      Deps
	articleID int64
	article   *store.Article
	pushes    []*store.Push
	board     *store.Board

	includePushes bool
	width, height int
	scroll        int
	rendered      string
	loadErr       error

	// transient feedback line shown under the help footer.
	statusLine string
}

func newArticleExportModel(deps Deps, articleID int64) articleExportModel {
	ctx := context.Background()
	m := articleExportModel{deps: deps, articleID: articleID}
	a, err := deps.Store.Articles().GetByID(ctx, articleID)
	if err != nil {
		m.loadErr = err
		return m
	}
	pushes, _ := deps.Store.Pushes().ListByArticle(ctx, articleID)
	board, _ := deps.Store.Boards().GetByID(ctx, a.BoardID)
	m.article = a
	m.pushes = pushes
	m.board = board
	m.rerender()
	return m
}

func (m *articleExportModel) rerender() {
	if m.article == nil {
		return
	}
	boardName := ""
	if m.board != nil {
		boardName = m.board.Name
	}
	out, err := markdown.Format(m.article, m.pushes, markdown.FormatOpts{
		IncludePushes: m.includePushes,
		BoardName:     boardName,
	})
	if err != nil {
		m.rendered = "(format error: " + err.Error() + ")"
		return
	}
	m.rendered = out
}

func (m articleExportModel) Init() tea.Cmd { return nil }

func (m articleExportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.loadErr != nil {
			return m, m.backCmd()
		}
		switch msg.String() {
		case "esc", "backspace", "left", "h":
			return m, m.backCmd()
		case "1":
			m.includePushes = false
			m.rerender()
			m.scroll = 0
			m.statusLine = ""
			return m, nil
		case "2":
			m.includePushes = true
			m.rerender()
			m.scroll = 0
			m.statusLine = ""
			return m, nil
		case "3":
			m.writeToDisk()
			return m, nil
		case "c", "C":
			m.copyToClipboard()
			return m, nil
		case "j", "down":
			m.scroll++
		case "k", "up":
			if m.scroll > 0 {
				m.scroll--
			}
		case "g":
			m.scroll = 0
		case "G":
			m.scroll = max(0, m.lineCount()-m.viewportLines())
		case " ", "pgdown":
			m.scroll += 10
		case "b", "pgup":
			m.scroll = max(0, m.scroll-10)
		}
	}
	return m, nil
}

func (m articleExportModel) backCmd() tea.Cmd {
	id := m.articleID
	boardID := int64(0)
	if m.article != nil {
		boardID = m.article.BoardID
	}
	return func() tea.Msg {
		return NavigateMsg{To: ScreenArticleView, ArticleID: id, BoardID: boardID}
	}
}

func (m articleExportModel) lineCount() int {
	return len(strings.Split(m.rendered, "\n"))
}

func (m articleExportModel) viewportLines() int {
	return max(m.height-10, 5)
}

// writeToDisk dumps the rendered markdown to data/exports/<userid>/<id>-<ts>.md.
// Guests are blocked: they have no UserID we'd want to write to disk, and
// they're a shared sentinel account anyway.
func (m *articleExportModel) writeToDisk() {
	if m.deps.User == nil || m.deps.User.Role == store.RoleGuest {
		m.statusLine = "guest 不能寫檔 (read-only)"
		return
	}
	dir := filepath.Join("data", "exports", m.deps.User.UserID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.statusLine = "mkdir: " + err.Error()
		return
	}
	name := fmt.Sprintf("%d-%d.md", m.articleID, time.Now().Unix())
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(m.rendered), 0o644); err != nil {
		m.statusLine = "write: " + err.Error()
		return
	}
	m.statusLine = "已寫入 " + path
}

// copyToClipboard pushes the rendered markdown via OSC 52. Most modern
// terminals honour this; some (Terminal.app default) silently drop it.
func (m *articleExportModel) copyToClipboard() {
	if len(m.rendered) > osc52Limit {
		m.statusLine = fmt.Sprintf("⚠ %dB 超過 OSC52 上限，可能被終端機截斷；改按 3 寫檔", len(m.rendered))
	} else {
		m.statusLine = "已複製到剪貼簿（需終端機支援 OSC 52）"
	}
	_ = emitClipboardOSC52(m.rendered)
}

func (m articleExportModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(StyleHeader.Render(" 匯出 markdown · Export "))
	b.WriteString("\n\n")

	if m.loadErr != nil {
		b.WriteString("  " + StyleError.Render("⚠ "+m.loadErr.Error()) + "\n")
		b.WriteString("\n  " + StyleHelp.Render("press any key to go back"))
		b.WriteString("\n")
		return b.String()
	}

	lines := strings.Split(m.rendered, "\n")
	maxLines := m.viewportLines()
	start := min(m.scroll, len(lines))
	end := min(start+maxLines, len(lines))
	for _, line := range lines[start:end] {
		b.WriteString(line + "\n")
	}
	if start > 0 || end < len(lines) {
		b.WriteString("\n  " + StyleDim.Render(fmt.Sprintf("[lines %d-%d / %d]", start+1, end, len(lines))))
		b.WriteString("\n")
	}

	help := "1 純內文 · 2 含留言 · 3 寫檔 · c 剪貼簿 (OSC52) · j/k 卷動 · Esc 返回"
	b.WriteString("\n  " + StyleHelp.Render(help))
	if m.statusLine != "" {
		b.WriteString("\n  " + StyleSuccess.Render(m.statusLine))
	}
	b.WriteString("\n")
	return b.String()
}
