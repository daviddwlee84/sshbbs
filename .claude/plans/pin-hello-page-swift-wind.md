# Plan — Pin articles as 板規 (multi-pin, mod-toggleable, seed-configurable)

## Context

Boards currently have a one-shot **banner** (per-board ANSI/ASCII art, mod-editable, seeded from `internal/seed/banners/<Name>.txt`) that lives above the article list. The user now wants a richer mechanism: **pin one or more full articles** to the top of a board so they serve as "板規" (board rules) — like PTT's M-mark / 置底 articles. Use cases:

- Default 板規 example: 禁止謾罵 / 禁止廣告 / 違規處理流程
- Mod-edited "important announcement" pinned above chronological posts
- Future: amendments to rules without losing prior wording (法律增修條文 — light convention this round, heavy versioning deferred)

Per the latest clarification: **admin/mod can pin multiple articles per board, can unpin, via keyboard shortcut, and the default set is configurable** (via embedded seed markdown — same idempotency contract as existing `welcome.md`).

## Scope this round

**In:**
- Multi-pin per board (no count cap)
- Pin / unpin toggle by mod+ via keyboard shortcut on the board view
- Pinned articles render at top of board view with `[M]` marker (PTT-style)
- Markdown frontmatter `pinned: true` so seed files configure default pins
- One new seed: `internal/seed/articles/welcome-rules.md` — example template with 禁止謾罵 / 禁止廣告 / 違規處理 — pinned to `Welcome`
- Tests mirroring `articles_test.go` patterns
- Update `Welcome` seed body's keyboard table to mention `M`

**Out (deferred to TODO P3/L `backlog/board-rules-amendment-history.md`):**
- Heavy revision/snapshot table for full audit trail of every edit. This round we lean on the existing markdown round-trip (`internal/markdown/`) and a body convention: pinned 板規 articles should keep an in-body **`## 修訂紀錄`** section listing `vX.Y (YYYY-MM-DD) — 變更摘要`. This is documented in the seed template only; the store doesn't enforce it. If the convention proves load-bearing, graduate it into a proper `article_revisions` table later.
- Per-board moderator gating (`boards.bm` CSV exists but already known unused — SetPinned uses the same global `RoleMod.AtLeast` check that banner-edit uses).
- Reordering pinned items (this round: pinned articles sort by `created_at DESC` like everything else; pin order is implicit).

## Approach — mirror the banner pattern, applied to articles

The banner system is the explicit template the user pointed at. Reuse its three layers verbatim:

| Layer | Banner (existing) | Pin (new) |
|---|---|---|
| Schema | `boards.banner TEXT` (migration 0006) | `articles.pinned_at INTEGER NULL` (migration 0007) |
| Mod write | `BoardRepo.UpdateBanner` w/ `writeMu` + `RoleMod.AtLeast` | `ArticleRepo.SetPinned` w/ `writeMu` + `RoleMod.AtLeast` |
| Seed | `internal/seed/banners.go` reads `banners/*.txt`, idempotent on `banner != ""` | extend `internal/seed/articles.go` to honor `pinned: true` frontmatter; piggybacks existing `board has any article ⇒ skip` idempotency |
| Mod-edit UI | `b` splash / `B` edit on board view | reuse existing article view + `E` edit; **new** `M` toggles pin state on selected article in board list |

## Concrete changes

### 1. Migration `internal/store/migrations/0007_articles_pinned_at.sql`

```sql
-- pinned_at: NULL = unpinned, non-NULL = pinned at this UTC timestamp.
-- Multi-pin per board is just "more than one row with pinned_at IS NOT NULL".
-- We sort pinned-first then by created_at DESC, so explicit pin-order is
-- intentionally not modeled this round — admins reorder by un/re-pinning.
ALTER TABLE articles ADD COLUMN pinned_at TIMESTAMP;

CREATE INDEX idx_articles_board_pinned ON articles(board_id, pinned_at);
```

### 2. Store layer — `internal/store/articles.go`

