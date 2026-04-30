# User Permission System (admin / mod / user / guest)

## Context

The BBS today has one user tier: register → bcrypt login → post/push at will.
The user wants four real tiers:

- **admin** — bootstrapped on first run, default password, **forced to change on first login**, can promote/demote others.
- **mod** — can delete *anyone's* article or push (granted by an admin).
- **user** — registered, can delete only their own articles + own pushes.
- **guest** — read-only, no password required (mirrors the existing `new` sentinel pattern).

This adds the missing primitives — a `role` column, a default admin, a
`must_change_password` flag, ownership-checked delete repo methods, a small
admin user-management screen, and the read-only guest path — without yet
introducing PTT's full `userlevel` bitfield (deferred per
`docs/ptt_trace_code/01_userec.md:48`).

## Design at a glance

- **Single enum column `users.role`** with `CHECK (role IN
  ('guest','user','mod','admin'))`. A new `auth.Role` Go type plus
  `Role.AtLeast(min Role) bool`. Future orthogonal flags (banned, suspended,
  friends-only) get their own `users.flags INTEGER` later — keeping the
  rank column clean.
- **Bootstrap is Go code, not SQL** — bcrypt in a migration is
  non-deterministic and would couple cost-12 to migration history. A
  `Users().SeedSystemAccounts()` mirrors `Boards().SeedDefaults` and runs
  in `cmd/sshbbs/main.go`.
- **Default admin = `admin`/`admin` + `must_change_password=1`**. *But*
  SSH login as `admin` is **refused from non-loopback addresses while
  `must_change_password=1`** — so the source-code default can't be
  remotely cracked on a public deploy. Operators override with
  `-admin-password=<...>` at boot.
- **`guest` is a real DB row** with `role='guest'` and an unmatchable
  bcrypt sentinel (`"!"`). The SSH password callback short-circuits like
  it already does for `new`. `NoteLogin` is skipped for both reserved
  sentinels (no point incrementing `num_logins` on a shared row).
- **Hard-delete only** in this PR. `TODO.md` already has `[P2/S] Soft
  delete for articles`; that work stays separate so this stays scoped.
- **Push delete uses an inline cursor in article view** — `p` / `P`
  cycle a push selection, `D` deletes whatever is targeted (article when
  no push is selected). Single-screen state machine; matches PTT's
  mental model. `j` / `k` stay as body-scroll. Help line updates
  contextually.
- **Read-only enforcement is layered** — (1) `Root.navigate` rejects
  compose-screen targets for guest; (2) screens hide unavailable help
  text; (3) chokepoint check inside the four mutate-paths
  (`ArticleRepo.Create`, `PushRepo.Create`, `MailRepo.Insert`,
  `WBRepo.Insert`).
- **Guest sessions are NOT registered in the broker** — sidesteps the
  `(user.ID)` collision when multiple `ssh guest@...` clients attach.
  Guest doesn't show up in `OnlineList`, doesn't receive water balloons,
  doesn't see live new-article broadcasts. That is what "read-only
  spectator" means in this design.

## Schema — `internal/store/migrations/0004_add_user_roles.sql` (new)

```sql
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'
    CHECK (role IN ('guest','user','mod','admin'));
ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_users_role ON users(role);
```

SQLite supports `ALTER TABLE ADD COLUMN` with NOT NULL+DEFAULT in place — no
table rebuild. Existing rows pick up `role='user'` automatically, which is
the correct default.

`internal/store/users.go` updates:
- Add `Role auth.Role` and `MustChangePassword bool` to the `User` struct.
- Extend `userColumns` const + `scanUser` to include the two new columns.
- Add `UserRepo.SetRole(ctx, id int64, role auth.Role) error`,
  `UserRepo.SetPassword(ctx, id int64, hash string) error` (also clears
  `must_change_password`), `UserRepo.ListAll(ctx, limit, offset int)
  ([]*User, error)`, and `UserRepo.CountByRole(ctx, role auth.Role) (int,
  error)` (used by the last-admin guard).
- All under `r.s.writeMu`, like every other multi-statement write.

## Auth package — `internal/auth/role.go` (new) + `auth.go` extensions

```go
package auth

type Role string

const (
    RoleGuest Role = "guest"
    RoleUser  Role = "user"
    RoleMod   Role = "mod"
    RoleAdmin Role = "admin"
)

func (r Role) AtLeast(min Role) bool // rank table: guest<user<mod<admin
func (r Role) Valid() bool            // membership check
```

In `auth.go`:
- Add `ReservedUsernameGuest = "guest"`, `ReservedUsernameAdmin = "admin"`.
- Add `IsReservedUsername(s string) bool` — case-insensitive match against
  any of the three.
