# 05 · 水球 (water balloon) — design notes

## pttbbs reference

In pttbbs, a 水球 is a one-line instant message between two **online** users.
The mechanism is a combination of:

- The `userinfo_t` shared-memory array (UTMP-like; see 06).
- A "talk" / "page" pipe handled by `mbbsd` per session, via Unix signals
  + a per-session message buffer in shared memory.
- Pager settings (`uflag` bits) so users can mute, restrict to friends, or
  enable do-not-disturb.

Because delivery only happened to currently-connected sessions, an offline
recipient simply missed the message — no persistence.

## Our model

We persist every water balloon to SQLite **and** attempt live delivery via
the in-process broker. Offline recipients see the message replayed on
their next connect.

```sql
CREATE TABLE water_balloons (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    from_user_id    INTEGER NOT NULL REFERENCES users(id),
    from_userid     TEXT    NOT NULL,                          -- denormalized
    to_user_id      INTEGER NOT NULL REFERENCES users(id),
    body            TEXT    NOT NULL,
    delivered_live  INTEGER NOT NULL DEFAULT 0,
    read_at         DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_wb_to_unread ON water_balloons(to_user_id, read_at);
```

## Send flow (`internal/tui/screen_wb.go : wbComposeModel.submit`)

1. Look up recipient by user_id (case-insensitive). Refuse if no such user.
2. `WBRepo.Insert(...)` persists the row.
3. `Broker.Send(target.ID, WBIncomingMsg{ID, FromUserID, Body})` attempts
   live delivery to every active session of the recipient. Returns true if
   delivered (i.e. recipient online).
4. If delivered, `WBRepo.MarkRead(ctx, wb.ID)` clears the unread flag —
   the toast was the read.

## Receive flow

- **Live**: `Broker.Send` calls `program.Send(WBIncomingMsg)` on each of the
  recipient's `*tea.Program`s. Their `tui.Root.Update` shows the toast in
  the footer (3-second decay) and marks the row read.
- **Replay on reconnect** (`internal/server/server.go : makeProgramHandler`):
  immediately after broker registration we drain `WBRepo.ListUnreadFor(uid)`
  and `Send` each as `WBIncomingMsg` to the freshly-created program. Each
  is marked read as it's drained.

## Inbox

`Ctrl+U` from any logged-in screen opens `wbInboxModel`, which lists every
WB the user has received (newest unread first, then read newest-first).
Press `c` to compose, `r` to reply (prefills the recipient).

## What we drop / defer

- **Pager settings / friends-only** (PTT `pager_*` bitfields) — P1.
- **Multi-line / thread "talk"** (PTT's full talk session with screen split) —
  out of scope.
- **Real `delivered_live` accounting.** Currently always inserted as
  `delivered_live = 0` because the delivery flag is known only after the
  insert (we'd need an UPDATE after Send). Tracked as a follow-up; the field
  is purely informational at the moment.
