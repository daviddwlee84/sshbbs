# English Mode (i18n: zh-TW + en)

## Context

The TUI currently hard-codes ~150 user-visible CJK strings across `internal/tui/` (headers, menu items, status flashes, errors, confirmations, help lines, placeholders). A handful of strings also live in `internal/store/pushes.go` (push glyphs, used by markdown export round-trip), `internal/store/boards.go` (seed content), and `internal/tui/notify_helpers.go` + `internal/tui/screen_article_view.go:544` (webhook notification titles).

Product owner direction:
- Add a per-user locale preference (`zh-TW` default, `en` opt-in) toggled from User Settings.
- In `en` mode, render `推`/`噓` as `👍`/`👎` so column widths stay stable (1:1 cell-width parity); other labels can be plain English translations.
- Keep zh-TW as the default and the canonical-feeling locale.

Non-goals (record-only, may surface as TODO.md items later):
- Per-board locale overrides
- Locale beyond zh-TW + en (ja/ko/zh-CN)
- SSH `LANG` env detection
- Locale-aware date formatting

## Hard invariant

**`store.PushKind.Glyph()` (`internal/store/pushes.go:33-43`) stays canonical `推`/`噓`/`→` regardless of locale.** It is the serializer for `markdown.Format` (`internal/markdown/markdown.go:143`) and the parser at `internal/markdown/markdown.go:296-303` only recognises those three CJK glyphs. Localising it would silently break `sshbbs export` → `sshbbs import` round-trip (CLI path: `cmd/sshbbs/cmd_import.go:54`).

Locale-aware glyphs go through a new `internal/i18n/glyphs.go` and are consumed only by TUI render functions.

## Approach

### 1. New `internal/i18n` package (Go map literals, not JSON)

```
internal/i18n/
  i18n.go      // Locale type, Valid, Normalize, T, Tf
  keys.go      // const Key... = "screen.main_menu.title" string identifiers
  zh_tw.go     // var zhTW = map[string]string{...}
  en.go        // var en   = map[string]string{...}
  glyphs.go    // PushGlyph / PushVerb / ScoreExploded / CommentsModeBadge
  i18n_test.go
  glyphs_test.go
```

API:
```go
type Locale string
const (
    LocaleZHTW Locale = "zh-TW"
    LocaleEN   Locale = "en"
    Default           = LocaleZHTW
)

func Valid(s string) bool
func Normalize(s string) Locale            // empty / unrecognised → Default
func T(loc Locale, key string) string      // missing in `en` → falls back to zh-TW; missing in both → «key» so it stands out
func Tf(loc Locale, key string, args ...any) string  // fmt.Sprintf wrapper

// Locale-aware glyphs (TUI only — never feed into markdown.Format)
func PushGlyph(loc Locale, k store.PushKind) string         // 推/噓/→  vs  👍/👎/→
func PushVerb(loc Locale, k store.PushKind) string          // for templated webhook titles
func ScoreExploded(loc Locale) string                       // 爆 vs 💥
func CommentsModeBadge(loc Locale, m store.CommentsMode) string  // [鎖]/[箭] vs [L]/[A]
```

Why Go maps over embedded JSON: compile-time key safety via `keys.go` consts, no loader I/O, two locales fit comfortably in literals, mirrors the leaf-package style used by `internal/markdown`.

Key naming: flat dotted, grouped by screen — `screen.<basename>.<role>`, plus `common.*`, `error.*`, `notify.*`, `help.<screen>.*`. Both locales must declare the same set of keys with the same `%s`/`%d`/`%v` directive count (asserted by `i18n_test.go`).

### 2. Locale storage + plumbing

**Migration `internal/store/migrations/0010_add_user_locale.sql`:**
```sql
ALTER TABLE users ADD COLUMN locale TEXT NOT NULL DEFAULT 'zh-TW';
```
Picked up automatically by `internal/store/migrate.go`'s `go:embed migrations/*.sql`.

**`internal/store/users.go` edits** (mirror the `Bio` precedent — bio was added the same way in migration `0009_user_settings_and_notify.sql`):
- Add `Locale string` to `User` struct (after `Bio`).
- Append `, locale` to `userColumns` const (line 34-35).
- Append `&u.Locale` to `scanUser` Scan list (line 40-44).
- Add repo method `func (r *UserRepo) SetLocale(ctx, id int64, locale string) error` — takes `r.s.writeMu.Lock()` per `Store.writeMu` discipline; mirror `SetBio` shape. Caller validates via `i18n.Valid` first.