- `Register` rejects all three reserved names (currently rejects only
  `new` at line 49–51); error message `ErrReservedUsername` updated to
  list them.
- Add `MustChangePasswordRemoteBlocked(u *store.User, host string) bool`
  — true when `u.Role == RoleAdmin && u.MustChangePassword && host` is
  not a loopback (`127.0.0.0/8`, `::1`).

`VerifyLogin` short-circuits `NoteLogin` for guest (the row is shared; the
counter is meaningless and would create write contention).

## Bootstrap — `Users().SeedSystemAccounts(ctx, opts)` + `cmd/sshbbs/main.go`

`cmd/sshbbs/main.go` adds:

```go
adminPassword := flag.String("admin-password", "",
    "default admin password on first run; empty = bake-in 'admin' with must_change_password=1")
// ...
if err := st.Users().SeedSystemAccounts(ctx, store.SeedOpts{
    AdminPassword: *adminPassword,
}); err != nil { log.Fatalf("seed users: %v", err) }
```

`SeedSystemAccounts`, idempotently:
1. **admin** — if not exists, hash (flag value or `"admin"`), insert with
   `role='admin'`, `must_change_password=1`. If exists, no-op (don't
   reset password on every boot).
2. **guest** — if not exists, insert with `role='guest'`,
   `password_hash="!"` (deliberately unmatchable bcrypt — short prefix
   does not parse), `must_change_password=0`.

## SSH password callback — `internal/server/auth_middleware.go`

```go
host := ...
ctx.SetValue(ctxKeyRemoteHost, host)

switch ctx.User() {
case auth.ReservedUsernameNew:
    ctx.SetValue(ctxKeyRegister, true); return true
case auth.ReservedUsernameGuest:
    u, err := st.Users().GetByUserID(context.Background(), auth.ReservedUsernameGuest)
    if err != nil || u.Role != auth.RoleGuest { return false } // fail-closed
    ctx.SetValue(ctxKeyUserID, u.ID); return true
}
user, err := auth.VerifyLogin(context.Background(), st, ctx.User(), password, host)
if err != nil { return false }
if auth.MustChangePasswordRemoteBlocked(user, host) {
    log.Warn("admin remote login blocked while must_change_password=1", "host", host)
    return false
}
ctx.SetValue(ctxKeyUserID, user.ID); return true
```

## First-login password change — `internal/tui/screen_password_change.go` (new)

Form-style screen (skips `h`/`l` like other forms). Three fields: current
password, new password, confirm. Submit:
1. `bcrypt.CompareHashAndPassword(user.PasswordHash, current)` — guard
   against shoulder-surfed sessions
2. Validate length against `auth.minPasswordLen/maxPasswordLen`
3. New==confirm
4. `bcrypt.GenerateFromPassword(new, bcryptCost)` →
   `UserRepo.SetPassword` (also clears flag)
5. `NavigateMsg{To: ScreenMainMenu}` and refresh `deps.User`

Routing in `internal/server/server.go` `makeProgramHandler`: after loading
the user, if `user.MustChangePassword`, build `tui.Deps{...,
MustChangePassword: true}`. `Deps` gains `MustChangePassword bool` (mutually
exclusive with `IsRegister`). `NewRoot` chooses sub in priority order:
`IsRegister > MustChangePassword > authenticated main menu`.

**This screen and the gate ship in the same commit** — otherwise an admin
who logs in between commits has no path forward.

## Article delete — `ArticleRepo.Delete` + UI

`internal/store/articles.go`:

```go
var ErrPermissionDenied = errors.New("permission denied")

// Delete hard-deletes the article. Cascades to pushes via FK.
func (r *ArticleRepo) Delete(ctx context.Context, articleID, requesterID int64,
    requesterRole auth.Role) error {
    r.s.writeMu.Lock(); defer r.s.writeMu.Unlock()
    var authorID int64
    if err := r.s.db.QueryRowContext(ctx,
        `SELECT author_id FROM articles WHERE id = ?`, articleID).Scan(&authorID); err != nil { ... }
    if authorID != requesterID && !requesterRole.AtLeast(auth.RoleMod) {
        return ErrPermissionDenied
    }
    _, err := r.s.db.ExecContext(ctx, `DELETE FROM articles WHERE id = ?`, articleID)
    return err
}
```

Note `ErrPermissionDenied` lives next to `ErrUserNotFound` /
`ErrArticleNotFound`; reused by push delete and read-only enforcement.

