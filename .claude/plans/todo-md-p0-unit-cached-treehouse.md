# Plan: P1 batch — PTT-feel polish, Online list, Article broadcast, Mailbox

## Context

P0 (MUST HAVE) shipped — see `## Done` in `TODO.md`. The next batch is the
five items in `## P1` of `TODO.md`: three `[S]` polish items plus two `[M]`
features. They share the same goal — make the BBS *feel* like PTT once
you're past register/login: keys do what muscle memory expects, you can see
who's online, and you have a real persistent inbox.

The exploration confirmed these can ship in one batch without rework:

- All keybinding work plugs into the existing per-screen tables; tests
  follow the established `[]string{"esc","backspace","left","h"}` table
  pattern in `internal/tui/screen_*_test.go`.
- The "send to everyone, filter on receive" broadcast pattern in
  `screen_article_view.go` (PushAddedMsg) is a direct template for the
  new `ArticleAddedMsg`.
- `WaterBalloonRepo` + `screen_wb.go` is a near-perfect blueprint for
  `MailRepo` + `screen_mail_*.go` — the only structural addition is
  threading (`parent_id` / `thread_id`).
- `chat.Broker.OnlineList()` already returns `[]OnlineUser` and is
  already tested — the online-user screen is purely a TUI shell.

Phases are written so each is independently shippable. You can stop
after Phase 1 if scope creeps; Phase 2 items don't depend on each other.

---

## Phase 1 — Quick wins (three `[S]` items)

### 1.1 PTT-style extra hotkeys

**Goal**: `[`/`]` (prev/next) and `Q` (quit-to-menu) on every list/menu
screen. `r` is **dropped** from board view (PTT convention reserves `r`
for "read" on lists and `R` for reply on the article view; binding our
own meaning would clash with muscle memory).

**Files to modify** (key handler + table-driven tests):

- `internal/tui/screen_board_list.go` — add `[`/`]` aliases for
  cursor up/down; `Q` returns `NavigateMsg{To: ScreenMainMenu}`.
- `internal/tui/screen_board_view.go:40-73` — add `[`/`]` aliases for
  cursor up/down; `Q` → main menu.
- `internal/tui/screen_article_view.go` — add `[`/`]` for **prev/next
  sibling article** in the same board (see new repo method below); `Q`
  → main menu.
- `internal/store/articles.go` — add
  `NeighboursOf(ctx, boardID, articleID int64) (prev, next int64, err error)`.
  Two `SELECT id FROM articles WHERE board_id=? AND id <? ORDER BY id DESC LIMIT 1`
  / `> ? ORDER BY id ASC LIMIT 1` queries; returns 0 sentinel when at the
  edge. Mind any soft-delete column once P2 ships (currently no filter
  needed).
- `internal/tui/messages.go` — `[`/`]` on article view emits
  `NavigateMsg{To: ScreenArticleView, BoardID: ..., ArticleID: prev/next}`;
  current `NavigateMsg` already has `BoardID` and `ArticleID` — no
  schema change.

**Tests** (extend existing tables, no new files except for the repo):

- `internal/tui/screen_board_list_test.go:34-48` — extend the back-key
  and forward-key tables with the new aliases (`[` and `]`); add a
  separate test for `Q` → ScreenMainMenu.
- `internal/tui/screen_board_view_test.go` — same pattern.
- `internal/tui/screen_article_view_test.go` — table covering `[`/`]`
  navigation across a fixture of three articles in one board: at the
  middle article both keys produce a NavigateMsg with the right
  ArticleID; at the edges the no-op sentinel produces no NavigateMsg.
- `internal/store/articles_test.go` — add `TestArticles_NeighboursOf`
  covering: middle article returns both neighbours, first article
  returns prev=0, last article returns next=0, single-article board
  returns both 0, wrong boardID returns 0/0.

### 1.2 Article pagination — `g` / `G`

`PgUp`/`PgDn` already exist as `b`/`pgup` and space/`pgdown` aliases
(see `screen_article_view.go:79-88`). Only `g` (top) and `G` (bottom)
are missing.

**Files to modify**:

- `internal/tui/screen_article_view.go:79-88` — add `case "g":
  m.scroll = 0` and `case "G": m.scroll = max(0, len(bodyLines) -
  viewportHeight)`. Compute `viewportHeight` the same way the existing
  paging keys do.

**Tests**:

- `internal/tui/screen_article_view_test.go` — add a table for scroll
  keys (`["g","G","b","pgup","space","pgdown"]`) asserting the resulting
  `m.scroll` value against a fixture article with N body lines.

### 1.3 Online-user list screen

**Files to create**:

- `internal/tui/screen_online.go` — new model `onlineModel`. Init pulls
  `m.deps.Broker.OnlineList()` (returns `[]chat.OnlineUser`), renders
  one line per user: `userid (N sessions)`. Cursor `j`/`k`, `h`/esc back
  to main menu, `l`/enter optionally opens a water-balloon compose
  pre-filled with the recipient (mirrors `NavigateMsg{To: ScreenWBCompose,
  Recipient: ...}`).