**`internal/tui/i18n.go` (new shim):**
```go
func localeOf(d Deps) i18n.Locale {
    if d.User == nil { return i18n.Default }   // register / pre-login
    return i18n.Normalize(d.User.Locale)
}
```
Each model file calls `m.deps` → `localeOf(m.deps)` (or stores it on the model when convenient).

**Refresh after locale change**: mirror `internal/tui/screen_bio_edit.go:79-83`:
```go
if fresh, err := m.deps.Store.Users().GetByID(ctx, m.deps.User.ID); err == nil {
    *m.deps.User = *fresh   // in-place — every screen sharing Deps sees it
}
```

### 3. New `screen_locale_settings.go` (dedicated chooser screen)

Two-row radio-style chooser (`[•] zh-TW (繁體中文) / [ ] English`), Enter/Space to select, `Ctrl+S` to commit, Esc to discard. NOT a textinput form, so:
- Add `ScreenLocaleSettings` to `internal/tui/messages.go` (append to the `Screen` iota — append-only is the safe pattern for the navigate switch).
- Add the navigate case in `internal/tui/root.go` (the single `Root.navigate` switch, per CLAUDE.md "NavigateMsg is the only legitimate way to swap screens").
- Insert as the 3rd entry in `internal/tui/screen_user_settings.go` items list (between "修改 Bio" and "通知設定 Notification settings"); extend the numeric-shortcut switch from `"1"-"4"` to `"1"-"5"`.
- **Do NOT add to `tui.isFormScreen`** in `internal/tui/help.go` — `?` should still pop the help overlay on this screen.
- Add `screenHelp[ScreenLocaleSettings]` entry in `help.go`.

### 4. Push / score / badge glyph swaps (en mode)

| Concept | zh-TW | en | Width parity |
|---|---|---|---|
| Push (+1) | `推` | `👍` | 2 = 2 ✓ |
| Boo (-1) | `噓` | `👎` | 2 = 2 ✓ |
| Arrow (0) | `→` | `→` | 1 = 1 (universal) |
| Score "exploded" (≥100) | `爆` | `💥` | 2 = 2 ✓ |
| Locked badge | `[鎖]` | `[L]` | 4 → 3 (acceptable; prefix, not column-aligned — `screen_board_view.go:324-326` runs through runewidth-aware `Truncate`) |
| Arrows-only badge | `[箭]` | `[A]` | 4 → 3 (same) |
| Pinned badge | `[M]` | `[M]` | 3 (universal) |

**Rewire** (rename existing helpers to take a `Locale` arg, delegate to `i18n.PushGlyph`):
- `internal/tui/screen_article_view.go:665-675` — `renderPushKind(loc, k)` → `i18n.PushGlyph` + existing lipgloss styles.
- `internal/tui/screen_article_view.go:677-690` — `kindLabel(loc, k)` → `i18n.PushVerb`.
- `internal/tui/screen_board_view.go:301-307` — `case score >= 100` returns `i18n.ScoreExploded(loc)`.
- `internal/tui/screen_board_view.go:318-323` — `[鎖]`/`[箭]` prefix → `i18n.CommentsModeBadge(loc, ...)`.
- `internal/tui/screen_article_view.go:573-577` — `[箭] 僅開放箭頭` / `[鎖] 已關閉留言` headers → compose `CommentsModeBadge` + `i18n.T(loc, "screen.article_view.{arrows,locked}_label")`.

### 5. Webhook notification titles → recipient locale

Push/WB/mail webhook events should render in the **recipient's** locale (their phone, their language), not the sender's. Lookup happens at the call site — keeps `internal/notify/dispatcher.go` string-agnostic (no need to import `internal/i18n` from the dispatcher package).

Touched call sites:
- `internal/tui/screen_article_view.go:539-547` — push notification: `Store.Users().GetByID(ctx, m.article.AuthorID)` → `i18n.Tf(recipientLoc, "notify.push.title", senderUserID, i18n.PushVerb(recipientLoc, m.pushKind))`.
- `internal/tui/notify_helpers.go:13-24` (`notifyWB`) — recipient lookup, `i18n.Tf(loc, "notify.wb.title", fromUserID)`.
- `internal/tui/notify_helpers.go:28-44` (`notifyMail`) — recipient lookup, `i18n.Tf(loc, "notify.mail.title", fromUserID, subject)`.

