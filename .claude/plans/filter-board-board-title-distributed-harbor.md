# Search & Sort: Boards, Articles, plus Help-Key Polish

## Context

Two complementary discoverability gaps surfaced from real usage:

1. **No way to find content quickly.** The board list grows past what fits on
   one page; the article list within a board paginates a hundred at a time
   without any way to narrow by title. Sorting is locked to "newest-first
   (with pinned floating to top)" — there's no way to surface popular threads.
2. **`?` help feels broken.** The help system at `internal/tui/help.go` is
   actually fully wired (`root.go:81-101` intercepts `?` globally and toggles
   `m.helpVisible`), but the **footer hint lines** on Board List, Board View,
   and Article View don't advertise `? help`, while the Main Menu does. Users
   who try `?` on form screens — where it's intentionally suppressed by
   `helpAvailable()` so the character types literally — also conclude the
   feature is broken. The keybinding works; the surface area lies.

This plan adds vim-idiomatic `/` search on both list screens, an `s` sort
toggle in the board view, and fixes the footer-hint inconsistency in the
same change set.

## Decisions (locked with the user)

- **Search activation key:** `/` on both list screens (vim convention,
  currently unbound, doesn't collide).
- **Boards filter:** **client-side**. Boards are bounded (~3 seeded today,
  expected lifetime <100). No DB churn, no migration.
- **Articles filter:** **server-side**. Extend `ArticleRepo.ListByBoard`
  with an options struct; add `WHERE title LIKE ? COLLATE NOCASE` when a
  query is set. Loading 100 then filtering client-side would silently miss
  matches in older articles; that surprise is worse than the cost of an
  un-indexed LIKE bounded to one board.
- **Sort field:** **`recommend_score DESC`**. The cached
  `articles.recommend_score` already exists (signed sum of +1/-1/0 from
  pushes) — semantically "popular/hot", which matches the user's intent
  for 推文量 排序. No `push_count` column, no migration.
- **Sort key:** `s` cycles `Newest ↔ 推文量(score)`. Pinned articles still
  float to top in both modes (matches existing
  `TestArticles_ListByBoard_PinnedFirst`).
- **Help-key polish:** add `? help · / search` to Board List footer,
  `? help · / search · s sort` to Board View footer, `? help` to Article
  View footer. Add the new keys to `screenHelp` in `help.go`. Form screens
  unchanged (still suppressed by `helpAvailable()`).

## State semantics — search mode

Both list screens share the same four-state machine:

| State          | Trigger                          | Behaviour                                                                                   |
|----------------|----------------------------------|---------------------------------------------------------------------------------------------|
| **idle**       | initial; or `Esc` from filtered  | full list visible, all keymap normal                                                        |
| **searching**  | `/` pressed                      | textinput focused; printable keys (incl. `h`/`j`/`k`/`l`/`s`/`p`/`M`/`b`/`B`) type into it  |
| **confirmed**  | `Enter` from searching           | input dismissed; filter persists; cursor reset to 0; `[搜尋: foo · N]` indicator shown      |
| **cleared**    | `Esc` from searching or confirmed| filter wiped; back to idle                                                                  |

Critically: while **searching**, vim navigation is suspended — same rule
as form screens (CLAUDE.md: form screens skip `h`/`l` so text input
works). Only `Enter`, `Esc`, arrows/PgUp/PgDn (for glancing at filtered
results while typing), `Ctrl+U`, `Ctrl+C` remain as controls.

## Files to add

None. All work fits in existing files; no new migrations.

## Files to modify

### `internal/store/articles.go`

Refactor list path to take an options struct so future sort/filter
extensions don't keep growing positional args.

- Add types near the top (after the `Article` struct, before
  `articleColumns`):
  ```go
  type ArticleSort int
  const (
      SortNewestFirst ArticleSort = iota // existing default
      SortByScoreDesc
  )

  type ListArticlesOpts struct {
      Limit       int          // 0 → no limit; callers pass 100 today
      TitleSearch string       // empty → no filter; LIKE '%q%' COLLATE NOCASE
      Sort        ArticleSort
  }
  ```
- New method `ListByBoardOpts(ctx, boardID, opts)` appended after the
  current `ListByBoard` (`articles.go:69-94`). SQL shape:
  ```sql
  SELECT <articleColumns>
  FROM articles
  WHERE board_id = ?
    [AND title LIKE ? ESCAPE '\' COLLATE NOCASE]   -- only when TitleSearch != ""
  ORDER BY (pinned_at IS NULL),
           CASE WHEN @sortByScore THEN recommend_score END DESC,
           created_at DESC, id DESC
  LIMIT ?;
  ```
  - Escape `%`, `_`, `\` in `TitleSearch` via
    `strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)` then wrap as
    `'%' + escaped + '%'` before binding.
  - Built dynamically: branch on whether `TitleSearch != ""` (adds the
    AND clause + bind) and whether `Sort == SortByScoreDesc` (adds the
    `recommend_score DESC` ordering layer). The `pinned-first` and the
    `created_at DESC, id DESC` tiebreaker stay constant in both modes.
  - `created_at DESC, id DESC` as the tail tiebreaker is essential in
    score mode — many fresh posts share `score=0` and need a stable
    secondary ordering.
- Existing `ListByBoard(ctx, boardID, limit)` becomes a thin shim:
  ```go
  return r.ListByBoardOpts(ctx, boardID, ListArticlesOpts{Limit: limit})
  ```
  All current call sites in `internal/tui/screen_board_view.go` (29, 43,
  57, 73 per exploration) keep compiling unchanged. Existing tests
  (`TestArticles_ListByBoard_NewestFirst`, `..._Limit`, `..._PinnedFirst`)
  remain unmodified — that's the regression check on the shim.

### `internal/tui/screen_board_view.go`

- Add to `boardViewModel` struct (currently lines 13-21):
  ```go
  search       textinput.Model
  searchActive bool             // input focused & accepting keys
  filter       string            // confirmed title filter
  sort         store.ArticleSort // SortNewestFirst (default) or SortByScoreDesc
  ```
- New private helper `reload(anchorID int64)` replaces the four
  ad-hoc `Articles().ListByBoard(...)` call sites (lines 29, 43, 57,
  73):
  ```go
  func (m *boardViewModel) reload(anchorID int64) {
      arts, err := m.deps.Store.Articles().ListByBoardOpts(ctx, m.board.ID,
          store.ListArticlesOpts{Limit: 100, TitleSearch: m.filter, Sort: m.sort})
      if err != nil { m.loadErr = err; return }
      m.articles = arts
      if anchorID > 0 { m.cursor = findCursorByID(arts, anchorID) }
      if m.cursor >= len(arts) { m.cursor = max(0, len(arts)-1) }
  }
  ```
  Reuse the existing `findCursorByID` helper (lines 52-77) — it already
  handles the missing-anchor (cursor=0) case. **Concurrent broadcast
  semantics:** when `ArticleAddedMsg`/`ArticlePinChangedMsg`/
  `ArticleCommentsModeChangedMsg` arrive while `searchActive`, still
  call `reload` silently — the textinput's own state is not disturbed
  because we only mutate `m.articles`/`m.cursor`. This matches existing
  message-driven re-fetch semantics ("DB is source of truth" — CLAUDE.md).
- Update key handling around the existing switch (lines 79-152):
  - `case "/"` (only when not searchActive): set `searchActive = true`,
    `m.search.Focus()`, return `m.search.Cursor.BlinkCmd()`.
  - When `searchActive`: route `enter` to confirm
    (`m.filter = strings.TrimSpace(m.search.Value())`,
    `searchActive = false`, `m.search.Blur()`, `m.cursor = 0`,
    `reload(0)`); route `esc` to cancel (drop in-flight value AND prior
    filter — `m.search.SetValue("")`, `m.filter = ""`, `searchActive =
    false`, `reload(0)`); route every other key (incl. `h`/`l`/`s`/`M`/
    `p`/`b`/`B`/`g`/`G`/letters/punct) through `m.search.Update(msg)`,
    then call `filterPreview()` if you want live preview (optional —
    cheap-and-correct version: only filter on Enter).
  - When idle and `m.filter != ""`: `esc` clears the filter and reloads.
  - `case "s"` (only when not searchActive): cycle
    `m.sort = (m.sort + 1) % 2`, then `reload(currentAnchorID)`.
- View() updates (lines 157-252):
  - When `searchActive`: render `m.search.View()` on its own line above
    the article table with prompt `搜尋 / 標題: `.
  - When `!searchActive && m.filter != ""`: render dim header
    `[搜尋: <q> · <N> 筆 · / 修改 · esc 清除]` above the table.
  - When `m.sort == SortByScoreDesc`: render `[排序: 推文量↓]` next to
    the column header (use 推文量 in the user-facing label per the
    user's wording, even though the underlying field is
    `recommend_score` — leave a one-line code comment near the sort
    branch noting the score-vs-pushcount trade-off).
  - Footer hint (lines 244 / 246 — main + guest variants): append
    `· / search · s sort · ? help`.
- Initialize textinput in `newBoardViewModel`: width budget mirrors the
  post-compose pattern (`screen_post_compose.go:62`):
  `m.search.Width = max(20, m.width - 12)`, also set on `WindowSizeMsg`.

### `internal/tui/screen_board_list.go`

Same pattern as board view, with client-side filter so no store change.

- Add to `boardListModel` (lines 13-20):
  ```go
  search       textinput.Model
  searchActive bool
  filter       string
  filtered     []*store.Board   // recomputed from m.boards whenever filter changes
  ```
- New `recomputeFilter()` method:
  ```go
  func (m *boardListModel) recomputeFilter() {
      if m.filter == "" {
          m.filtered = m.boards
      } else {
          lq := strings.ToLower(m.filter)
          m.filtered = m.filtered[:0]
          for _, b := range m.boards {
              if strings.Contains(strings.ToLower(b.Name), lq) ||
                 strings.Contains(strings.ToLower(b.Title), lq) ||
                 strings.Contains(strings.ToLower(b.Description), lq) {
                  m.filtered = append(m.filtered, b)
              }
          }
      }
      if m.cursor >= len(m.filtered) {
          m.cursor = max(0, len(m.filtered)-1)
      }
  }
  ```
  Note: `strings.ToLower` is a no-op for CJK runes, which is fine —
  CJK matching falls back to direct rune-substring comparison and still
  works. If accented Latin board names ever appear, switch to a folded
  comparison; out of scope today.
- Key handling (lines 29-62) gains the same `/` / `enter` / `esc`
  state-machine branch as board view. All cursor/render logic switches
  from `m.boards` to `m.filtered`.
- View() (lines 65-107):
  - Search input row when `searchActive`.
  - Indicator line when filter is active but not searching.
  - Footer (line 104): append `· / search · ? help`.

### `internal/tui/screen_article_view.go`

Footer-only fix; no functional change.

- Line 634 footer hint: append `· ? help`. The article view already has
  a complete `screenHelp[ScreenArticleView]` entry, so users who hit `?`
  see all keys — only the surface advertisement is missing.

### `internal/tui/help.go`

- In `screenHelp[ScreenBoardList]` (block around lines 43-49), add:
  ```go
  {"/", "search boards (Esc clears, Enter confirms)"},
  {"Esc (filtered)", "clear filter"},
  ```
- In `screenHelp[ScreenBoardView]` (block around lines 51-63), add:
  ```go
  {"/", "search articles by title"},
  {"s", "toggle sort (Newest ↔ 推文量)"},
  {"Esc (filtered)", "clear filter"},
  ```
- `helpGlobals` (lines 173-177) already documents `?` — no change.
- `helpAvailable()` (lines 199-204) — unchanged. `/` is **not** a form
  trigger — board list / board view stay non-form, so `?` continues to
  work even when filter is active. The `searchActive` state intercepts
  `?` itself only because every printable key routes to the textinput
  while focused — which is the correct behaviour (typing `?` into a
  search query should literally insert `?`).

## Tests

Extend existing files; no new test files needed.

### `internal/store/articles_test.go`

- `TestArticles_ListByBoardOpts_TitleSearch` — seed 4 articles with
  titles `["First post", "Second post", "Third entry", "First update"]`.
  Query with `TitleSearch: "first"` → expect 2 hits (case-insensitive),
  newest-first.
- `TestArticles_ListByBoardOpts_TitleSearch_EscapesLikeWildcards` —
  seed titles `["50% off", "5 things"]`, search `"50%"` → exactly the
  first; `"5"` → both.
- `TestArticles_ListByBoardOpts_TitleSearch_BoardIsolation` — confirm
  the title filter doesn't leak across boards (echoes
  `TestArticles_BoardIsolation` at line 556).
- `TestArticles_ListByBoardOpts_SortByScore` — seed three articles,
  push to scores 5, 0, 10 via the existing `Pushes().Create` helper;
  query with `Sort: SortByScoreDesc` → expect order `10, 5, 0`.
- `TestArticles_ListByBoardOpts_SortByScore_PinnedStillFirst` — pin a
  0-score article; sort by score; expect pinned article precedes
  unpinned 10-score article (override semantics).
- `TestArticles_ListByBoardOpts_TitleSearchAndSort_Combined` — both
  active simultaneously, pinned-first preserved.
- `TestArticles_ListByBoard_BackwardCompatible` — explicit assertion
  that the legacy shim returns identical row sets to the previous
  baseline for `Limit=N` (regression guard on the shim).

### `internal/tui/screen_board_list_test.go`

- `TestBoardList_SlashEntersSearchMode` — typing `/`, then `Te`, then
  `enter` → filter applied, cursor at 0.
- `TestBoardList_SearchSuspendsHJKLNav` — table-driven over
  `["h", "j", "k", "l", "Q"]`: while `searchActive`, each key types
  into the input rather than navigating/quitting.
- `TestBoardList_EscFromSearchingClearsBoth` — start search, type some
  text, `esc` → both in-flight value and prior filter cleared.
- `TestBoardList_EscFromConfirmedClearsFilter` — confirm a filter,
  cursor away, `esc` → full list restored, cursor=0.
- `TestBoardList_FilterCursorClamps` — cursor=2, filter narrows to 1
  result → cursor=0.
- `TestBoardList_FilterByDescription` — query matches description
  substring on a seeded board.
- `TestBoardList_FilterCJK` — query `閒` matches `ChitChat`/`閒聊`
  board.

### `internal/tui/screen_board_view_test.go`

- `TestBoardView_SlashEntersSearchMode`
- `TestBoardView_SearchSuspendsActionKeys` — table over
  `["s", "M", "p", "b", "B", "h", "l"]`, all type into input.
- `TestBoardView_FilterByTitle` — seed 5 articles, filter to one.
- `TestBoardView_SortToggleByScore` — seed three, push to known scores,
  press `s`, verify order.
- `TestBoardView_SortPinnedStillFirst` — pinned article precedes
  higher-score unpinned in score mode.
- `TestBoardView_SortPersistsAcrossArticleAddedMsg` — press `s`, then
  inject `ArticleAddedMsg` for this board, expect re-rendered list
  still ordered by score with the new article placed correctly.
- `TestBoardView_FilterAndSortCombined`.
- `TestBoardView_FilterCursorReanchorsByID` — narrow filter, the
  `findCursorByID` path keeps highlight on the same article when it's
  still in the filtered set.

### `internal/tui/help_test.go` (if it exists; otherwise skip)

If the file exists with a parity-check pattern, extend it to assert
`screenHelp[ScreenBoardList]` and `screenHelp[ScreenBoardView]` now
contain entries for `/` and (board view only) `s`. If no parity test
exists, don't create one solely for this change.

## Order of work

1. **Store layer first**: `articles.go` (`ListByBoardOpts` + shim) + new
   tests. Run `go test -race ./internal/store/...` — gates everything
   downstream.
2. **Board list** (simpler, no store dependency): textinput field,
   state machine, `recomputeFilter`, view, footer. Tests.
3. **Board view**: refactor four `ListByBoard` calls to `reload(...)`;
   add filter + sort UI. Tests.
4. **Footer hints + `help.go`**: trivial textual edits across three
   files; no test changes beyond the optional `help_test.go` extension.

## Verification

```bash
make test-race                                      # full suite
go test -race -run 'ListByBoardOpts|BoardList_|BoardView_' \
    ./internal/store/... ./internal/tui/...         # focused

