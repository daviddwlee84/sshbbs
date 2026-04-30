# TODO

Long-term backlog for sshbbs. See [CLAUDE.md](CLAUDE.md)
for the maintenance workflow that agents should follow.

> **For agents**: when the user surfaces an idea explicitly **not** being
> implemented this session (signals: "maybe later", "nice to have",
> "工程量太大需要再評估", "先記下來"), add it here with priority + effort tags.
> Do not create new `ROADMAP.md` / `IDEAS.md` / `BACKLOG.md` files —
> `TODO.md` is the single backlog index. Long-form research goes in
> [`backlog/<slug>.md`](backlog/).

<!-- Use the exact section order: P1, P2, P3, P?, Done.
     The bundled scripts/todo-kanban.sh validator only inspects top-level
     `- [ ]` and `- ✅` items inside these sections. Prose paragraphs,
     blockquotes, indented sub-bullets, HTML comments, and `---` rules are
     ignored — feel free to add inline guidance like this without breaking
     machine readability. -->

## P1

Likely next batch — items you'd reach for if you sat down to work today.
Mostly "PTT-feel polish" carried forward from the original plan's SHOULD HAVE bucket.

(P1 batch shipped 2026-04-29 — see Done.)

## P2

Worth doing, no rush.

- [ ] **[S] Soft delete for articles** — add `articles.deleted_at` column + filter in `ArticleRepo.ListByBoard`. Avoid breaking the recno-vs-uuid invariant (see `docs/ptt_trace_code/03_fileheader_dir.md`). When implemented, mod-deletion (now in `ArticleRepo.Delete` from the role work) should populate `deleted_at` instead of `DELETE FROM`, so banned content stays in the audit trail.
- [ ] **[S] Audit log of admin actions** — table `audit_log(actor_id, target_id, action, at)` capturing role promotions/demotions and mod deletes. Mod tools without audit are tempting and we already ship them as of the role-permission batch.
- [ ] **[M] Per-board moderators** — `boards.bm` is a CSV of userids today (unused). Resolve `bm` against `users.user_id` at the article/push delete check site so a regular user with their userid in `bm` can mod that one board. Currently mod is global.
- [ ] **[S] Banned / suspended flags** — orthogonal `users.flags INTEGER` bitmap. `RoleGuest` covers spectator; banning a registered user still needs a flag separate from `role` (e.g. set `flags |= FLAG_BANNED` and refuse SSH at the password callback before VerifyLogin returns).
- [ ] **[S] Per-board BM list in header** — `boards.bm` is already populated; just render in `screen_board_view`'s header line.
- [ ] **[S] Seed-data CLI** — `sshbbs seed --articles 50` for demo content / load testing. Lift the SQL from `scripts/record-demo.sh` into a proper subcommand.
- [ ] **[S] ANSI color art splash screen** — render a small lipgloss-styled banner on the welcome/main-menu screens.
- [ ] **[M] Pager settings** — friends-only / do-not-disturb for water balloons. Bitfield on `users` (currently no attr column) + check in `Broker.Send` before delivering.
- [ ] **[M] Per-board attr / hidden boards** — `boards.attr` bitmap (currently always 0). Define const flags in code, gate `BoardRepo.List` on user level.
- [ ] **[M] Public-key auth alongside password** — `wish.WithPublicKeyAuth` callback that looks up a `user_keys` table. Useful for power users who don't want to type a password every time.
- [ ] **[M] Friends / blocklist tables** — needed before pager-settings can be friends-only and before mailbox can have a blocklist. Tables: `user_friends(user_id, friend_id)`, `user_blocks(user_id, blocked_id)`.
- [ ] **[M] Structured logging via `log/slog`** — replace the package-level `log` calls in `internal/server` with slog, add per-session correlation IDs.
- [ ] **[M] Rate limiting on register / post** — token-bucket per IP for register, per-user for post. The PTT bot wave we'd inevitably attract.
- [ ] **[M] SQLite FTS5 search across articles** — virtual table + trigger to mirror inserts. Useful even for a small archive; can be opt-in via build tag if `modernc.org/sqlite` doesn't bundle FTS5.

## P3

Someday / nice-to-have.

