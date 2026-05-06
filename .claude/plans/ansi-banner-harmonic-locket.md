# Per-Board ANSI/ASCII-Art Banner

## Context

Board screens currently jump straight from a single-line `StyleHeader` ("看板 X · 標題") into the article list — there's no place to express a board's identity, no PTT-style splash, no signature ASCII art. We want each board to carry an editable banner that:

1. Renders **inline** above the article list (PTT-like, always visible while browsing the board).
2. Can be expanded into a **full-screen splash** on demand (when capped inline doesn't show the full piece).
3. Is **seeded once per board** from `internal/seed/banners/<Name>.txt`, with a shared `_default.txt` fallback for boards that don't ship a per-board file.
4. Is **editable by mods+** through a TUI form, mirroring the existing article-edit screen.

UX decisions (locked via AskUserQuestion in this session):

- **Mode**: permanent inline header (capped 8 lines) + summonable full splash via `b`. No auto-splash on entry — that needs per-session "seen" state we don't otherwise need yet.
- **Format**: raw ANSI/ASCII passthrough only. No `glamour` for banners — PTT culture is colored ASCII art (figlet/lolcat output, manual `\x1b[…m` codes), and glamour mangles raw ANSI. Future markdown-banner mode can ship later via a frontmatter switch if desired.

Non-goals this round: per-board mod binding (the `boards.bm` column stays unused; mod+ is the global role check, matching `ArticleRepo.Update`); markdown rendering through glamour; splash-on-entry session tracking; banner versioning / history.

## Architecture summary

| Layer | Change |
|---|---|
| Schema | New column `boards.banner TEXT` via migration `0006_boards_banner.sql` |
| Repo | `BoardRepo.UpdateBanner(ctx, boardID, requesterID, requesterRole, banner)` mirroring `ArticleRepo.Update` shape (`internal/store/articles.go:144-165`) |
| Seed | New `internal/seed/banners.go` + `internal/seed/banners/*.txt` (embed). Idempotency = `banner IS NULL` per board (different from articles' "board has zero rows" rule because banner is one field, not many rows) |
| Width helpers | New `stripANSI` / `bannerVisualWidth` / `bannerTruncate` in `internal/tui/runewidth.go` — leave existing `Width/Truncate/PadRight/PadLeft` untouched (they're load-bearing for the article-list table; banner is the only ANSI-bearing render path) |
| Inline render | `screen_board_view.View()` gains a banner block between current header (line 96) and error/list (line 98); cap at `MaxInlineBannerLines = 8` with "(b 看完整)" tail when over |
| Splash screen | New `screen_board_splash.go` (`ScreenBoardSplash`) — full-render banner, any key returns to `ScreenBoardView` |
| Edit screen | New `screen_board_banner_edit.go` (`ScreenBoardBannerEdit`) — single `textarea.Model`, ctrl+s submits, esc cancels, no h/l binding. Permission gated to `RoleMod`+ at construction (early `loadErr`) |
| Keys on board view | `b` → splash (everyone); `B` → edit (mod+ only, hint hidden otherwise) |
| Wiring | Two new `Screen*` constants in `messages.go`; two new cases in `root.go` navigate switch |

## Files to create

- `internal/store/migrations/0006_boards_banner.sql` — `ALTER TABLE boards ADD COLUMN banner TEXT;`
- `internal/seed/banners.go` — `//go:embed banners/*.txt` + `Banners(ctx, st)` function
- `internal/seed/banners/Welcome.txt`, `Test.txt`, `ChitChat.txt`, `_default.txt` — small ANSI placeholders (figlet-style "Welcome" / "Test" / "ChitChat" + a generic "BBS" fallback). Keep ≤ 6 lines so they fit inline cap with breathing room.
- `internal/tui/screen_board_splash.go`
- `internal/tui/screen_board_banner_edit.go`
- Tests:
  - `internal/store/boards_banner_test.go` — `UpdateBanner` permission/notfound/success
  - `internal/seed/banners_test.go` — seed-on-NULL + skip-on-set + `_default.txt` fallback
  - `internal/tui/runewidth_ansi_test.go` — `stripANSI`, `bannerVisualWidth`, `bannerTruncate` (preserves CSI across cuts, doesn't drop trailing reset)
  - `internal/tui/screen_board_view_test.go` *(if not present)* — banner block appears, `b` emits splash navigate, `B` gated by role
  - `internal/tui/screen_board_banner_edit_test.go` — guest/user denied at construction; mod can save; ctrl+s persists

## Files to modify

- `internal/store/boards.go`:
  - Add `Banner string` field to `Board` struct (line 10-18)
  - Add `, banner` to `boardColumns` (line 24)
  - Add `&b.Banner` to `scanBoard` (line 28)
  - New `UpdateBanner(ctx, boardID, requesterID int64, requesterRole Role, banner string) error` — pattern from `ArticleRepo.Update` (`internal/store/articles.go:144-165`): take `writeMu`, return `ErrPermissionDenied` if not `requesterRole.AtLeast(RoleMod)`, return `ErrBoardNotFound` if missing, single `UPDATE boards SET banner = ? WHERE id = ?`. Reuse existing `ErrPermissionDenied` if exported, else mirror locally.
- `internal/tui/runewidth.go`:
  - Add `stripANSI(s string) string` — regex `\x1b\[[0-9;?]*[a-zA-Z]` (matches CSI escapes; covers SGR colors and the rare cursor codes a banner author might paste)
  - Add `bannerVisualWidth(s string) int` = `runewidth.StringWidth(stripANSI(s))`
  - Add `bannerTruncate(s string, w int) string` — walks `s`, copying CSI runs verbatim (zero width) and stopping body chars when visual width hits `w`; appends a trailing `\x1b[0m` if the input contained any CSI (defensive — author may have forgotten the reset). On overflow, append `…` after the visible width budget.
  - Don't touch the existing four helpers' behavior — they're naive and that's fine for the article table.
- `internal/tui/screen_board_view.go`:
  - Add `func (m boardViewModel) renderBanner(width int) string` returning rendered block (or `""` if no banner). Splits on `\n`, applies `bannerTruncate(line, width-2)` per line, caps at 8 lines, appends `"  " + StyleDim.Render("…(b 看完整)")` line when truncated by line count.
  - Insert call into `View()` after line 96 (`b.WriteString("\n\n")`) and before line 98 (loadErr block). Output a trailing `"\n"` when banner is non-empty so the article header has breathing room.
  - Add to `Update()` KeyMsg switch:
    - `case "b":` → `NavigateMsg{To: ScreenBoardSplash, BoardID: m.board.ID}` if `m.board != nil`
    - `case "B":` → permission check via new `canEditBanner()`; on pass, `NavigateMsg{To: ScreenBoardBannerEdit, BoardID: m.board.ID}`
  - Add `func (m boardViewModel) canEditBanner() bool` — mirrors `articleViewModel.canEditArticle` (`screen_article_view.go:135-143`): require `m.deps.User != nil && m.deps.User.Role.AtLeast(store.RoleMod)`.
  - Update both help-text branches (lines 104, 154-156): always append ` · b banner`; conditionally append ` · B edit` when `canEditBanner()`.
- `internal/tui/messages.go`:
  - Add `ScreenBoardSplash` and `ScreenBoardBannerEdit` to the `const (...)` block (lines 6-22). Append at the end to avoid renumbering of any persisted state.
- `internal/tui/root.go`:
  - Add cases in the navigate switch instantiating `newBoardSplashModel(deps, navMsg.BoardID)` and `newBoardBannerEditModel(deps, navMsg.BoardID)`. Forward last-known window size per existing pattern (per CLAUDE.md "Window-resize forwarding").
- `cmd/sshbbs/main.go`:
  - After existing `seed.Articles(...)` call (line 58), call `seed.Banners(ctx, st)`. Treat error like the articles call (log + continue).

## Idempotent seed flow (mirrors `seed.Articles`)

`internal/seed/banners.go` outline:

```
//go:embed banners/*.txt
var bannersFS embed.FS

func Banners(ctx, st) error {
    boards, _ := st.Boards().List(ctx)         // existing method
    defaultBytes, hasDefault := readEmbed("banners/_default.txt")
    for _, board := range boards {
        if board.Banner != "" { continue }     // admin-edited or already seeded
        bytes, ok := readEmbed("banners/" + board.Name + ".txt")
        if !ok {
            if !hasDefault { continue }
            bytes = defaultBytes
        }
        st.Boards().UpdateBanner(ctx, board.ID, adminID, RoleAdmin, string(bytes))
    }
}
```

The contract intentionally differs from `seed.Articles`: that one keys on "board has any article" (because articles are a row collection); this one keys on "the single banner field is empty". Once a mod edits, the field is non-empty and never gets overwritten.

`UpdateBanner` is called from seed with a synthetic "admin" identity — same trick as `seed.Articles` resolving `adminUserID` via `st.Users().GetByUserID(ctx, auth.ReservedUsernameAdmin)` (`internal/seed/articles.go:47`).

## Reused functions / patterns (don't reinvent)

- `seed.Articles` flow (`internal/seed/articles.go`) — copy the embed + admin-resolve + per-entity loop shape.
- `ArticleRepo.Update` permission/error pattern (`internal/store/articles.go:144-165`) — verbatim shape for `UpdateBanner`.
- `articleEditModel` (`internal/tui/screen_article_edit.go`) — body-only variant for banner-edit. Drop the title field and the focus toggle; keep ctrl+s submit, esc cancel, loadErr early-deny.
- `articleViewModel.canEditArticle` (`screen_article_view.go:135-143`) — template for `canEditBanner`.
- `boardViewModel.isGuest` style (`screen_board_view.go:163-165`) — same pattern for permission predicates.
- Form-screen no-h/l convention — just don't bind them in the Update switch (per `screen_post_compose.go`).
- `StyleHeader` / `StyleDim` / `StyleHelp` / `StyleHighlight` / `StyleError` from `internal/tui/styles.go` — reuse for banner block + splash.

## Verification

End-to-end manual smoke:

```bash
make db-reset
make hostkey            # only if first run
make run                # in one terminal

# in another terminal
ssh admin@localhost -p 2222
# enter Welcome board → banner shows inline above list (≤8 lines)
# press b → full splash → any key returns
# press B → edit form opens, type ANSI text (e.g. "\x1b[31mHELLO\x1b[0m"), ctrl+s saves
# back in board view, new banner is shown

ssh alice@localhost -p 2222   # regular user
# enter Welcome → banner shows, b works
# B does nothing / no hint shown

# idempotency check
# stop server, restart — `make run`
# admin's edit survives (banner column non-NULL → seed skips that board)
```

Automated:

```bash
make test-race             # CI standard, must pass
go test -race ./internal/store/... -run TestBoards_UpdateBanner -v
go test -race ./internal/seed/... -run TestBanners -v
go test -race ./internal/tui/... -run TestStripANSI -v
go test -race ./internal/tui/... -run TestBoardView_BannerKeys -v
```

Width-edge tests to include in `runewidth_ansi_test.go`:

- `stripANSI("\x1b[31mABC\x1b[0m") == "ABC"`
- `bannerVisualWidth("\x1b[31m中文\x1b[0m") == 4`
- `bannerTruncate("\x1b[31mABCDEFGHIJ\x1b[0m", 5)` keeps the leading CSI, cuts at 4 visible chars, appends `…` (visible width 5), and the result still contains `\x1b[0m` so subsequent terminal output isn't tinted red.
- `bannerTruncate("ABC", 10) == "ABC"` (no padding, no escape injection on under-width input).

## Out-of-scope (capture as TODO if user wants)

If the user later asks for any of these, route through `scripts/add-todo.sh` per CLAUDE.md:

- Markdown banner mode via frontmatter (`mode: markdown` runs through `glamour`) — `[M]` effort, P3.
- Auto-splash on first per-session entry — needs `chat.Session` to track `seenBanners map[boardID]bool`. `[S]`, P3.
- Per-board mod binding (use `boards.bm` for who-can-edit instead of global RoleMod) — `[M]`, P3.
- Banner versioning / undo. `[L]`, P4.