**Files to modify**:

- `internal/tui/messages.go:3-23` — add `ScreenOnline` constant.
- `internal/tui/root.go:109-141` — add a `case ScreenOnline:` arm in
  `navigate()` that constructs `newOnlineModel(m.deps)`.
- `internal/tui/screen_main_menu.go:22-30` — insert as **slot 3**
  ("線上使用者 Online"); bump Quit to **slot 4**. Update the numeric
  shortcut handler accordingly.
- `internal/tui/screen_main_menu_test.go:110-132` — extend the numeric
  shortcut table to cover the new slot 3 → ScreenOnline mapping and
  slot 4 → Quit.

**Tests**:

- `internal/tui/screen_online_test.go` — new file. Use a fake broker
  (or stub `OnlineList()` via a small interface in `Deps` if needed —
  check whether the current `Deps.Broker` is concrete or interface;
  if concrete, pass through anyway and pre-register fake sessions
  with a real `chat.Broker` in tests, mirroring `broker_test.go`'s
  `fakeSender` approach).
- Cover: empty list rendering, cursor movement, enter-to-WB-compose
  navigation, h/esc back to main menu.

---

## Phase 2 — Two `[M]` features

### 2.1 Live broadcast of new articles (`ArticleAddedMsg`)

Direct mirror of the `PushAddedMsg` pattern.

**Files to modify**:

- `internal/tui/messages.go:36-44` — add:
  ```go
  type ArticleAddedMsg struct {
      BoardID    int64
      ArticleID  int64
      AuthorUserID string
      Title      string
  }
  ```
- `internal/tui/screen_post_compose.go:101-124` (`submit`) —
  - Capture the `*Article` returned by `Articles().Create()` (currently
    discarded on line 114).
  - After `IncrementPosts` succeeds, call
    `m.deps.Broker.SendToAll(u.ID, ArticleAddedMsg{BoardID: m.boardID,
    ArticleID: art.ID, AuthorUserID: u.UserID, Title: art.Title})`
    if `m.deps.Broker != nil`.
- `internal/tui/screen_board_view.go` — handle `ArticleAddedMsg`:
  ```go
  case ArticleAddedMsg:
      if msg.BoardID == m.board.ID {
          // re-fetch article list from DB (canonical state)
          if arts, err := m.deps.Store.Articles().ListByBoard(ctx, m.board.ID); err == nil {
              m.articles = arts
          }
      }
  ```
  Mirrors the `PushAddedMsg` re-fetch pattern in `screen_article_view.go:54-66`.

**Tests**:

- `internal/tui/screen_post_compose_test.go` — assert that a successful
  submit returns a `tea.BatchMsg` (or sequence) that includes both the
  `NavigateMsg` and a broker `SendToAll` invocation. Use a fake broker
  satisfying `chat.Sender`-like semantics (or expose a recording broker
  via the existing `Sender` interface).
- `internal/tui/screen_board_view_test.go` — feed an `ArticleAddedMsg`
  with matching `BoardID` and assert `m.articles` was re-fetched (one
  more entry); feed a non-matching `BoardID` and assert no change.

### 2.2 Mailbox 站內信

Persistent threaded messages — distinct from water balloons (transient
toast). Direct structural analog of the WaterBalloon repo + screens, plus
a thread/parent column.

**Files to create**:

- `internal/store/migrations/0003_add_mail.sql`:
  ```sql
  CREATE TABLE mail (
      id              INTEGER PRIMARY KEY AUTOINCREMENT,
      thread_id       INTEGER,                  -- root mail id; NULL ⇒ self-root
      parent_id       INTEGER REFERENCES mail(id),
      from_user_id    INTEGER NOT NULL REFERENCES users(id),
      from_userid     TEXT NOT NULL,
      to_user_id      INTEGER NOT NULL REFERENCES users(id),
      subject         TEXT NOT NULL,
      body            TEXT NOT NULL,
      read_at         DATETIME,
      created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
  );
  CREATE INDEX idx_mail_to_unread ON mail(to_user_id, read_at);
  CREATE INDEX idx_mail_thread ON mail(thread_id);
  ```
  On insert, if `parent_id IS NULL` then `thread_id = id` (set with a
  follow-up UPDATE inside the same transaction, like `PushRepo.Create`'s
  two-statement pattern).

- `internal/store/mail.go` — `MailRepo` mirroring `WaterBalloonRepo`
  (see `internal/store/waterballoons.go`):
  - `Insert(ctx, fromID, fromUserID, toID, subject, body, parentID *int64) (*Mail, error)`
  - `ListInboxFor(ctx, toUserID, limit int) ([]*Mail, error)` — unread first,
    ordered by `(read_at IS NULL) DESC, id DESC`
  - `ListThread(ctx, threadID int64) ([]*Mail, error)` — chronological
  - `MarkRead(ctx, id int64) error`
  - `MarkAllReadFor(ctx, toUserID int64) error`
  - All write methods grab `Store.writeMu` (process-level mutex
    discipline — see `CLAUDE.md` for why).