`internal/tui/screen_article_view.go`: add `pendingDelete bool` (confirm
overlay) and `pushCursor int` (-1 = no push selected). New keys in
`Update`:
- `D` (capital): if `pushCursor < 0`, ask "Delete article? (y/N)";
  else "Delete push #N? (y/N)". Sets `pendingDelete=true`.
- `y` while `pendingDelete`: dispatch the delete. Article →
  `NavigateMsg{To: ScreenBoardView, BoardID}`; push → revert score
  locally, refresh `m.pushes`, clamp `pushCursor` to `len(pushes)-1` (or
  -1 if zero remain).
- `n` / `esc` while `pendingDelete`: cancel.
- `p`: cursor next push (-1 → 0 → … → len-1 → -1).
- `P`: cursor prev push (mirror).

`pushCursor` rendered as a `>` gutter on the cursored line. Help line is
contextual: when `pushCursor>=0` shows `D delete push`, otherwise shows
`D delete article` (only if `m.deps.User` is owner or mod).

## Push delete — `PushRepo.Delete` with score revert

`internal/store/pushes.go`:

```go
func (r *PushRepo) Delete(ctx context.Context, pushID, requesterID int64,
    requesterRole auth.Role) error {
    r.s.writeMu.Lock(); defer r.s.writeMu.Unlock()
    tx, err := r.s.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer tx.Rollback()

    var authorID, articleID int64
    var kind string
    if err := tx.QueryRowContext(ctx,
        `SELECT user_id, article_id, kind FROM pushes WHERE id = ?`,
        pushID).Scan(&authorID, &articleID, &kind); err != nil { return err }
    if authorID != requesterID && !requesterRole.AtLeast(auth.RoleMod) {
        return ErrPermissionDenied
    }

    delta := scoreDelta(store.PushKind(kind)) // push:+1 boo:-1 arrow:0 (extracted helper, reused by Create)
    if _, err := tx.ExecContext(ctx, `DELETE FROM pushes WHERE id = ?`, pushID); err != nil { return err }
    if delta != 0 {
        if _, err := tx.ExecContext(ctx,
            `UPDATE articles SET recommend_score = recommend_score - ? WHERE id = ?`,
            delta, articleID); err != nil { return err }
    }
    return tx.Commit()
}
```

Mirrors `PushRepo.Create`'s mutex+tx discipline (CLAUDE.md "writeMu is
process-level"). Concurrent-score atomicity test mirror added in
`pushes_test.go`.

## Admin user-management — `internal/tui/screen_admin_users.go` (new)

- Add `ScreenAdminUsers` constant in `messages.go`.
- `screen_main_menu.go` builds `items` dynamically; appends `{label: "管理
  Admin", to: ScreenAdminUsers}` only when
  `m.deps.User.Role == auth.RoleAdmin`.
- `Root.navigate` adds the case. Defense in depth: if a non-admin
  somehow reaches `NavigateMsg{To: ScreenAdminUsers}`, toast `"管理員專用"`
  and route back to main menu.
- The screen renders a paginated user list (20/page, `[`/`]` for
  page nav) via `UserRepo.ListAll(ctx, limit, offset)`. j/k cursor;
  per-row keys `g`/`u`/`m`/`a` set role to guest/user/mod/admin.
- **Last-admin guard**: before applying `RoleAdmin → other` for the
  cursored user, call `UserRepo.CountByRole(ctx, RoleAdmin)`; if count
  ≤ 1 and the target is admin, refuse with toast `"無法解除最後一名管理員"`.
- Self-edit allowed except for the last-admin demote (above) — admin
  can re-style themselves to mod/user freely otherwise.

## Read-only enforcement — three layers

1. **Navigation gate** (`internal/tui/root.go` `navigate`): when
   `deps.User.Role == auth.RoleGuest`, intercept
   `ScreenPostCompose | ScreenWBCompose | ScreenMailCompose |
   ScreenAdminUsers` and emit `ErrorMsg{Err: errors.New("guest 為唯讀
   帳號")}` instead of building the sub-screen. Origin screen stays.
2. **Cosmetic** (per-screen help lines): board view hides "P 發文";
   article view hides `+/-/=` and `D` for guest; main menu still
   *shows* "水球 / 信箱" entries (read inbox is fine for guest) but
   compose entries inside those screens are dimmed and disabled.
3. **Chokepoint**: `ArticleRepo.Create`, `PushRepo.Create`,
   `WBRepo.Insert`, `MailRepo.Insert` accept `requesterRole auth.Role`
   (or read it off a `*store.User` parameter the caller already has)
   and reject `RoleGuest` with `ErrPermissionDenied`. Two call sites
   per repo (the matching `screen_*_compose.go`). This is intentionally
   narrow — only the four mutate-paths that exist today.

