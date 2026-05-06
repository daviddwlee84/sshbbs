# User Settings + Notification System

## Context

Three user-visible needs land together because they share one new "個人設定" screen:

1. **Self-service password change** — today only forced rotation
   (`MustChangePassword=1` route through `ScreenPasswordChange`) is wired up.
   Users have no way to change their password voluntarily.
2. **User bio** — no profile/about field exists. Adding one is a prerequisite
   for any future "看其他用戶資料" feature and is the lightest possible
   schema change to validate the settings-screen plumbing.
3. **Notification system** — when a user is offline (or even online,
   user-controllable), they currently miss every push, water balloon, mail,
   and article reply. Hooking each event into a per-user webhook lets users
   wire up Apprise (Discord/Telegram/email/etc.) without the BBS taking on
   the apprise:// URL parsing burden.

Plus one in-scope feature requested in the same session:

4. **Article reply (Re:)** — the post-compose screen only creates new
   articles. Mail already has email-style "Re: " + blockquote-quoted reply
   (`screen_mail.go:339-348` `quoteForReply`); articles do not. We extend
   the post-compose screen to accept a parent article so an `R` press in
   article-view jumps straight to a quoted reply draft.

**Architecture decision (confirmed with user):** the BBS will not embed
`unraid/apprise-go` (alpha v0.1.x, undocumented Go API). It will POST a
small JSON payload to one or more user-configured webhook URLs. The
recommended deployment puts `caronc/apprise-api` in the same
`docker-compose.yml` as the BBS, and the user pastes
`http://apprise-api:8000/notify/<config-id>` (or any other webhook —
ntfy.sh, Discord, Slack, etc.) into the settings screen.

## File map (what changes, where)

### Schema — migration 0009

**New file**: `internal/store/migrations/0009_user_settings_and_notify.sql`

```sql
ALTER TABLE users ADD COLUMN bio TEXT NOT NULL DEFAULT '';

CREATE TABLE user_notif_prefs (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    on_push    INTEGER NOT NULL DEFAULT 1,  -- 推/噓
    on_wb      INTEGER NOT NULL DEFAULT 1,  -- 水球
    on_mail    INTEGER NOT NULL DEFAULT 1,  -- 站內信
    on_reply   INTEGER NOT NULL DEFAULT 1,  -- 文章被回 (Re:)
    only_when_offline INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_notif_targets (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label      TEXT NOT NULL DEFAULT '',
    url        TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_notif_targets_user ON user_notif_targets(user_id, enabled);
```

`user_notif_prefs` is upserted lazily on first save; absence ≡ all-default.

### Store layer

**Edit `internal/store/users.go`** — extend `User` struct with `Bio
string`, extend `Create/GetByID/GetByUserID` SELECT/INSERT lists, add
method:

```go
func (r *UserRepo) SetBio(ctx context.Context, id int64, bio string) error
```

(Mirror `SetPassword` shape; take `writeMu`.) Add `auth.ValidateBio`
(≤ 1024 chars, allow newlines) in `internal/auth/auth.go`.

**New file `internal/store/notify.go`** — `NotifyRepo` exposed via
`Store.Notify()`:

```go
type NotifyPrefs struct {
    OnPush, OnWB, OnMail, OnReply, OnlyWhenOffline bool
}
type NotifyTarget struct {
    ID         int64
    UserID     int64
    Label, URL string
    Enabled    bool
    CreatedAt  time.Time
}

func (r *NotifyRepo) GetPrefs(ctx, userID) (NotifyPrefs, error)        // returns defaults if row missing
func (r *NotifyRepo) SetPrefs(ctx, userID, NotifyPrefs) error          // upsert
func (r *NotifyRepo) ListTargets(ctx, userID) ([]NotifyTarget, error)
func (r *NotifyRepo) AddTarget(ctx, userID, label, url string) (int64, error)
func (r *NotifyRepo) UpdateTarget(ctx, id, label, url string, enabled bool) error
func (r *NotifyRepo) DeleteTarget(ctx, id, userID int64) error         // userID for guard
```

All multi-statement methods take `Store.writeMu` per CLAUDE.md invariant.

### Notification dispatcher — new package `internal/notify/`

**New file `internal/notify/dispatcher.go`:**