- Add `PinnedAt sql.NullTime` to `Article` struct (line 10–21) and to `articleColumns` const (line 30) + `scanArticle` (line 33–40). Mirrors how `UpdatedAt` was added in migration 0005.
- Modify `ListByBoard` (line 42) ORDER BY clause:
  ```sql
  ORDER BY (pinned_at IS NULL), created_at DESC, id DESC
  ```
  (Pinned rows sort `0` for `IS NULL`, unpinned sort `1` → pinned first; within each group, newest article first.)
- New method:
  ```go
  // SetPinned pins (pinned=true) or unpins (pinned=false) an article. Permission:
  // requesterRole must be at-or-above mod (mirrors UpdateBanner / not the
  // article-author exception that Update/Delete carry — pinning is a
  // moderation action, not an authorial one).
  func (r *ArticleRepo) SetPinned(ctx context.Context, articleID, requesterID int64,
      requesterRole Role, pinned bool) error
  ```
  Acquires `writeMu`, checks article exists (returns `ErrArticleNotFound`), checks `requesterRole.AtLeast(RoleMod)` (returns `ErrPermissionDenied`), then `UPDATE articles SET pinned_at = CASE WHEN ? THEN CURRENT_TIMESTAMP ELSE NULL END WHERE id = ?`.
- Update `Update` (line 144) to **not** touch `pinned_at` — already correct since it only sets `title, body, updated_at`. Verify in test.

### 3. Markdown frontmatter — `internal/markdown/markdown.go`

- Add `Pinned bool` to `Parsed` struct (line 66).
- In the parser (find the `case "score"` / `case "id"` switch), add `case "pinned"` reading `"true"` / `"1"` → true, anything else → false.
- In `Format`, when serializing, omit `pinned:` when false (keep frontmatter minimal); emit `pinned: true` when true. This makes `internal/tui/screen_article_export.go` round-trip a pinned article losslessly.
- Tests in `internal/markdown/markdown_test.go`: round-trip `pinned: true` → parse → format → assert string contains `pinned: true`.

### 4. Seed — `internal/seed/articles.go`

- After successful `Articles().Create(...)` (line 109), if `parsed.Pinned`, call `Articles().SetPinned(ctx, art.ID, admin.ID, admin.Role, true)`. Two-step write is fine for seed (low frequency, idempotent because the board-empty short-circuit at line 105 still holds).
- Capture the returned `*Article` from Create (currently discarded with `_,`) so we have its ID.
- New seed file: `internal/seed/articles/welcome-rules.md`

  ```markdown
  ---
  title: [板規] Welcome 看板規範 v1.0
  board: Welcome
  author: admin
  pinned: true
  ---

  # 看板規範

  歡迎來到 Welcome 看板。本板為新手綜合區，發文前請閱讀以下規範。

  ## 一、發文守則

  1. **禁止謾罵 / 人身攻擊**：對事不對人。違者刪文 + 警告。
  2. **禁止廣告 / 商業行為**：未經板主同意不得張貼商業連結。
  3. **禁止重複發文**：同一主題請集中討論串，避免洗版。
  4. **禁止散布不實資訊**：引用來源請附連結並標註出處。

  ## 二、推文守則

  - 推文同樣適用發文守則。
  - 不得連續推文洗版（同帳號連續 5 推以上視為洗版）。

  ## 三、違規處理

  | 違規等級 | 處置 |
  |---|---|
  | 輕微 | 板主口頭提醒、刪推文 |
  | 一般 | 刪文 + 站內信警告 |
  | 嚴重 / 累犯 | 提報站長水桶 |

  ## 四、修訂紀錄

  - **v1.0 (2026-05-06)** — 初版上線，定義發文 / 推文 / 違規處理三大區塊。

  > 修訂條文採累積式：未來修訂請新增條目於此區（最新者列於頂端），保留歷史版本以供對照。
  > 若條文有實質變更，請於 commit message / 站務公告同步說明變更原因。
  ```

  This file simultaneously: (a) is a real seed for `Welcome` (b) demonstrates the 修訂紀錄 convention (c) shows mods what a "good" 板規 article looks like.