- [ ] **[L] Migrate query layer to `sqlc`** — once the schema stops churning. Buys type-safe queries and removes the by-hand `scanX` helpers in repos.
- [ ] **[L] Web read-only mirror** — read-only HTTP frontend so non-SSH users can browse / link to articles. Out of scope until the SSH side feels feature-complete.
- [ ] **[M] Self-service account deletion + GDPR shape** — out of scope until regulatory pressure arrives. Delete-or-anonymize: pick the path before implementing. Article and push denormalize `author_userid` so we can null user_id while keeping the public string.
- [ ] **[M] Prometheus metrics endpoint** — `/metrics` HTTP listener on a separate port, exposing per-board / per-user counters from the broker + DB. Premature without an actual deployment to monitor.
- [ ] **[S] GitHub Actions CI: test-race + vet + staticcheck on PR** — gates M2 entry per `docs/operations/00_overview.md`. ~40-line workflow under `.github/workflows/`.
- [ ] **[M] Migrate config from CLI flags to env vars (M2 prep)** — additive: env vars override flags. Touches `cmd/sshbbs/main.go:22-26`. See `docs/operations/01_environments.md`.
- [ ] **[M] Switch migration runner to `pressly/goose`** — gated on M2 entry (reversibility need). Keep the `fs.FS` / `go:embed` interface. → [research](backlog/goose-migration-switch.md)

## P?

Needs a spike before committing to a real priority. Tag as `[?/Effort]`.

- [ ] **[?/L] Article import from real PTT `.DIR` dumps** — needs spike on `Ptt-official-app/go-bbs` library quality + Big5↔UTF-8 transcoding fidelity before committing. → [research](backlog/ptt-import-spike.md)
- [ ] **[?/L] External chat broker (Redis pub/sub) for zero-downtime** — decouples `chat.Broker` from process memory so live presence + in-flight messages survive restart. Spike on whether `SO_REUSEPORT` socket-handoff is cheaper. → [research](backlog/external-chat-broker.md)
- [ ] **[?/XL] Postgres migration plan** — only if M4 ever triggers (single-box CPU or write throughput sustained at 70%). Captures the SQLite-WAL → Postgres delta + the two-writer constraint that forces it. → [research](backlog/postgres-migration-plan.md)
- [ ] **[?/M] Push round-trip on markdown import** — `markdown.Format` already emits the `<!-- sshbbs:pushes -->` block and `markdown.Parse` reads it back, but `cmd_import.go` and `screen_post_compose` Ctrl+I both drop pushes silently. Restoring them needs (a) per-push author lookup against `users` and a fallback for unknown authors, (b) a permission story so non-admins can't fabricate pushes signed by other users. Probably admin-CLI-only; spike the security model first.
- [ ] **[?/S] OSC 52 chunking for >32KB exports** — `screen_article_export` toasts a warning when payload exceeds `osc52Limit` (32KB) but still emits the full sequence; xterm and friends silently truncate. Either chunk via the `52;c;<part>;<id>` extension or fall back automatically to the file-write path.
- [ ] **[?/S] Per-user export quota for `data/exports/`** — currently unbounded; one user can fill the disk. Cheapest fix: cap per-user to N files, evict oldest. Implement when disk pressure becomes real.

## Done

Recently shipped. When implementing an active item, in the same commit run:

```
scripts/promote-todo.sh --title "<substring>" --summary "<one-line shipped summary>"
```

This moves the entry here using the dated `Done` syntax and re-validates.

<!-- All 12 P0 (MUST HAVE) items from the original plan, plus the post-MVP
     polish work (h/l navigation, layered test suite, VHS demo recording,
     CLAUDE.md) that shipped in the same week. -->

