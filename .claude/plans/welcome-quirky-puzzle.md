# Welcome 種子文 + 文章 Markdown round-trip + 編輯/匯出/匯入

## Context

目前 `Welcome` 看板是空的 — `internal/store/migrations/0002_seed_boards.sql` 只建了三個看板，沒有任何種子文章。我們希望：

1. Welcome 看板首頁能放一篇 repo 介紹。但**不寫死在 Go 程式或 SQL migration**，而是放在獨立 `.md` 檔，便於日後維護。
2. admin/mod（以及作者本人）可以**事後編輯**那篇文，而不是每次重啟才能改。
3. 任意一篇文章都能用 Markdown 表達（含 metadata + 內文 + 留言），可**匯出**（給 mouse-select / OSC 52 / 寫進伺服器檔案）也可**匯入**（貼上 + 後端 CLI）。

對應使用者的話：「也許能夠以某種 seed 文章的方式存著，這樣直接拿 markdown 初始化它（不用寫死在 code 或是 DB migration）」。

User 已選擇全包 (1)~(5)，編輯權限 = 作者 OR mod+，匯出三種出口都做（差別在於用途/權限）。

## 高層設計

```
            ┌──────────────┐
            │ welcome.md   │ go:embed (repo 內 source-of-truth)
            └──────┬───────┘
                   │ on startup, if Welcome 看板空 → seed
                   ▼
┌──────────────┐  edit (E)   ┌──────────────────┐  export(y)  ┌────────────────┐
│ articles     │◀───────────▶│ screen_article_  │────────────▶│ TUI viewer +   │
│ (DB)         │             │ edit/view        │             │ OSC52 + file   │
└──────────────┘             └──────────────────┘             └────────────────┘
       ▲                              ▲
       │ Create()                     │ paste markdown (Ctrl+I)
       │                              │
┌──────────────┐                ┌──────────────────┐
│ sshbbs CLI   │                │ screen_post_     │
│ import cmd   │                │ compose          │
└──────────────┘                └──────────────────┘
        ▲                              ▲
        └──────── markdown.Parse()─────┘
```

統一的 markdown 格式，存放在新的 `internal/markdown/` 純函式套件，`Format`/`Parse` 完全不依賴 DB，便於測試與重用。

## Markdown 格式規範

```markdown
---
title: 歡迎來到 SSH-BBS
board: Welcome
author: admin
created_at: 2026-04-30T12:34:56Z
updated_at: 2026-04-30T13:00:00Z
score: 0
id: 1
---

[任意內文，可含 markdown 或純文字]

<!-- sshbbs:pushes -->
- 推 [alice] 2026-04-30T12:35:01Z  great post!
- 噓 [bob]   2026-04-30T12:35:14Z  disagree
- → [carol]  2026-04-30T12:35:20Z  fyi
```

**規則（手寫迷你 parser，不引入 yaml 套件）：**
- Frontmatter：開頭必須是 `---\n`，結尾為 `\n---\n`。每行 `key: value`（取第一個 `:` 切）。未知 key 進 `Extra map[string]string` 保留 forward-compat。
- 留言區：sentinel = `<!-- sshbbs:pushes -->`（夠獨特避免內文撞名；文件註明這行不要寫進內文）。每行 `- <推/噓/→> [<author>] <RFC3339>  <body>`，雙空格分隔最後 body 欄。Body 可含空格但不可含換行。
- 缺欄位用 zero value。
- 為什麼不用 `gopkg.in/yaml.v3`：~6 個欄位用全套 YAML 是過度工程；hand-rolled parser ~80 LOC + 測試，零新依賴。

## 權限矩陣

| 動作 | guest | user | mod | admin | 備註 |
|------|-------|------|-----|-------|------|
| 看 / scroll | ✔ | ✔ | ✔ | ✔ | 既有 |
| 推/噓/→ | ✘ | ✔ | ✔ | ✔ | 既有 `canPush()` |
| 刪文/刪推 | ✘ | 自己的 | ✔ | ✔ | 既有 `canDeleteArticle/Push` |
| **編輯文章 (E)** | ✘ | 自己的 | ✔ | ✔ | 新增 `canEditArticle()` 鏡像 delete |
| **匯出 TUI (y)** | ✔ | ✔ | ✔ | ✔ | 純 read，含 guest |
| **匯出 OSC52** | ✔ | ✔ | ✔ | ✔ | 純 read |
| **匯出寫檔** | ✘ | ✔ | ✔ | ✔ | 寫 `data/exports/<userid>/`，guest 不寫盤 |
| **匯入貼上 (Ctrl+I in compose)** | ✘ | ✔ | ✔ | ✔ | 走 post-compose，author = 自己，pushes 略 |
| **CLI import** | — | — | — | ✔ | `sshbbs import --file ... --board ...`，admin 認證 |