### 5. TUI — `internal/tui/screen_board_view.go`

- **Render**: in `View()` row loop (line 146–168), prefix the title column with `[M] ` when `a.PinnedAt.Valid`. Width-aware: account for the 4-char prefix in `titleW` calculation. Use `StyleHighlight` (or a new `StyleMark`) so pinned rows are visually distinct even when not under the cursor. Keep the `▸` cursor caret behavior unchanged.
- **Keyboard**: add a new case alongside the existing `"B"` banner-edit case (line 90):
  ```go
  case "M":
      if m.board == nil || !m.canPin() || len(m.articles) == 0 {
          return m, nil
      }
      a := m.articles[m.cursor]
      pinned := !a.PinnedAt.Valid // toggle
      if err := m.deps.Store.Articles().SetPinned(
          context.Background(), a.ID, m.deps.User.ID,
          m.deps.User.Role, pinned,
      ); err == nil {
          if arts, err := m.deps.Store.Articles().ListByBoard(
              context.Background(), m.board.ID, 100,
          ); err == nil {
              m.articles = arts
              // Re-find the cursor — pin re-sorted the slice. Locate by ID.
              for i, x := range m.articles {
                  if x.ID == a.ID {
                      m.cursor = i
                      break
                  }
              }
          }
      }
      return m, nil
  ```
- New helper `canPin()` mirrors `canEditBanner()` (line 184–186): `return m.deps.User != nil && m.deps.User.Role.AtLeast(store.RoleMod)`.
- Append to `appendBannerHelp` (or rename to `appendModHelp`): when `canPin()`, add ` · M pin/unpin` to the help line.
- **Splash flow unchanged** — pinning doesn't interact with the banner splash.

### 6. Live broadcast — `internal/tui/messages.go` + chat broker

Mod presses `M` → other live sessions viewing the same board should see the order update. Reuse the existing `ArticleAddedMsg`-style fan-out: emit a new `ArticlePinChangedMsg{BoardID, ArticleID, Pinned}` after `SetPinned` succeeds; in `boardViewModel.Update`, on receipt re-fetch the list (mirrors the existing `ArticleAddedMsg` handler at line 40–47). One-line broker call: `m.deps.Broker.SendToAll(m.deps.User.ID, ArticlePinChangedMsg{...})`. Filter on receive (msg.BoardID == m.board.ID).

This matches the project's "send to everyone, filter on receive" convention documented in CLAUDE.md (push broadcasts).

### 7. Welcome article — update keyboard table

`internal/seed/articles/welcome.md` line 30 area: append a row mentioning `M  置頂 / 取消置頂（mod 以上）`. Won't re-seed existing DBs (idempotency skips boards with articles), but keeps fresh installs accurate.

## Tests

All under `-race`. New tests in `internal/store/articles_test.go`:

1. `TestArticles_SetPinned_BasicToggle` — pin then unpin, verify `PinnedAt` flips between `Valid=true` and `Valid=false`.
2. `TestArticles_SetPinned_RequiresMod` — guest / user roles → `ErrPermissionDenied`; mod / admin → success. Mirrors `TestUpdateBanner_PermissionDenied` shape (in `boards_test.go`).
3. `TestArticles_SetPinned_NotFound` — articleID `9999` → `ErrArticleNotFound`.
4. `TestArticles_ListByBoard_PinnedFirst` — create 4 articles, pin the 2nd one, verify the pinned article appears at index 0 followed by the rest in `created_at DESC` order.
5. `TestArticles_Update_PreservesPinnedAt` — pin, then `Update(title, body)`, then re-fetch and assert `PinnedAt.Valid` still true.

In `internal/markdown/markdown_test.go`:

6. `TestRoundTrip_Pinned` — Format an Article with `Pinned: true` → Parse → assert `Parsed.Pinned == true`.