## Critical files modified

| File | Change |
|---|---|
| `internal/store/migrations/0004_add_user_roles.sql` | new — schema migration |
| `internal/store/users.go` | model + `SetRole`/`SetPassword`/`ListAll`/`CountByRole`/`SeedSystemAccounts` |
| `internal/auth/role.go` | new — `Role` type + helpers |
| `internal/auth/auth.go` | reserved-name extension, `IsReservedUsername`, `MustChangePasswordRemoteBlocked`, `Register` rejection update |
| `internal/server/auth_middleware.go` | guest branch + admin remote block |
| `internal/server/server.go` | `MustChangePassword` routing; skip broker register for guest |
| `internal/tui/root.go` | `Deps.MustChangePassword`, navigate guard for guest |
| `internal/tui/screen_password_change.go` | new screen |
| `internal/tui/screen_main_menu.go` | dynamic items; admin entry only for admins |
| `internal/tui/screen_article_view.go` | push cursor (`p`/`P`), `D` + confirm overlay, contextual help |
| `internal/tui/screen_admin_users.go` | new screen |
| `internal/store/articles.go` | `Delete`, `Create` takes role, `ErrPermissionDenied` |
| `internal/store/pushes.go` | `Delete`, `Create` takes role, `scoreDelta` helper extracted |
| `internal/store/waterballoons.go` | `Insert` takes role |
| `internal/store/mail.go` | `Insert` takes role |
| `internal/tui/messages.go` | `ScreenAdminUsers` + any new `Msg` types |
| `cmd/sshbbs/main.go` | `-admin-password` flag, `SeedSystemAccounts` call |

## Existing utilities to reuse