# manual two-user smoke (CLAUDE.md "manual two-user smoke")
make hostkey                                        # if not already done
make run &                                          # boots :2222
ssh alice@localhost -p 2222
# in TUI:
#   board list → press / → type "test" → Enter, see narrowed list
#   open a board → press / → type a known title fragment → Enter
#   press s → list reorders by 推文量
#   press ? → help overlay shows / and s entries
#   press Esc on filtered list → filter clears
#   press / on register form (sshing as `new`) → ? must NOT trigger
#     help overlay (form screen suppression preserved)
```

Specifically watch for:
- Cursor position survives filter narrow/widen without panicking.
- Live `ArticleAddedMsg` while a filter or non-default sort is active
  doesn't blow away the user's selection or force-reset the sort.
- Footer hints render `? help` on the three target screens; nothing
  changes on form screens (compose, register, password change, banner
  edit).

## Risks & trade-offs

- **`recommend_score` ≠ literal push count.** A future product
  decision may want "raw push amount excluding 噓" — that's a
  schema-level change (add `push_count`, maintain in PushRepo).
  Logged as a `// TODO(score-vs-pushcount)` near the score-sort branch.
- **Un-indexed LIKE.** `WHERE title LIKE '%q%'` is a sequential scan
  bounded to one board. At BBS scale (hundreds-to-low-thousands of
  articles per active board) this is fine; for an indexed solution we'd
  need FTS5 (currently not compiled into the pure-Go
  `modernc.org/sqlite` build) or a prefix index. Out of scope.