Graceful degradation: if `GetByID` fails (deleted-user race), fall back to `i18n.Default`.

### 6. String migration — phased over 3 PRs

**PR 1 — foundation (zh-only render still):** migration 0010, User struct + repo method, full `internal/i18n` package with all keys + glyph helpers + tests. `en.go` empty (every `T(en, key)` falls back to zh-TW so UI is bit-identical to today). Add `screen_locale_settings.go` + Root.navigate case + user-settings menu entry.

**PR 2 — populate `en.go` and convert hard-coded strings, screen by screen.** Recommended order (least state to most): `screen_main_menu.go` → `screen_user_settings.go` → `screen_locale_settings.go` → `screen_board_list.go` → `screen_board_view.go` → `screen_article_view.go` (excluding glyph rewires) → article_edit/export/post_compose → register/password_change/bio_edit → wb/mail/online/admin_users → notify_settings/board_banner_edit/board_splash → `help.go` keymap. Also localize webhook titles per §5.

**PR 3 — push glyph swap + width tests.** Rewire `renderPushKind`/`kindLabel`/`ScoreExploded`/`CommentsModeBadge` per §4. Add `glyphs_test.go` width-parity assertion. Update `help.go` Article-view help entry that hard-codes `推 / 噓 / →` to use locale glyphs.

This phasing isolates the highest-risk visual change (emoji widths in real terminals) into a small reviewable PR with a dedicated test.

### 7. Files modified / created

**Created:**
- `internal/store/migrations/0010_add_user_locale.sql`
- `internal/i18n/{i18n,keys,zh_tw,en,glyphs}.go`
- `internal/i18n/{i18n,glyphs}_test.go`
- `internal/tui/i18n.go` (the `localeOf` shim)
- `internal/tui/screen_locale_settings.go` + `_test.go`

**Modified:**
- `internal/store/users.go` (struct + columns + scan + `SetLocale`)
- `internal/store/pushes.go` (comment-only — reinforce the canonical-glyph invariant)
- `internal/tui/messages.go` (`ScreenLocaleSettings` const)
- `internal/tui/root.go` (navigate case; localize the existing CJK error toasts at lines 167/181)
- `internal/tui/help.go` (register screen, localize section headings)
- `internal/tui/screen_user_settings.go` (3rd entry + numeric range)
- `internal/tui/notify_helpers.go` + `internal/tui/screen_article_view.go:540` (recipient-locale lookup)
- Every `internal/tui/screen_*.go` containing CJK literals (~20 files; mechanical T() replacement)
- Tests that grep for CJK headers/badges (e.g. `screen_board_view_test.go:339,350-351,383`, `screen_article_view_test.go:889-890`, `help_test.go`) — parameterize by locale or assert against `i18n.T(LocaleZHTW, key)`

**Untouched:**
- `internal/markdown/markdown.go`, `cmd/sshbbs/cmd_import.go` (round-trip layer — see Hard invariant)
- `internal/notify/dispatcher.go` (call sites pre-render strings; dispatcher stays string-agnostic)
- `internal/store/migrate.go` (embed glob auto-picks `0010_*`)
- `internal/store/boards.go` seed titles (content, not UI chrome — admin can rename per deployment)

### 8. Testing

`internal/i18n/i18n_test.go`:
1. **Key completeness** — every `zhTW` key exists in `en` and vice-versa; failure prints the missing key list.
2. **Format-arg parity** — for each key, count `%s`/`%d`/`%v`/`%q` in both locale values; mismatch fails (catches a translator dropping a `%s` which would emit `%!(MISSING)` at runtime).
3. **`Normalize` table-driven** — `("","fr","zh","zh-TW","en")` → `(Default, Default, Default, LocaleZHTW, LocaleEN)`.

`internal/i18n/glyphs_test.go`:
- **Width parity** using `github.com/mattn/go-runewidth` (already a dep via `internal/tui/runewidth.go`): assert `runewidth.StringWidth(PushGlyph(LocaleZHTW, k)) == runewidth.StringWidth(PushGlyph(LocaleEN, k))` for every `PushKind`. Same for `ScoreExploded`. Catches any future "improvement" to a 1-cell ASCII glyph that would silently break article-list column math.

