# Admin/Mod: Lock Comments / Arrows-Only on Articles

## Context

PTT-style boards routinely have 板規 / 公告 articles where the BM wants to **stop discussion** entirely, or **prevent 推/噓 voting** while still allowing neutral 箭頭 follow-ups. Today the BBS has no per-article moderation control over comments — anyone non-guest can push/boo/arrow on anything.

This plan adds a single article-level `comments_mode` state, settable by mod+, that gates the `+`/`-`/`=` push flow and is broadcast live to every session viewing the article or its board. It mirrors the existing `pinned_at` moderation pattern (mod-only, not author-admitted) so the surface stays internally consistent.

## Decisions (locked with the user)

- **Schema shape**: single 3-way enum `comments_mode ∈ {open, arrows_only, locked}`. Mutually exclusive; `locked` strictly stronger than `arrows_only`.
- **Permission**: `Role.AtLeast(RoleMod)` only. Author of the article cannot self-lock — mirrors `ArticleRepo.SetPinned` (`internal/store/articles.go:180`), NOT `Update`/`Delete`.
- **Display scope**: badge in both the article list (`screen_board_view.go`) and the article view header (`screen_article_view.go`).
- **Live broadcast**: yes — new `ArticleCommentsModeChangedMsg{BoardID, ArticleID, Mode}` mirroring `ArticlePinChangedMsg`; receivers re-fetch from DB (DB is source of truth).
- **Defense in depth**: gate three layers — UI early (`openPush`), UI late (`updatePushInput` Enter), and store (`PushRepo.Create` inside the existing tx). The store gate is non-negotiable because multiple sessions can race against a mode change.

## State semantics

| Mode          | `+` (push) | `-` (boo) | `=` (arrow) |
|---------------|:---------:|:---------:|:-----------:|
| `open`        | ✅        | ✅        | ✅          |
| `arrows_only` | ❌        | ❌        | ✅          |
| `locked`      | ❌        | ❌        | ❌          |

Existing 推/噓 pushes on an article are **not** retroactively deleted when mode changes — only new pushes are blocked. `recommend_score` is therefore preserved across mode flips. Push **deletion** by author/mod is unaffected by mode (mods can always clean up).

## Files to add

- **`internal/store/migrations/0008_articles_comments_mode.sql`** — `ALTER TABLE articles ADD COLUMN comments_mode TEXT NOT NULL DEFAULT 'open' CHECK (comments_mode IN ('open','arrows_only','locked'))`. No index — column is only read inline with article rows. Follows existing `0007_articles_pinned_at.sql` style.

## Files to modify

### `internal/store/articles.go`

- Add type + constants near the top:
  ```go
  type CommentsMode string
  const (
      CommentsModeOpen        CommentsMode = "open"
      CommentsModeArrowsOnly  CommentsMode = "arrows_only"
      CommentsModeLocked      CommentsMode = "locked"
  )
  ```
- Append `CommentsMode CommentsMode` to `Article` struct (`articles.go:10-22`).
- Add `comments_mode` to `articleColumns` (`articles.go:31-32`).
- Update `scanArticle` (`articles.go:34-41`) to scan into a `string` then assign to `CommentsMode`.
- New method appended after `SetPinned` (after `articles.go:206`):
  ```go
  func (r *ArticleRepo) SetCommentsMode(ctx context.Context,
      articleID, requesterID int64, requesterRole Role, mode CommentsMode) error
  ```
  Implementation copies the `SetPinned` shape exactly: validate mode is one of the three constants, gate on `requesterRole.AtLeast(RoleMod)` returning `ErrPermissionDenied`, take `writeMu`, verify article exists (else `ErrArticleNotFound`), single `UPDATE articles SET comments_mode = ? WHERE id = ?`. `requesterID` is unused for the gate (kept for signature parity with Pin/Update/Delete).

### `internal/store/pushes.go`

- Two new sentinel errors near `ErrPushNotFound` (`pushes.go:13`):
  ```go
  ErrCommentsLocked      = errors.New("comments are locked on this article")
  ErrCommentsArrowsOnly  = errors.New("only arrow comments are allowed on this article")
  ```