## 檔案 / 套件變更

### 新建

| 路徑 | 用途 |
|------|------|
| `internal/markdown/markdown.go` | `Format` / `Parse` / `Parsed` / `ParsedPush` |
| `internal/markdown/markdown_test.go` | round-trip + edge case 測試 |
| `internal/seed/articles.go` | `go:embed articles/*.md` + `Articles(ctx, st, adminUserID)` 邏輯 |
| `internal/seed/articles/welcome.md` | Welcome 看板的 repo 介紹（內容由我從 README/CLAUDE.md 擷取草擬，user review 後改） |
| `internal/seed/articles_test.go` | seed 冪等性 + 空看板才寫入 |
| `internal/store/migrations/0005_articles_updated_at.sql` | `ALTER TABLE articles ADD COLUMN updated_at DATETIME` (nullable) |
| `internal/tui/screen_article_edit.go` | 編輯表單（form-style，不綁 h/l） |
| `internal/tui/screen_article_edit_test.go` | 鍵綁 + prefill + Ctrl+S submit + h/l 不被攔截 |
| `internal/tui/screen_article_export.go` | 唯讀檢視畫面，按 `1`/`2`/`3` 切換模式 |
| `internal/tui/screen_article_export_test.go` | 三模式輸出 + round-trip 驗證 |
| `internal/tui/osc52.go` | `func WriteClipboard(s string) tea.Cmd` 包 OSC52 escape |
| `cmd/sshbbs/cmd_import.go` | admin CLI：解析 flag、開 store、`markdown.Parse`、`articles.Create` |

### 修改

| 路徑 | 變更 |
|------|------|
| `internal/store/articles.go` | Article 加 `UpdatedAt sql.NullTime`；`scanArticle`/`articleColumns` 同步；新增 `Update(ctx, articleID, requesterID, requesterRole, newTitle, newBody) error` 鏡像 `Delete` 模式（permission check + writeMu + `updated_at = CURRENT_TIMESTAMP`） |
| `internal/store/articles_test.go` | 新增 4 個 Update 測試（自己改成功、非作者非 mod 拒絕、404、updated_at 有寫入） |
| `internal/tui/messages.go` | 加 `ScreenArticleEdit` / `ScreenArticleExport` / `ScreenArticleImport`(若需獨立) 常數；加 `ArticleUpdatedMsg{ArticleID int64}` |
| `internal/tui/root.go` | `navigate()` switch 加三個 case；`guestWriteBlocked` 加 `ScreenArticleEdit` |
| `internal/tui/screen_article_view.go` | 加 `canEditArticle()` 鏡像 `canDeleteArticle()`；加 `case "E"` 跳編輯、`case "y"` 跳匯出檢視；handle `ArticleUpdatedMsg`（filter `ArticleID` 匹配 → 重新 fetch） |
| `internal/tui/screen_article_view_test.go` | 加 E key（作者/mod 准、其他人拒）、`ArticleUpdatedMsg` 觸發 refetch |
| `internal/tui/screen_post_compose.go` | 加 `Ctrl+I` 匯入：開啟一個 paste-overlay，使用者貼整段 markdown，按 Ctrl+S 跑 `markdown.Parse()`，title/body 自動 prefill；推文段忽略 |
| `internal/tui/screen_post_compose_test.go` | Ctrl+I 模式 + 解析失敗 fallback 到原始字串 |
| `cmd/sshbbs/main.go` | 在 `auth.SeedSystemAccounts` 之後呼叫 `seed.Articles(ctx, st, auth.ReservedUsernameAdmin)`；加 `import` 子命令 dispatch（`os.Args[1] == "import"` → 走 `cmd_import.go`） |
| `internal/chat/broker.go` | 確認 `ArticleUpdatedMsg` 廣播路徑 — 沿用 `SendToAll` 模式（編輯者自己不需重 fetch，所以用 `SendToAll(editorUID, msg)` 排除自己） |
| `Makefile` | 新增 `make import FILE=foo.md BOARD=Welcome` 包 CLI 呼叫 |
| `TODO.md` | 完工後更新（如果有 stretch 推遲再加） |

### `internal/markdown` 套件 API

```go
package markdown

import (
    "time"
    "github.com/daviddwlee84/sshbbs/internal/store"
)

type FormatOpts struct {
    IncludePushes bool
    BoardName     string // 由 caller 傳 — 純套件不查 DB
}

func Format(a *store.Article, pushes []*store.Push, opts FormatOpts) (string, error)

type Parsed struct {
    Title      string
    BoardName  string
    AuthorName string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    ScoreHint  int64
    Body       string
    Pushes     []ParsedPush
    Extra      map[string]string
}

type ParsedPush struct {
    Kind      store.PushKind
    Author    string
    CreatedAt time.Time
    Body      string
}

func Parse(md string) (*Parsed, error)
```