`internal/tui/screen_locale_settings_test.go`:
- Toggle to `en`, `Ctrl+S`, assert `*deps.User` updated in place AND DB row updated via fresh `Users().GetByID`.

`internal/tui/notify_helpers_test.go` (extend or new):
- Insert a recipient User with `Locale: "en"`, send a push event, capture via a recording dispatcher (analogous to `chat.fakeSender` in `internal/chat/broker_test.go`), assert `Title` is in English.

`internal/markdown/markdown_test.go` (extend the round-trip test):
- `Format` a push originating from an `en`-locale user, assert the output literally contains `推` (NOT `👍`); then `Parse` and assert the kind round-trips.

CI: `make test-race` (the project's stated CI standard).

### 9. Verification end-to-end

Automated:
```bash
make test-race
go test -race ./internal/i18n/...
go test -race ./internal/tui/... -run TestLocaleSettings
go test -race ./internal/markdown/... -run TestRoundtripPushGlyphCanonical
```

Manual two-user smoke (after PR 3):
```bash
make hostkey       # if not already done
make db-reset      # fresh DB so 0010 runs cleanly
make run
# new shell:
ssh new@localhost -p 2222   # register "alice"
# main menu in zh-TW (default). Navigate: 個人設定 → 語言 Language.
# Move cursor to English, Enter, Ctrl+S. Toast: "✓ Locale saved".
# Esc back to main menu — header now "Main Menu"; items "Boards", "Online", etc.
# Open a board, open an article: 推 column shows 👍 (green), 噓 shows 👎 (red), → unchanged.
# With recommend_score >= 100 (doctor in DB if needed), board list shows 💥 not 爆.
# Lock comments on article (mod): prefix shows [L], header says "[L] Comments locked".

# new shell:
ssh bob@localhost -p 2222   # register "bob", leave at zh-TW default.
# bob pushes alice's article. alice's webhook receives "[BBS] bob 👍 your post".
# alice pushes bob's article. bob's webhook receives "[BBS] alice 推了你的文章".
```

Markdown round-trip (proves Hard invariant):
```bash
./sshbbs export --article 5 --include-pushes > /tmp/a.md
grep -E '^- (推|噓|→) \[' /tmp/a.md      # MUST find canonical zh glyphs
! grep -E '^- (👍|👎) \[' /tmp/a.md       # MUST NOT find emoji
./sshbbs import --file /tmp/a.md --board Welcome   # round-trip back in
```

Schema:
```bash
sqlite3 data/bbs.db ".schema users" | grep -i locale
# Expect: locale TEXT NOT NULL DEFAULT 'zh-TW'
sqlite3 data/bbs.db "SELECT user_id, locale FROM users;"
# Every existing row should show 'zh-TW' (DEFAULT applied to backfill).
```

## Critical files

- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/store/users.go` — User struct, scan, repo
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/store/pushes.go` — canonical Glyph() invariant
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/markdown/markdown.go` — round-trip parser at line 296-303
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/root.go` — Deps struct, navigate switch
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/messages.go` — Screen iota
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/help.go` — screenHelp registry, isFormScreen list
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_user_settings.go` — sub-menu we extend
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_article_view.go` — renderPushKind, kindLabel, push notification dispatch
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_board_view.go` — score column + comments-mode badge prefix
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/notify_helpers.go` — WB / mail webhook titles
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_bio_edit.go` — reference for the in-place User refresh pattern
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/internal/tui/screen_notify_settings.go` — reference for `prefsDirty` + Ctrl+S commit pattern

## Deferred (TODO.md candidates per CLAUDE.md)

Use `scripts/add-todo.sh` after PR 3 ships:
- `[P3/M] Per-board locale override` — `boards.locale TEXT NULL`, override resolution at render time
- `[P3/S] SSH `LANG` env override (one-shot, non-persistent)` — borrowed-account demos
- `[P3/S] Locale-aware date formatting` — en-mode `Jan 02 15:04` vs current `01/02 15:04`
- `[P?/L] Additional locales (ja / ko / zh-CN)` — needs translator volunteers