```go
type Event struct {
    Kind       string  // "push" | "wb" | "mail" | "reply"
    ToUserID   int64
    FromUserID string
    Title      string  // pre-formatted, e.g. "alice 推了 «Hello»"
    Body       string  // pre-formatted, e.g. "→ 推 cool post"
}

type Manager struct {
    store   *store.Store
    broker  *chat.Broker  // for "only when offline" check
    queue   chan Event
    client  *http.Client  // 5s timeout
}

func New(st *store.Store, br *chat.Broker) *Manager
func (m *Manager) Start(ctx context.Context)            // spins N=4 workers; cancel via ctx
func (m *Manager) Dispatch(ev Event)                    // non-blocking; drops on full buffer + logs
```

Worker loop: read prefs+targets for `ToUserID`, filter by `Kind`-matching
flag, optionally skip if `OnlyWhenOffline && broker.IsOnline(ToUserID)`,
POST `{"title": ev.Title, "body": ev.Body}` to each enabled target URL.
Apprise-API's `/notify/<key>` endpoint accepts exactly this JSON shape,
so no payload translation needed for the recommended deployment.

`broker.IsOnline(uid int64) bool` — small new method on `chat.Broker`
(check `len(b.sessions[uid]) > 0` under RLock). Pure-read addition.

### Trigger wiring

| Event  | Trigger site                                                   | Notes                                         |
|--------|----------------------------------------------------------------|-----------------------------------------------|
| push   | `screen_article_view.go:516` after `Broker.SendToAll`           | `ToUserID = m.article.AuthorID`; skip self    |
| wb     | `screen_wb.go:271` (compose), `:486` (thread)                   | self-WB already gated; reuse that branch     |
| mail   | `screen_mail.go:483` after `Broker.Send`                        | self-mail already gated; reuse                |
| reply  | new article-reply submit in `screen_post_compose.submit()`      | when `parentArticleID != 0`, fire reply-to-author |

Each call site formats `Title` + `Body` once and calls
`m.deps.Notify.Dispatch(...)`. Failures are logged, never block UI.

### Server wiring

**Edit `internal/server/server.go`:**
- construct `notify.New(st, broker)` once
- call `notifyMgr.Start(ctx)` from server startup
- pass `notifyMgr` into `tui.Deps` (new field `Notify *notify.Manager`)
- shutdown: `cancel()` on context to drain workers

### TUI screens

**Edit `internal/tui/messages.go`** — add four `Screen*` constants:

```go
ScreenUserSettings     // top-level menu
ScreenBioEdit          // textarea form
ScreenNotifySettings   // toggles + target list
ScreenArticleReply     // optional; see below
```

For article reply, **prefer extending `ScreenPostCompose`** over a new
screen — they're ~95% identical. Add `ParentArticleID int64` to
`NavigateMsg`. In `newPostComposeModel(deps, boardID, parentArticleID)`,
if `parentArticleID != 0`, fetch parent and prefill title (`"Re: " +
strip-existing-Re(parent.Title)`) and body (markdown blockquote of
parent body). No new screen const needed; this also avoids splitting
help-screen entries.

**New file `internal/tui/screen_user_settings.go`** — vim-style menu
(reuses the `mainMenuModel` shape from `screen_main_menu.go`):

```
=== 個人設定 ===
  > 修改密碼  Change password
    修改 Bio  Edit bio
    通知設定  Notification settings
    返回      Back
```

j/k/↑/↓ to move, Enter/l/→ to select, q/Esc/h/← back. **Lifts an
existing constraint:** `ScreenPasswordChange` currently quits the
session on Esc (line 64-66) because it's the must-change gate. When
reached from settings (i.e., `deps.MustChangePassword == false`), Esc
should instead `NavigateMsg{To: ScreenUserSettings}`. Branch on the
flag inside `passwordChangeModel.Update`.

**New file `internal/tui/screen_bio_edit.go`** — single textarea (reuse
the post-compose pattern: textarea + Ctrl+S to save, Esc to back). Form
screen, no h/l binding. Pre-populates from `deps.User.Bio`. On submit
calls `Store.Users().SetBio(...)` and refreshes `*deps.User` in place
(same trick as password change line 136-138).

**New file `internal/tui/screen_notify_settings.go`** — split layout:
- top half: 5 boolean toggles (push / wb / mail / reply / only-when-offline)
  rendered as `[x] 有人推/噓我的文章` lines; space toggles, j/k moves