- `Store.writeMu` (`internal/store/store.go`) — wrap every multi-statement repo write
- `storetest.New(t)` — every TUI / store test fixture
- `auth.bcryptCost = 12`, `bcrypt.GenerateFromPassword/CompareHashAndPassword`
- `runCmd` / `keyOf` test helpers (already in `internal/tui/`)
- `StyleHighlight` / `StyleError` / `StyleHelp` / `StyleDim` for the new screens
- `PadRight` / `Truncate` (`internal/tui/runewidth.go`) for the admin user-list rendering
- `chat.Broker.OnlineList` (unchanged; we just don't register guests)

## Test plan

**Store layer** (`go test -race ./internal/store/...`):
- `TestArticleRepo_Delete_OwnerSucceeds`, `_NonOwnerDenied`, `_ModDeletesAnyone`, `_AdminDeletesAnyone`, `_CascadesPushes`
- `TestPushRepo_Delete_OwnerSucceeds`, `_NonOwnerDenied`, `_ModDeletesAnyone`, `_RevertsScore` (table over `push/boo/arrow`)
- `TestPushRepo_Delete_ConcurrentScoreAtomicity` — 50× concurrent deletes, mirror of the existing Create canary
- `TestUserRepo_SeedSystemAccounts_Idempotent` — call twice, observe one row each
- `TestUserRepo_SetRole_LastAdminRefused`
- `TestUserRepo_ListAll_Pagination`

**Auth layer:**
- `TestRegister_RejectsReservedUsernames` (table over `new`, `guest`, `admin`, mixed case)
- `TestRole_AtLeast` — table-driven across all 16 pairs
- `TestRole_Valid` — accepts the four constants, rejects empty / "root"
- `TestMustChangePasswordRemoteBlocked` — only when `role==admin && must_change && host` is non-loopback

**TUI layer** (`go test -race ./internal/tui/...`):
- `TestArticleView_DeleteArticle_RequiresConfirm` — `D` → `y` flow, `D` → `n` cancels
- `TestArticleView_PushCursor_pPCycles`
- `TestArticleView_DeleteCursoredPush_RevertsScoreInUI`
- `TestArticleView_GuestSeesNoDeleteHint`
- `TestPasswordChange_ClearsFlagOnSuccess`, `_RejectsLengthFloor`, `_RejectsOldPasswordMismatch`, `_RejectsConfirmMismatch`
- `TestAdminUsers_NonAdminBlockedByRoute`, `_PromoteToMod`, `_RefusesLastAdminDemote`
- `TestMainMenu_AdminEntry_OnlyForAdmin`
- `TestNavigate_GuestComposeBlocked` — table over `ScreenPostCompose`, `ScreenWBCompose`, `ScreenMailCompose`, `ScreenAdminUsers`

**Test sweep**: grep `INSERT INTO users` across `internal/store/storetest/`,
`internal/store/*_test.go`, and `internal/tui/*_test.go`. Bare-column
inserts pick up `role='user'` automatically via DEFAULT, so most tests are
unaffected; explicit-column inserts (rare) need `role` added.

## End-to-end manual smoke test

1. `make db-reset && make run` → log shows seed of `admin` and `guest`
2. `ssh admin@localhost -p 2222` (password "admin") → password-change
   screen; cannot reach main menu without successful change
3. From a non-loopback IP attempting `ssh admin@<host>` while
   `must_change_password=1` → connection refused with
   `auth failed ... admin remote login blocked` in server log
4. `ssh guest@localhost -p 2222` (any password / empty) → main menu in
   read-only mode; selecting `P 發文` from board view toasts `guest 為
   唯讀帳號`; `+/-/=` on article view does nothing
5. `ssh new@localhost -p 2222` → register flow as before; attempt to
   register `admin` / `guest` / `new` (any case) → friendly
   `ErrReservedUsername`
6. As `alice` (registered): post an article, then `D` → `y` → article
   gone, list refreshed; on bob's article: `D` → `y` → `permission
   denied` toast
7. As `alice`: `p` to cursor a push, `D` → `y` → push removed,
   `recommend_score` decremented in the header
8. As admin (post-rotation): main-menu `管理 Admin` → cursor alice → `m`
   → "alice 已升為 mod" toast
9. Reconnect as alice → `D` on bob's article → success
10. Two parallel SSH sessions as `alice` and `bob` simultaneously
    deleting different pushes on the same article → final
    `recommend_score` matches the algebraic expectation (covered by the
    automated 50× concurrent test, but spot-check manually to confirm
    UX is sane)

## Out of scope — file as TODO entries when shipping

Run `scripts/add-todo.sh` for each:

- **[P2/S] Audit log of admin actions** — table
  `audit_log(actor_id, target_id, action, at)` so promote/demote/delete-by-mod
  is traceable. Mod tools without audit are temptations.
- **[P2/M] Per-board moderators** — `boards.bm` is already a CSV column.
  Mod is currently global; per-board would require resolving `bm`
  against `users.user_id` at the delete check site. Worth a
  `backlog/per-board-moderators.md` if the user wants the option.
- **[P2/S] Soft-delete articles** — *already* in `TODO.md`. Cross-link
  the moderation work so the future implementer knows mod-deletion is
  a first-class case (not just author self-delete).
- **[P3/M] Self-service account deletion + GDPR shape** — out of scope;
  record the shape if regulatory pressure ever arrives.
- **[P3/S] Banned / suspended flags** — the orthogonal `users.flags
  INTEGER` mentioned at the top. `auth.RoleGuest` covers spectator;
  banning a registered user still needs a flag separate from `role`.

## Commit sequence (10 commits, each independently testable)

1. **Migration `0004_add_user_roles.sql` + `User` struct + repo column
   wiring.** No behavior change; existing tests stay green; new
   `User.Role` defaults to `RoleUser` and `MustChangePassword` to false
   for all existing rows.
2. **`auth.Role` type + reserved-username helpers + `Register` rejection.**
   Pure additions; `TestRegister_RejectsReservedUsernames` lands here.
3. **`Users().SeedSystemAccounts` + `-admin-password` flag in main.**
   First boot now creates `admin` and `guest`. Verify by inspecting
   `users` table after `make run`.
4. **Guest password-less SSH + admin remote-block.** Update
   `auth_middleware.go`. Local test: `ssh guest@localhost -p 2222`
   succeeds; non-loopback `ssh admin@...` is refused while flag is
   set.
5. **Password-change screen + `Deps.MustChangePassword` routing.** Both
   pieces in one commit so admin first-login works end-to-end.
6. **Article delete (repo + UI + confirm).** Hard delete, owner-or-mod
   check, table-driven tests for the four role combinations.
7. **Push cursor + push delete (repo with score revert + `p`/`P` +
   `D`).** Concurrent-score-atomicity test mirrored from the existing
   `Create` canary.
8. **Read-only enforcement (layers 1+2+3).** `Root.navigate` guard,
   help-line cosmetics, `Create`/`Insert` chokepoints take `role`. Two
   call sites per mutate-path.
9. **Admin user-management screen + `SetRole` + `ListAll` +
   `CountByRole` + last-admin guard + dynamic main-menu entry.**
10. **Test sweep + TODO promotion.** Add the keybinding-parity tests
    (`D`, `p`/`P`, `y`/`n`), run `scripts/promote-todo.sh` if any
    P-tagged work consumed an existing entry, file the new follow-up
    TODOs from "Out of scope".