- In `PushRepo.Create` (`pushes.go:97-136`), after `tx.BeginTx` and **before** the INSERT:
  ```go
  var modeStr string
  if err := tx.QueryRowContext(ctx,
      `SELECT comments_mode FROM articles WHERE id = ?`, articleID,
  ).Scan(&modeStr); err != nil { /* return err / ErrArticleNotFound */ }
  switch CommentsMode(modeStr) {
  case CommentsModeLocked:
      return nil, ErrCommentsLocked
  case CommentsModeArrowsOnly:
      if kind != PushKindArrow { return nil, ErrCommentsArrowsOnly }
  }
  ```
  Done inside the same tx so the read sees the latest committed mode (`writeMu` is already held). No further changes to score logic.

### `internal/tui/messages.go`

Append after `ArticlePinChangedMsg` (`messages.go:88-97`):
```go
// ArticleCommentsModeChangedMsg is broadcast when a mod changes an article's
// comments_mode. Both article-view (filter by ArticleID) and board-view
// (filter by BoardID) re-fetch from DB so badges and gates stay current.
type ArticleCommentsModeChangedMsg struct {
    BoardID   int64
    ArticleID int64
    Mode      string // CommentsMode value, kept as string for tui-package isolation
}
```

### `internal/tui/screen_article_view.go`

1. **New permission helper** alongside `canPush` (`screen_article_view.go:104-106`):
   ```go
   func (m articleViewModel) canSetCommentsMode() bool {
       return m.deps.User != nil && m.deps.User.Role.AtLeast(store.RoleMod)
   }
   ```
2. **Picker state** on the model (after `pendingDelete` at `:38-39`):
   ```go
   pickingCommentsMode bool
   ```
3. **Update msg handlers** in `Update` switch (`screen_article_view.go:122+`):
   - Add `case ArticleCommentsModeChangedMsg:` — if `m.article != nil && msg.ArticleID == m.article.ID`, refetch article (mirrors `PushAddedMsg` shape at `:134-145`). No body re-render needed (only the header badge changes).
4. **Picker dispatch** in the key switch — add at the top of the `tea.KeyMsg` branch (right after `if m.pendingDelete` at `:167-169`):
   ```go
   if m.pickingCommentsMode { return m.updateCommentsModePicker(msg) }
   ```
5. **New keybinding** appended in the main key switch (after `case "y"` at `:281-289`):
   ```go
   case "M":
       if !m.canSetCommentsMode() || m.article == nil { return m, nil }
       m.pickingCommentsMode = true
       m.err = ""
       return m, nil
   ```
6. **New handler** `updateCommentsModePicker(msg tea.KeyMsg)`:
   - `1` → `store.CommentsModeOpen`
   - `2` → `store.CommentsModeArrowsOnly`
   - `3` → `store.CommentsModeLocked`
   - `esc` / `n` / `N` → cancel
   - On choice: call `m.deps.Store.Articles().SetCommentsMode(ctx, m.article.ID, u.ID, u.Role, mode)`. On success: refetch article locally, broadcast `ArticleCommentsModeChangedMsg{BoardID, ArticleID, Mode: string(mode)}` via `m.deps.Broker.SendToAll(u.ID, ...)`. Always clear `m.pickingCommentsMode`.
7. **Early gate in `openPush`** (`screen_article_view.go:364-371`):
   ```go
   func (m articleViewModel) openPush(k store.PushKind) tea.Model {
       if m.article != nil {
           switch m.article.CommentsMode {
           case store.CommentsModeLocked:
               m.err = "本文已鎖定留言"; return m
           case store.CommentsModeArrowsOnly:
               if k != store.PushKindArrow { m.err = "本文僅開放箭頭留言"; return m }
           }
       }
       // ...existing body
   }
   ```
8. **Late gate in `updatePushInput` Enter handler** (`:379-391`): re-check the same `CommentsMode` switch right before `Pushes().Create(...)`. The `Create` call will also reject server-side, but checking client-side gives a nicer error than the raw sentinel string.
9. **View() badge** in the article header block (`:425-432`): after the `Score:` line, if `a.CommentsMode != CommentsModeOpen`, append a badge line like:
   ```
   留言: [鎖] 已關閉留言
   留言: [箭] 僅開放箭頭
   ```
   Use `StyleHeader` or a new dim+inverse style for visibility. (No emoji — codebase is text-only and uses `[M]` style markers.)