- **Cursor invalidation.** Both screens must clamp `m.cursor` after
  every filter recompute or reload. Tests guard this.
- **`enter` overload.** In idle state, `enter` opens the cursored item;
  in `searching` state, `enter` confirms the filter. Cleanly gated by
  `searchActive`.
- **Pinned overrides score sort.** A mod-pinned -10 article still floats
  to top in score mode. Intentional — pin is an explicit moderation
  signal stronger than score. Document with a code comment.
- **Backward-compat shim.** `ListByBoard(ctx, boardID, limit)` must
  produce byte-identical output to the legacy implementation when
  called with the same args; the existing
  `TestArticles_ListByBoard_NewestFirst|Limit|PinnedFirst` tests are
  the regression guard and must not be modified.
- **CJK width in textinput.** `bubbles/textinput` handles CJK width
  correctly, but the Width field needs to be set on `WindowSizeMsg`
  (mirrors `screen_post_compose.go:62`). Otherwise long queries wrap
  oddly.
- **Help-key on form screens stays suppressed.** Search mode is *not*
  a form screen at the `helpAvailable()` level, but the `searchActive`
  state intercepts `?` (because all printable keys route to the input)
  — which is correct. Users typing `?` into a search query mean it
  literally.

## Critical files

- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/store/articles.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/store/articles_test.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_board_list.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_board_list_test.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_board_view.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_board_view_test.go`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_article_view.go` (footer text only)
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/help.go`