- `internal/tui/screen_mail_inbox.go` — list view (subject, from,
  date, status). Cursor `j`/`k`, enter/`l` opens thread view, `c`
  composes new, `r` replies to highlighted, `h`/esc back.
- `internal/tui/screen_mail_thread.go` — chronological list of one
  thread; `r` to reply (pre-fills `parent_id` and `subject` with `Re:`
  prefix if not already present); `h`/esc back to inbox.
- `internal/tui/screen_mail_compose.go` — three-field form: To,
  Subject, Body (vs WB's two-field). `Tab` cycles focus, `Ctrl+S`
  sends. Optionally pre-fill recipient/parent_id from a NavigateMsg.

**Files to modify**:

- `internal/store/store.go:25,51,65` — add `mail *MailRepo` field,
  initialize in `Open()`, add `Mail()` accessor.
- `internal/tui/messages.go` — add `ScreenMailInbox`, `ScreenMailThread`,
  `ScreenMailCompose` constants. Extend `NavigateMsg` with `MailID` and
  `MailThreadID` fields if the existing ones (`BoardID`, `ArticleID`,
  `Recipient`) don't suffice for thread-view routing.
- `internal/tui/root.go:109-141` — add `case ScreenMailInbox/Thread/Compose`
  arms.
- `internal/tui/screen_main_menu.go` — add a "信箱 Mail" item (slot 3
  if 1.3 hasn't shipped; slot 4 otherwise; bump Quit accordingly).
- `internal/server/server.go` — on session login, fetch unread mail
  count via `MailRepo.CountUnreadFor` and surface as a one-time toast
  on the main menu (mirrors the on-reconnect water balloon replay
  pattern at server.go).

**Tests**:

- `internal/store/mail_test.go` — mirrors `waterballoons_test.go`:
  Insert (root + reply), thread-id self-fix on root, ListInboxFor
  ordering, ListThread chronological, MarkRead, MarkAllReadFor.
  Use `storetest.New(t)` and `storetest.MustUser()`.
- `internal/tui/screen_mail_inbox_test.go`,
  `internal/tui/screen_mail_thread_test.go`,
  `internal/tui/screen_mail_compose_test.go` — `Update()` tests
  mirroring `screen_wb_test.go`. Cover keybinding tables, navigation,
  send-success flow.

---

## Resolved decisions

- **Scope** — implement all five P1 items in this batch (Phase 1 + Phase 2).
- **`r` on board view** — dropped. Enter/`l` already opens the article;
  `r`/`R` reply is reserved for article view in a later batch so we
  don't preempt PTT's reply-on-article-view convention.
- **`[`/`]` on article view** — *does* navigate to prev/next sibling
  article via the new `ArticleRepo.NeighboursOf` method. Tests land
  in `articles_test.go` alongside the existing repo tests.

## Still to decide (cheap; resolve during implementation)

- **Mailbox unread badge on main menu** — whether to render `信箱 Mail (N)`
  when unread > 0. Adds a `MailRepo.CountUnreadFor` call on main-menu
  Init plus a `MailAddedMsg` listener for live refresh. Cheap to add
  later; not a blocker for the first cut.

---

## Verification

After each phase:

```bash
make test-race                # CI standard — must stay green
scripts/todo-kanban.sh --validate-only TODO.md
```

Manual smoke (two SSH sessions, `alice` + `bob`):

- **Phase 1.1**: open board list, press `[`/`]` — cursor moves; press
  `Q` — back to main menu.
- **Phase 1.2**: open a long article (use seed-data CLI or paste a
  long body via post compose), press `g`/`G` — scroll jumps to top/bottom.
- **Phase 1.3**: connect both sessions; press `3` from main menu in
  either — sees the other (and self) in the online list.
- **Phase 2.1**: alice on `screen_board_view` of board X; bob posts
  to board X — alice's list refreshes within ~1 frame without
  re-entering. bob posts to board Y while alice is on X — alice's
  list does NOT refresh.
- **Phase 2.2**: alice sends a mail to bob; bob disconnects, reconnects,
  sees unread on main menu, reads in inbox; bob replies; alice sees the
  thread on next inbox open. Concurrent insert × 50 race test on
  `MailRepo.Insert` mirrors `TestPushes_ConcurrentScoreAtomicity`.

After each item ships:

```bash
scripts/promote-todo.sh --title "<substring>" --summary "<one-liner>"
```

---

## Out of scope (stays in `TODO.md`)

- All `## P2` items (soft delete, BM list, seed CLI, splash, pager
  settings, board attr, public-key auth, friends/blocklist, slog,
  rate limiting, FTS5).
- All `## P3` items (sqlc, web mirror, prometheus).
- The `## P?` PTT `.DIR` import spike — see `backlog/ptt-import-spike.md`.

If the user wants to reduce scope mid-flight, the natural cut points are
"stop after Phase 1" (3 small wins, no schema change) or "stop after 2.1"
(everything except mailbox).