`internal/markdown` 可以 import `internal/store`（取 `PushKind` 等型別），因為 `store` 沒 import `markdown`，無循環。

### `internal/seed` 套件

```go
package seed

import (
    "context"
    "embed"
    "github.com/daviddwlee84/sshbbs/internal/markdown"
    "github.com/daviddwlee84/sshbbs/internal/store"
)

//go:embed articles/*.md
var articlesFS embed.FS

// Articles 把所有 articles/*.md 解析後依 frontmatter 中的 board 名稱寫入。
// 對每個 board：若該 board 已有任何文章（>=1），則整體 skip — 確保
// admin 編輯後的版本不會被下次重啟覆蓋。
// 若 frontmatter 指定的 board 不存在，記 log 後 skip 該檔，不 fatal。
func Articles(ctx context.Context, st *store.Store, adminUserID string) error
```

**冪等策略**：以「目標看板有沒有文」為準（`ListByBoard(ctx, boardID, 1)` 結果非空就 skip）。理由：

- 不用額外 marker table（schema 簡單）
- 符合 user 訴求：admin 編輯後不被重啟覆蓋
- 代價：要重新 seed 必須先把該看板清空 — 罕見場景，可接受

### 匯出檢視畫面 `screen_article_export.go`

按鍵：
- `1` → 只有 metadata + 內文（`IncludePushes=false`）
- `2` → metadata + 內文 + 留言（`IncludePushes=true`）
- `3` → 寫到 `data/exports/<userid>/<articleID>-<unix>.md` 並 toast 路徑（guest 拒）
- `c` → 透過 OSC52 推到剪貼簿並 toast「已複製到剪貼簿（需終端機支援 OSC52）」
- `j/k`/`g/G`/`pgup`/`pgdown` → 卷動
- `esc/h/left/backspace` → 回 `ScreenArticleView`

不畫 `▸` cursor，不加裝飾，方便 mouse-select 抓乾淨的 markdown。

### CLI import: `cmd/sshbbs/cmd_import.go`

```
sshbbs import --file path/to/article.md --board Welcome [--author admin]
```

- 開 store（重用 `store.Open`）
- 讀檔 → `markdown.Parse`
- 找 board id（`Boards().GetByName`）
- 找 author user id（`--author` 預設 `admin`）
- `Articles().Create(ctx, boardID, authorID, authorUserID, parsed.Title, parsed.Body)` — pushes 段忽略（保留 round-trip 設計，但 import 時不重建 author 對應的 user_id 會有 spoofing 風險，留到 stretch）
- 輸出新建的 article id 到 stdout

`main.go` 在 `flag.Parse()` 前先檢查 `len(os.Args) >= 2 && os.Args[1] == "import"`，呼叫 `cmd_import.Main()` 後 `os.Exit(rc)`。

## 主要重用的既有函式

- `store.ArticleRepo.Create / GetByID / ListByBoard / Delete` — `internal/store/articles.go`
- `store.ArticleRepo.Delete` 的 permission check 樣板 — copy 到 `Update`
- `screen_article_view.canDeleteArticle()` — copy 樣板成 `canEditArticle()`
- `screen_post_compose.go` — copy 樣板成 `screen_article_edit.go`（textinput + textarea + Ctrl+S submit + Esc cancel + Tab switch focus）
- `screen_article_view.go` 對 `PushAddedMsg` 的 filter+refetch 樣板 — copy 給 `ArticleUpdatedMsg`
- `chat.Broker.SendToAll(senderUID, msg)` — `internal/chat/broker.go`
- `internal/store/migrate.go` 的 `go:embed migrations/*.sql` 樣板 — copy 給 `internal/seed/articles.go`
- `auth.SeedSystemAccounts` 提供的 admin user lookup — `internal/auth/seed.go`

## 邊界情況與風險