In `internal/tui/screen_board_view_test.go` (or whatever the existing file is — same pattern as keybinding-parity tests):

7. Table-driven test asserting that pressing `"M"` on a pinned article calls `SetPinned(false)` and on an unpinned article calls `SetPinned(true)`. Use a fake `Store` mock or hit a real `storetest.New(t)` store.

## Verification

```bash
make hostkey                                  # if first run
make db-reset                                 # ensure fresh seed runs
make run                                      # boots :2222

# In another terminal:
ssh admin@localhost -p 2222                   # admin password from initial seed
# → enter Welcome board → expect to see "[M] [板規] Welcome 看板規範 v1.0"
#   pinned at top with chronological articles below.
# → press M on the pinned article → expect [M] marker to disappear and the
#   article to fall back into chronological order.
# → press M again → expect [M] marker to reappear.

# In a third terminal (live broadcast check):
ssh alice@localhost -p 2222                   # any non-mod
# → enter Welcome board, leave it open. Have admin (term 2) press M on a
#   different article → expect alice's list to reorder live.

# Permission gate sanity check:
ssh alice@localhost -p 2222
# → enter Welcome → press M → expect no change (silent no-op since alice is RoleUser).

# Finally:
make test-race
scripts/todo-kanban.sh --validate-only TODO.md   # if a TODO entry was added for the deferred amendment-history work
```

## Files to create / modify

**Create:**
- `internal/store/migrations/0007_articles_pinned_at.sql`
- `internal/seed/articles/welcome-rules.md`
- (Maybe) `backlog/board-rules-amendment-history.md` — capture the deferred heavy-revision-table design with trade-offs, linked from TODO.md P3/L

**Modify:**
- `internal/store/articles.go` — add `PinnedAt`, `SetPinned`, update `ListByBoard` + `articleColumns` + `scanArticle`
- `internal/store/articles_test.go` — 5 new tests
- `internal/markdown/markdown.go` — add `Parsed.Pinned`, parse + format `pinned:` frontmatter line
- `internal/markdown/markdown_test.go` — round-trip test
- `internal/seed/articles.go` — capture `Article` from `Create`, conditional `SetPinned` post-create
- `internal/tui/screen_board_view.go` — `M` keybinding, `[M]` render prefix, `canPin()`, help-line update, `ArticlePinChangedMsg` handler
- `internal/tui/messages.go` — define `ArticlePinChangedMsg{BoardID, ArticleID, Pinned bool}`
- `internal/tui/screen_board_view_test.go` (or equivalent) — `M` keybinding test
- `internal/seed/articles/welcome.md` — extend keyboard table with `M` row
- `TODO.md` — add Done entry for this feature once shipped + (separately) a P3/L entry pointing at `backlog/board-rules-amendment-history.md` for the deferred heavy versioning

## Why these specific choices

- **`pinned_at TIMESTAMP NULL`, not `pinned BOOLEAN`** — gives us a future ordering signal (when was this pinned?) for free, costs nothing extra now. Avoids touching `articles.filemode` (reserved as PTT-compat bitmask in `docs/ptt_trace_code/03_fileheader_dir.md`; cleaner not to overload it).
- **Uppercase `M` shortcut** — matches the existing convention (`B` for banner-edit is mod-only uppercase; lowercase `b` for read-only banner splash). PTT's M-mark is also literally "M".
- **Mod+ only, not author-also** — pinning is a moderation action; an author shouldn't pin their own post. Diverges intentionally from `Update`/`Delete` which allow author-or-mod.
- **Light amendment convention this round** — the user phrased it exploratorily ("看看是不是有什麼規則可以定一下"). A markdown body section costs nothing and we keep the heavy table on the backlog where it can be designed properly when the need is real.
- **Seed `welcome-rules.md` rather than mutating `welcome.md`** — keeps two distinct seed files: one greeter (existing), one rules. Mods can pin/unpin them independently, edit them independently. Reflects the user's "禁止謾罵 等等" template ask.