10. **Picker overlay rendering** in `View()`: when `m.pickingCommentsMode`, append at the bottom (where the delete-confirm currently overlays) a one-line prompt: `留言模式: 1 開放  2 僅箭頭  3 鎖文  Esc 取消`.
11. **Help line**: add `M 留言模式` (only when `canSetCommentsMode()`).

### `internal/tui/screen_board_view.go`

1. **Update msg handler** alongside `case ArticlePinChangedMsg` (`screen_board_view.go:48`): add `case ArticleCommentsModeChangedMsg:` filtering on `m.board != nil && msg.BoardID == m.board.ID`, then re-running the existing `Articles().ListByBoard(...)` re-fetch (lift the existing block into a helper or copy verbatim — pin already does this).
2. **List title prefix** (`screen_board_view.go:195-201`): extend the existing `[M] ` prefix logic so badges stack:
   ```go
   prefix := ""
   if a.PinnedAt.Valid              { prefix += "[M]" }
   if a.CommentsMode == store.CommentsModeLocked      { prefix += "[鎖]" }
   if a.CommentsMode == store.CommentsModeArrowsOnly  { prefix += "[箭]" }
   if prefix != "" { title = prefix + " " + a.Title }
   ```
   `Truncate(title, titleW)` already handles overflow (`:207`).

## Reused infrastructure

- `store.Role.AtLeast(store.RoleMod)` — gate (defined `internal/store/role.go`)
- `chat.Broker.SendToAll(senderUID, msg)` — fan-out (already used at `screen_article_view.go:401-407`, `screen_board_view.go:131-134`)
- `Store.writeMu` discipline — `SetCommentsMode` and the augmented `Create` both grab/already-hold it, mirroring `SetPinned`/existing `Create` (see `internal/store/store.go` mutex notes in `CLAUDE.md`)
- `ArticleRepo.SetPinned` template — copy permission gate + writeMu + existence check (`articles.go:172-206`)
- `ArticlePinChangedMsg` template for the new broadcast type
- Existing `[M]` marker rendering pattern in the article list (`screen_board_view.go:195-201`)

## Tests

All under `-race` (CI standard per `CLAUDE.md`).

### `internal/store/articles_test.go`

- `TestArticles_SetCommentsMode_RequiresMod` — `RoleUser` → `ErrPermissionDenied`; `RoleMod` and `RoleAdmin` → success. Even the article author with `RoleUser` is rejected (parity assertion vs `Update`/`Delete`).
- `TestArticles_SetCommentsMode_RoundTrip` — `open` → `arrows_only` → `locked` → `open` all persist and reload via `GetByID`.
- `TestArticles_SetCommentsMode_RejectsInvalidMode` — empty / unknown string returns a validation error before touching the DB.
- `TestArticles_SetCommentsMode_NotFound` — bogus articleID → `ErrArticleNotFound`.
- Extend the existing migration / scan tests if any assert on column count.

### `internal/store/pushes_test.go`

- `TestPushes_Create_RejectsWhenLocked` — set `comments_mode='locked'`, all three kinds rejected with `ErrCommentsLocked`. Assert no row inserted, `recommend_score` unchanged (use `RowsAffected` / re-`GetByID`).
- `TestPushes_Create_RejectsPushAndBooWhenArrowsOnly` — `comments_mode='arrows_only'`: `PushKindPush` and `PushKindBoo` → `ErrCommentsArrowsOnly`; `PushKindArrow` succeeds and `recommend_score` stays at 0.
- `TestPushes_Create_NoRegressionWhenOpen` — sanity: existing tests still pass; the `comments_mode` SELECT inside the tx doesn't break the concurrent-score canary `TestPushes_ConcurrentScoreAtomicity`.

### `internal/tui/screen_article_view_test.go`