1. **Migration 0005 的 column ordering** — `ALTER TABLE ADD COLUMN` 在 SQLite 是 append。`articleColumns` SELECT list 要明確列出順序而非 `*`，避免 `scanArticle` rune 對位錯誤。
2. **內文恰好包含 `<!-- sshbbs:pushes -->`** — `Parse` 取「frontmatter 結束之後第一個」sentinel，後續同字串視為留言。`Format` 時若內文有此字串就在輸出時於該行末加 `<!-- sshbbs:literal -->` 註記（best-effort），測試覆蓋這個 case。
3. **Concurrent edit by author + mod** — last-writer-wins，沒有 row-level optimistic concurrency。MVP 可接受；若日後要 row-version 再加。
4. **`updated_at` 排序** — `ListByBoard` 維持 `created_at DESC, id DESC` 不變。bookmark / next-prev 不受影響。
5. **OSC52 大小限制** — 部分終端機（xterm 預設）對 OSC52 payload 有 size cap（通常 1KB-100KB 不等）。長文章可能被截。文件註明，並在 `> 32KB` 時 toast 警告。
6. **`data/exports/` 磁碟用量** — 沒做配額，純信任。Plan 註明：日後若有問題可加 per-user quota。
7. **Import CLI 的 author 假冒** — `--author` 是 admin 的權力（CLI 要 root shell），可接受。In-TUI Ctrl+I 一律覆寫成 `m.deps.User`，避免 user 假冒他人。
8. **Welcome 看板的 admin user 必須先存在** — `seed.Articles` 必須在 `auth.SeedSystemAccounts` 之後呼叫，順序在 `main.go` 已正確安排。

## 測試策略

| 層級 | 範圍 | 檔案 |
|------|------|------|
| L1 unit | `markdown.Format`/`Parse` round-trip + 7 個 edge cases | `internal/markdown/markdown_test.go` |
| L1 unit | `seed.Articles` 冪等、空看板才寫、看板不存在 skip | `internal/seed/articles_test.go` |
| L1 unit | `ArticleRepo.Update` 權限、404、updated_at 設值 | `internal/store/articles_test.go` |
| L2 TUI | `screen_article_edit` 鍵綁 + prefill + h/l 不攔截 | `internal/tui/screen_article_edit_test.go` |
| L2 TUI | `screen_article_view` E 鍵權限分支、`ArticleUpdatedMsg` refetch | `internal/tui/screen_article_view_test.go` |
| L2 TUI | `screen_article_export` 三模式 + 與 `markdown.Parse` round-trip | `internal/tui/screen_article_export_test.go` |
| L2 TUI | `screen_post_compose` Ctrl+I 匯入分支 | `internal/tui/screen_post_compose_test.go` |
| Integration | CLI import → DB → 重新匯出回 markdown 比對 | `cmd/sshbbs/cmd_import_test.go` |

全部跑 `make test-race`。

## 端對端驗證

```bash
# 1. 建表 + seed
make db-reset
make hostkey   # 若需
make run &     # 背景啟動

# 2. SSH 連入觀察 Welcome 看板
ssh new@localhost -p 2222         # 註冊一個一般 user
ssh alice@localhost -p 2222       # 進去看 Welcome 應有 1 篇

# 3. 編輯：用 admin 帳號（密碼 admin / 第一次需改）
ssh admin@localhost -p 2222
# Welcome → 進首篇 → 按 E → 改完 Ctrl+S
# 重啟 server，再進 Welcome 看是不是改過的版本（不是 seed 原版） → 驗證冪等

# 4. 匯出：在文章畫面按 y
# - 1 = 不含留言；2 = 含留言；3 = 寫盤；c = OSC52
# 檢查 data/exports/admin/ 出現檔案

# 5. 匯入（in-TUI）：在 post-compose 按 Ctrl+I 貼上整段含 frontmatter 的 markdown
# 確認 title/body 自動 prefill

# 6. 匯入（CLI）：
./sshbbs import --file ./internal/seed/articles/welcome.md --board Test --author admin
# 預期 stdout 有 article id；ssh 進 Test 看板看到該文

# 7. 跑全部測試
make test-race
```

## 出 PR 與 promote-todo

完工後：
- `scripts/promote-todo.sh --title "Welcome seed + markdown round-trip" --summary "..."` 把對應 TODO（如果有）移到 Done
- 把延後事項（push round-trip with author validation、per-user export quota、OSC52 chunking for >32KB）`scripts/add-todo.sh` 加進 TODO

## 不在這次範圍

- **推文（pushes）的 import round-trip**：`markdown.Format` 已輸出 pushes 段，`markdown.Parse` 能讀回 `[]ParsedPush`，但 import 流程不會把這些 pushes 寫回 DB（會有 spoofing / unknown-user 風險）。要做需要：(a) 確認 author username 存在於 users 表 (b) 決定 fallback 行為。記為 TODO。
- **OSC52 大檔分塊**：>32KB 警告但不分塊。記為 TODO。
- **per-user export quota**：先信任，沒有上限。記為 TODO。
- **wiki/版本歷史**：`updated_at` 只記最後一次；不存歷史版本。日後若要做版本對比再加 `article_revisions` 表。