- bottom half: target list with `a` add, `e` edit, `d` delete, `t` toggle-enabled
- "add" inlines a URL textinput (modeled on the `screen_wb.go` inline
  textinput pattern — it's the closest precedent for "modal input within
  a list screen")
- Ctrl+S saves toggles, target ops save immediately

**Edit `internal/tui/screen_main_menu.go`** — insert "個人設定 Profile"
entry between online list and quit. Numeric shortcut: next free.

**Edit `internal/tui/screen_article_view.go`** — bind `R` (uppercase to
avoid clashing with any lowercase `r` already taken; check the existing
key map in this file before assigning) to:

```go
return m, func() tea.Msg {
    return NavigateMsg{
        To: ScreenPostCompose,
        BoardID: m.article.BoardID,
        ArticleID: m.article.ID, // becomes ParentArticleID for compose
    }
}
```

Reuse the existing `ArticleID` field on `NavigateMsg` for the parent
pointer (no new field needed) — `screen_post_compose` is the only
consumer that would interpret it.

**Edit `internal/tui/help.go`** — add `screenHelp` entries for
`ScreenUserSettings` and `ScreenNotifySettings` (which are list screens,
not form screens — they DO contribute help). `ScreenBioEdit` is a form
screen; deliberately omit per the existing convention (`?` should pass
through to textarea).

**Edit `internal/tui/root.go`** — add 3 navigate cases (user-settings,
bio-edit, notify-settings); the post-compose case extends to read
`ArticleID` as parent.

### Deployment — addresses the user's docker-compose question

Add `docker-compose.example.yml` (or extend the existing one if any) at
repo root:

```yaml
services:
  apprise:
    image: caronc/apprise:latest
    container_name: bbs-apprise
    ports: ["8000:8000"]
    volumes: [./apprise/config:/config]   # holds notify URLs per "config key"
    restart: unless-stopped

  bbs:
    build: .
    ports: ["2222:2222"]
    volumes: [./data:/app/data, ./.ssh:/app/.ssh]
    depends_on: [apprise]
```

User's per-account flow:

1. `docker compose up -d`
2. browse `http://localhost:8000`, save apprise:// URLs under config key
   `mykey` (or use `apprise add`)
3. SSH into BBS → 個人設定 → 通知設定 → Add target →
   `http://apprise:8000/notify/mykey`
4. push test from another account → notification fires through apprise

### Documentation deliverable — `docs/notifications.md`

This is a first-class part of the PR (not optional). Outline:

1. **Overview** — what the notification system does, four event kinds,
   per-user opt-in.
2. **Architecture decision: why webhook, not embedded library** —
   capture the trade-off so future contributors don't reopen it:
   - `unraid/apprise-go` is pure-Go but v0.1.x alpha with undocumented
     Go API; binding the BBS to it locks us to its release cadence.
   - `caronc/apprise` (Python, the canonical project) supports 100+
     services and is battle-tested.
   - Webhook indirection means the BBS speaks one protocol (HTTP POST
     `{title, body}`) and any of {apprise-api, ntfy.sh, Discord webhook,
     Slack webhook, custom} works without BBS code changes.
   - Operational cost: one extra container (`caronc/apprise-api`) for
     users who want the full apprise:// surface.
3. **Recommended deployment** — the docker-compose snippet from above,
   plus a `make notif-up` target that brings up the apprise sidecar.
4. **First-time apprise-api setup** — copy-pasteable steps for adding
   a Discord webhook under config key `mykey`:
   ```bash
   curl -X POST http://localhost:8000/add/mykey \
     -d '{"urls": "discord://webhook_id/webhook_token"}'
   ```
5. **Per-user BBS configuration walk-through** — screenshots-or-ASCII of
   the 通知設定 screen showing toggle + target list.
6. **Payload schema** — exact JSON the BBS sends, so users can plug in
   non-apprise webhooks if they want:
   ```json
   {"title": "alice 推了你的文章 «Hello»", "body": "→ 推 cool post"}
   ```
7. **Troubleshooting** — what "delivered=false" looks like in the BBS
   server log, how to verify the apprise-api endpoint with `curl`, how
   to disable a noisy target without deleting it (the `enabled` toggle).
8. **Privacy / threat-model note** — webhook URLs leave your network;
   recommend self-hosted apprise-api over public webhook services for
   sensitive content. Webhook URLs are stored in plaintext in
   `data/bbs.db`; treat the DB file as a secret.

Cross-link from `README.md` (one line under "Features") and from
`CLAUDE.md` (one line in the Architecture section pointing to
`docs/notifications.md` for the full mapping). The architecture
decision (item 2) is also worth a one-liner in `pitfalls/` if we ever
revisit the embedded-library choice — but only after the doc lands;
no speculative pitfall entry yet.

## Critical files to read before implementing

- `internal/store/users.go` — `SetPassword` for the SetBio template, `Create/GetByID` SELECT lists for the `bio` column addition
- `internal/store/migrate.go` — confirm migration loader handles 0009 with no extra wiring
- `internal/store/store.go` — `writeMu` discipline; how `Notify()` accessor is exposed
- `internal/tui/screen_password_change.go:60-78` — Esc behavior to branch on `MustChangePassword`
- `internal/tui/screen_post_compose.go` — extend `newPostComposeModel` signature for `parentArticleID`; reuse mail's `quoteForReply` (move it to a shared helper, or duplicate — duplicating is fine if simpler)
- `internal/tui/screen_mail.go:339-348` — `quoteForReply` pattern
- `internal/tui/screen_wb.go:148-230` — inline-textinput-within-list pattern for the notify-target add UI
- `internal/chat/broker.go` — add small `IsOnline(uid) bool` method
- `internal/server/server.go:76-150` — wire `notify.Manager` into Deps and shutdown
- `internal/tui/help.go` — `screenHelp` map and `isFormScreen` gate
- `internal/tui/root.go:182-243` — navigate switch
- `internal/tui/screen_main_menu.go` — add menu entry

## Tests

- `internal/store/users_test.go` — extend with bio round-trip
- `internal/store/notify_test.go` (new) — prefs upsert, target CRUD, defaults when row missing
- `internal/notify/dispatcher_test.go` (new) — `httptest.Server` recording POST body, verify `{title, body}` JSON; verify timeout doesn't block; verify "only when offline" honoured via fake `chat.Broker`-shaped sender; verify disabled targets are skipped; cover all four event kinds
- `internal/tui/screen_user_settings_test.go` (new) — j/k/Enter navigation between sub-screens
- `internal/tui/screen_bio_edit_test.go` (new) — Ctrl+S persists, `User.Bio` refreshed in place
- `internal/tui/screen_notify_settings_test.go` (new) — toggle flips bit, add/delete target round-trips through Store
- `internal/tui/screen_password_change_test.go` (extend) — Esc-from-settings goes to `ScreenUserSettings`, not `tea.Quit`, when `MustChangePassword=false`
- `internal/tui/screen_article_view_test.go` (extend) — `R` emits `NavigateMsg{To: ScreenPostCompose, BoardID, ArticleID: parent.ID}`
- `internal/tui/screen_post_compose_test.go` (extend) — `ArticleID != 0` constructor prefills `"Re: " + ...` title and `> `-quoted body
- `internal/chat/broker_test.go` (extend) — `IsOnline` true/false

All run under `make test-race`. The notify dispatcher test is race-bait
in the same spirit as `TestPushes_ConcurrentScoreAtomicity`: fire 50
concurrent `Dispatch` calls and verify no panic / no leaked goroutines
on `Stop`.

## Verification (end-to-end)

1. `make db-reset && make build && make hostkey` (if needed)
2. `docker compose up -d apprise` (or run `caronc/apprise-api` standalone on `:8000`)
3. Configure apprise key `test` with a URL you can observe (e.g., a `webhook.site` URL or a local `nc -l 9999` for raw inspection)
4. `make run`
5. Two SSH sessions: register `alice` and `bob`
6. As `alice`: 個人設定 → 通知設定 → Add `http://localhost:8000/notify/test` → enable all four events
7. As `bob`: post an article on a board, then push on alice's article → expect notification
8. As `bob`: send 水球 / mail to alice → expect notifications
9. As `bob`: open alice's article, press `R`, send reply → expect "reply" notification (and a new article titled `Re: …` with quoted body in the same board)
10. As `alice`: 個人設定 → 修改 Bio → save; reconnect → bio survives
11. As `alice`: 個人設定 → 修改密碼 → set new password; reconnect with new password
12. `go test -race ./...`

## TODO.md follow-ups (out of scope)

- Profile-view screen for *other* users (bio + last-login + post count) — `[M]` effort, separate PR
- Notification template customization (user-editable title/body format strings) — `[M]`, P3
- Per-board mute / per-author mute for push notifications — `[M]`, P3
- Bundle a `caronc/apprise` config-init helper script in `scripts/` — `[S]`, P4

These get the canonical CLAUDE.md treatment: `scripts/add-todo.sh
--priority P? --effort ?` after this PR ships, with `backlog/<slug>.md`
companions for the `[M]` items.