- `TestArticleView_M_OpensPickerForMod` — mod presses `M`, model goes into `pickingCommentsMode=true`; non-mod presses `M`, no state change (silent no-op, consistent with `M` on board view).
- `TestArticleView_PickerSelectionsCallStore` — table-driven over `{"1": open, "2": arrows_only, "3": locked}`: after the keystroke, `Articles().GetByID` shows the new mode AND the picker closes.
- `TestArticleView_PickerEscCancels` — picker open → `esc` → picker closed, no DB write.
- `TestArticleView_PressPlusOnLockedShowsError` — set article locked, press `+` → `m.err = "本文已鎖定留言"`, `m.pushing == false` (input never opens).
- `TestArticleView_PressMinusOnArrowsOnlyShowsError` — arrows-only, press `-` → blocked; press `=` → input opens.
- `TestArticleView_HandlesArticleCommentsModeChangedMsg` — send `ArticleCommentsModeChangedMsg{ArticleID: m.article.ID, Mode: "locked"}`, model re-fetches and now blocks `+`. Mismatched `ArticleID` → no-op (parity with `PushAddedMsg` test at `screen_article_view_test.go:632`).
- Mod broadcast end-to-end: after picker action, the `fakeSender` in tests records the `ArticleCommentsModeChangedMsg` with correct `BoardID/ArticleID/Mode`.

### `internal/tui/screen_board_view_test.go`

- `TestBoardView_RendersLockBadgeOnLockedArticle` — seed an article with `comments_mode='locked'`, render board view, `[鎖]` appears in the row.
- `TestBoardView_RendersArrowBadgeOnArrowsOnly` — analogous.
- `TestBoardView_StacksPinAndLockBadges` — both pinned AND locked → `[M][鎖] ...` prefix.
- `TestBoardView_HandlesArticleCommentsModeChangedMsg` — send the msg with matching `BoardID`, `Articles().ListByBoard` is re-issued and the badge updates; mismatched `BoardID` → no refetch.

## Verification (end-to-end)

1. **DB layer**: `make db-reset && make test-race` — all new + existing tests pass; `TestPushes_ConcurrentScoreAtomicity` still green (the new `SELECT comments_mode` inside the tx must not break the 50-goroutine canary).
2. **Migration**: `rm data/bbs.db* && make run` — fresh boot applies through `0008`. `sqlite3 data/bbs.db ".schema articles"` shows the new column with default `'open'`.
3. **Manual two-session smoke**:
   - Terminal A: `ssh admin@localhost -p 2222` (rotate password if first run).
   - Terminal B: `ssh alice@localhost -p 2222` (regular user).
   - Both navigate to `Welcome` board → open the same article.
   - In A: press `M` → see the picker → press `2`. Header in A shows `[箭] 僅開放箭頭`. Within ~1s B's article-view header updates without a manual refresh; B's board list (after going back) shows `[箭]` prefix.
   - In B: press `+` → "本文僅開放箭頭留言" toast, input never opens. Press `=` → input opens, submit a comment, push appears (it's an arrow, score unchanged).
   - In A: press `M` → `3`. Both A and B now show `[鎖]`. B presses `=` → "本文已鎖定留言".
   - In A: press `M` → `1` (back to open). All gates lift in B without reconnect.
   - As alice (regular user) in A: open any article, press `M` → silent no-op (no picker, no error toast).
4. **Defense-in-depth**: open two A-sessions as admin; one keeps the article in open mode (its model is stale because A1 hasn't re-fetched after A2's flip); A1 presses `+` → server returns `ErrCommentsLocked`, the toast surfaces it, the model heals on next view change. Confirms the store-side gate is doing real work.

## Out of scope (explicit non-goals)

- Per-board default mode (e.g., a board where all new articles default to `arrows_only`). Add later only if requested — extend `boards.attr` or add a column then.
- Time-bounded locks (auto-unlock after N hours). Trivial follow-up via a sweep job; not now.
- Retroactively deleting existing 推/噓 when switching to `arrows_only`/`locked`. Existing pushes and `recommend_score` stay frozen — clean and surprises nobody.
- Audit log of who flipped the mode and when. None of the existing moderation actions (pin, banner edit, role promotion) log either; out of scope until a broader audit story.
- Per-board moderator (`boards.bm`) enforcement. The `bm` column remains a denormalized legacy field; gate stays at the global `Role`.