- ✅ [2026-04-29] [P1/S] Repo skeleton + Makefile — `go.mod`, `.gitignore`, `Makefile` (run/build/test/hostkey/db-reset/test-race/cover/demo), `scripts/gen-hostkey.sh`.
- ✅ [2026-04-29] [P1/M] SQLite store with embedded migrations — `internal/store/`, six tables (users / boards / articles / pushes / water_balloons / schema_migrations), `migrations/0001_init.sql` + `0002_seed_boards.sql`, WAL mode, process-level `writeMu`.
- ✅ [2026-04-29] [P1/M] Auth: register + bcrypt login — `internal/auth` Register/VerifyLogin, reserved `new` username, `auth_test.go` covering validation/duplicates/case-insensitive lookup.
- ✅ [2026-04-29] [P1/M] Storage repos — UserRepo / BoardRepo / ArticleRepo / PushRepo / WaterBalloonRepo with the methods every P0 screen needed; full test coverage in `internal/store/*_test.go`.
- ✅ [2026-04-29] [P1/M] wish SSH server bootstrap — `internal/server/server.go` using `bubbletea.MiddlewareWithProgramHandler` so the broker can hold `*tea.Program` refs; password-auth middleware stashes `user_id` in `ssh.Context`; SSH user `new` routes to register screen.
- ✅ [2026-04-29] [P1/M] In-memory chat broker — `internal/chat/broker.go` with Register/Unregister/Send/SendToAll/OnlineList/LookupByUserID, `Sender` interface for testability, supports same-user multi-session ("雙開").
- ✅ [2026-04-29] [P1/L] TUI screens (state machine) — `internal/tui/`: root + welcome/register/main_menu/board_list/board_view/article_view/post_compose/wb (inbox+compose), all driven by `NavigateMsg` through `Root.navigate`.
- ✅ [2026-04-29] [P1/S] CJK-safe runewidth helpers — `internal/tui/runewidth.go` PadRight/PadLeft/Truncate/Width used in every list rendering; UTF-8 throughout, no Big5.
- ✅ [2026-04-29] [P1/M] 推/噓/→ with live broadcast — inline push input in `screen_article_view`, `PushRepo.Create` runs INSERT + score-update atomically under `writeMu`, `broker.SendToAll` notifies other viewers who filter by `ArticleID` and re-fetch from DB for canonical state.
- ✅ [2026-04-29] [P1/M] Water balloon round-trip — `WBRepo.Insert/ListUnreadFor/MarkRead`, `screen_wb` inbox + compose, live delivery via `broker.Send`, offline persistence + on-reconnect replay (drained in `server.go` after broker registration), Ctrl+U global hotkey.
- ✅ [2026-04-29] [P1/S] Graceful shutdown — `cmd/sshbbs/main.go` with `signal.NotifyContext`, `srv.Shutdown` + broker drain (sends `tea.Quit` to each session), 3s drain timeout, deferred DB close.
- ✅ [2026-04-29] [P1/M] docs/ptt_trace_code/ notes + README quickstart — six trace files (overview / userec / boardheader / fileheader_dir / push_comment / water_balloon / session_userinfo) mapping pttbbs concepts to our schema.
- ✅ [2026-04-29] [P1/S] Vim-style h/l navigation across non-form screens — `h`/`left` = back and `l`/`right` = enter on every list/menu screen; form screens (register/post compose/wb compose) intentionally don't bind h/l so the keys remain available for text editing.
- ✅ [2026-04-29] [P1/L] Layered test suite (46 functions) — store repos + chat broker (concurrent-stress under `-race`) + TUI Update() tests; shared `internal/store/storetest` helper; `chat.Sender` interface refactor for mockability; strategy in `docs/testing.md`.
- ✅ [2026-04-29] [P1/M] VHS demo recording — `scripts/record-demo.sh` + `scripts/demo.tape` produce `docs/demo.gif` / `.webm` embedded in README; fixed lipgloss color profile (`SetColorProfile(termenv.TrueColor)` in `tui/styles.go init`) so background colors render when server stdout is redirected.
- ✅ [2026-04-29] [P1/S] CLAUDE.md authored — non-obvious architecture notes (SSH-user-as-login, ProgramHandler choice, Sender interface, broadcast-then-filter, writeMu discipline, embed scoping for migrations).
- ✅ [2026-04-29] [P1/S] PTT-style extra hotkeys — `[`/`]` cursor aliases on board list / board view / online list / mail inbox; `[`/`]` sibling navigation on article view via new `ArticleRepo.NeighboursOf`; `Q` quit-to-menu on every list screen. `r` deliberately not bound on board view (PTT reserves it for article view's reply, deferred). Table-driven tests extended for every screen.
- ✅ [2026-04-29] [P1/S] Article pagination `g`/`G` — top/bottom jumps in `screen_article_view`. `PgUp`/`PgDn` already worked via `b/pgup` and space/`pgdown` aliases. Helpers `bodyLineCount`/`viewportLines` keep `G` in sync with the View's render maths. Scroll-key table test covers all three cases (short body, long body, edge).
- ✅ [2026-04-29] [P1/S] Online-user list screen — new `screen_online.go` reading `chat.Broker.OnlineList()` (sorted by user_id for stable order); main-menu slot 3 ("線上使用者 Online"), Quit bumped to slot 4. Enter/`l` opens water-balloon compose pre-filled with the cursored user. Test fixture registers fake sessions on a real broker.
- ✅ [2026-04-29] [P1/M] Live broadcast of new articles — `ArticleAddedMsg{BoardID, ArticleID, ...}` mirrors `PushAddedMsg`. `screen_post_compose.submit` calls `broker.SendToAll(authorUID, ...)`; `screen_board_view` filters by `BoardID == m.board.ID` and re-fetches the article list from DB so ordering is canonical. Tests assert recording-broker receipt for non-author, exclusion of author, and DB persistence; receiver-side tests cover both same-board and cross-board cases.
- ✅ [2026-04-29] [P1/M] Mailbox 站內信 — persistent threaded private messages distinct from water balloons. New migration `0003_add_mail.sql` with thread_id (back-filled to self for roots) + parent_id; `MailRepo` (Insert/ListInboxFor/ListThread/MarkRead/MarkAllReadFor/CountUnreadFor/GetByID) under `writeMu`. Three TUI screens (`screen_mail_inbox` / `_thread` / `_compose`) wired through `NavigateMsg{MailID, MailThreadID}`. Main menu slot 4 ("信箱 Mail"). 50× concurrent-insert race test guards the writeMu discipline.
- ✅ [2026-04-30] [P1/L] Welcome 種子文 + 文章 markdown round-trip + 編輯/匯出/匯入 — `internal/markdown/` 純函式套件（hand-rolled YAML-ish frontmatter parser，零新依賴）；`internal/seed/` 把 `articles/welcome.md` 用 `go:embed` 包進 binary，第一次發現 Welcome 看板空就 seed，admin 編輯後重啟不會被覆蓋（看板有任何文就跳過）。新 migration `0005_articles_updated_at.sql` + `ArticleRepo.Update` 鏡像 Delete 的「作者 OR mod+」權限；新 `ScreenArticleEdit` 表單（不綁 h/l）+ `ScreenArticleExport` 唯讀畫面（`1` 純內文 / `2` 含留言 / `3` 寫到 `data/exports/<userid>/` / `c` OSC52 推剪貼簿）。`screen_article_view` 加 `E`（編輯，作者或 mod+）與 `y`（匯出）鍵，handle `ArticleUpdatedMsg` 用 `PushAddedMsg` 樣板 filter+refetch。`screen_post_compose` 加 `Ctrl+I` 從貼上的 markdown 抽 frontmatter 自動填 title/body。新 CLI `sshbbs import --file FILE --board NAME` 走同一個 `markdown.Parse`。煙測通過：seed → 改 → 重啟 → 改的版本保留。
- ✅ [2026-04-30] [P1/L] User permission system — admin / mod / user / guest. New migration `0004_add_user_roles.sql` (role enum + must_change_password). `store.Role` type with rank-ordered `AtLeast`. Reserved usernames extended to `admin` and `guest`; `auth.SeedSystemAccounts` bootstraps both via `-admin-password` flag, default `admin`/`admin` with first-login change forced. `MustChangePasswordRemoteBlocked` refuses bootstrapped admin from non-loopback addresses. New `screen_password_change` (form-style) and `screen_admin_users` (paginated list with last-admin guard via `CountByRole`). Article + push delete with owner-or-mod permission check; push cursor (`p`/`P`) targets pushes for the unified `D` confirm flow under `writeMu`+`tx` (50× concurrent-delete race test mirrors the create canary). Read-only enforcement: `Root.navigate` rejects compose screens for guests with toast; cosmetic help-line tweaks per role; guests skip broker registration so multi-session collisions don't matter.

<!-- Prune older entries into CHANGELOG.md once prior-year items appear here
     or this section grows past ~20 entries. -->
