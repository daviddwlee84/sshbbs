# 水球 → 對話 thread → DM 視窗 (兩階段交付)

## Context

使用者問「水球目前是否有歷史記錄? 能不能做成即時聊天, 水球變成快速回覆的快捷鍵」。

事實核對 (探索結果):

- **水球已經完全持久化** — `water_balloons` 表 (`internal/store/migrations/0001_init.sql:57-68`) 永久保存每一封, 離線 replay (`server.go` Drain unread 段) + server 重啟存活皆 OK。
- **但 UX 暴露不出歷史**, 三個缺口:
  1. `ListInboxFor` 只查 `to_user_id = ?` (`internal/store/waterballoons.go:80-102`) — 看不到自己寄出去的訊息, 不算真正的對話歷史。
  2. 平攤清單, 沒有按對方 grouped, 跟 alice/bob/carol 的訊息會交錯混雜。
  3. 沒有即時 append — 在 inbox 時收到新水球只看到 toast, 列表不會更新。
- **既存基礎設施可以重用** — `chat.Broker.Send(toUID, WBIncomingMsg)` 已是 1-to-1 通道, `Root` 已會在 `WBIncomingMsg` 收訊時 `MarkRead`, mail 模組的 `ScreenMailThread` + `MailThreadID` 是同形 precedent。

**已確認 scope** (透過 AskUserQuestion):

- **兩階段交付**: Phase 1 = 對話 thread 視覺化 (read-only 歷史); Phase 2 = 在同一畫面加 textinput 變成 DM 視窗。
- **Read-marking**: 移除「進 inbox 就全標已讀」, 改成「進入跟某人的 thread 才標該對方的 inbound 為已讀」。

整個 feature **不需要 schema migration** — `water_balloons` 表已經夠用。

---

## Phase 1: 對話 thread (本次交付重點)

### 1. Repo 新增 (`internal/store/waterballoons.go`)

#### 1a. 新型別 `WBCounterparty`

```go
type WBCounterparty struct {
    UserID      int64     // counterparty (對方) user.id
    UserIDStr   string    // 對方目前的 handle (從 users JOIN, 不是 from_userid 快照)
    LastBody    string
    LastFromMe  bool
    LastAt      time.Time
    UnreadCount int64     // 對方寄給 viewer 且未讀的筆數
}
```

#### 1b. `ListCounterpartiesFor(ctx, viewerID, limit) ([]*WBCounterparty, error)`

CTE + GROUP BY 衍生 counterparty id, 再 JOIN 回 `users` 取現行 handle (不信任 `from_userid` 快照, 處理 rename):

```sql
WITH related AS (
    SELECT id, from_user_id, from_userid, to_user_id, body, read_at, created_at,
           CASE WHEN from_user_id = ?1 THEN to_user_id ELSE from_user_id END AS cp_id
    FROM water_balloons
    WHERE from_user_id = ?1 OR to_user_id = ?1
),
agg AS (
    SELECT cp_id,
           MAX(id)         AS last_id,
           MAX(created_at) AS last_at,
           SUM(CASE WHEN to_user_id = ?1 AND read_at IS NULL THEN 1 ELSE 0 END) AS unread_count
    FROM related GROUP BY cp_id
)
SELECT a.cp_id, u.user_id, r.body, (r.from_user_id = ?1), a.last_at, a.unread_count
FROM agg a
JOIN related r ON r.id = a.last_id
JOIN users    u ON u.id = a.cp_id
ORDER BY (a.unread_count > 0) DESC, a.last_at DESC
LIMIT ?2;
```

關鍵點: `MAX(id)` 而非 `MAX(created_at)` 解最新 row (created_at 在 SQLite 只到秒, 會打平); 排序 unread-first 對齊既有 `ListInboxFor` 慣例 (`waterballoons.go:86`)。

#### 1c. `ListConversation(ctx, viewerID, counterpartyID, limit) ([]*WaterBalloon, error)`

```sql
SELECT id, from_user_id, from_userid, to_user_id, body, delivered_live, read_at, created_at
FROM water_balloons
WHERE (from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?)
ORDER BY id ASC
LIMIT ?
```

ASC 順序 → 最舊在上, 最新在下 (chat-style scrollback)。`limit` 預設 200, Phase 1 不分頁; SQL 加 TODO 註記未來「load older」要改 DESC + offset。

#### 1d. `MarkConversationRead(ctx, viewerID, counterpartyID) error`

```sql
UPDATE water_balloons SET read_at = CURRENT_TIMESTAMP
WHERE to_user_id = ? AND from_user_id = ? AND read_at IS NULL
```

只動 inbound 一邊。Hold `s.writeMu` 與其他 mutator 一致 (`waterballoons.go:104-121`)。

#### 1e. `GetByID(ctx, id) (*WaterBalloon, error)`

一行用 `wbColumns` 抓單筆。讓 thread 的 live-append 能精確拿到剛收到的那筆 row。

---

### 2. Inbox 改寫 (`internal/tui/screen_wb.go` 的 `wbInboxModel`)

| 位置 | 改動 |
|------|------|
| `screen_wb.go:22` | `items []*store.WaterBalloon` → `items []*store.WBCounterparty` |
| `screen_wb.go:30` | `ListInboxFor(...)` → `ListCounterpartiesFor(ctx, deps.User.ID, 100)` |
| `screen_wb.go:33-35` | **刪掉整個 `MarkAllReadFor` 區塊** (核心 UX 變更) |
| `screen_wb.go:60-67` | `r/enter/space/right/l` 改 navigate 到 `ScreenWBThread` 並帶 `CounterpartyUserID: it.UserID` (不再直接跳 compose) |
| `screen_wb.go:103-118` | View 欄位改成: Date · 對方 handle · UnreadCount (數字, 0 顯示 "·") · `LastFromMe ? "you: " : ""` + body preview |

`c` (compose) 行為不變。Help line 改 `Enter/→/l open`。

---

### 3. 新增 thread 畫面 `wbThreadModel` (附加在 `screen_wb.go` 尾巴, 跟 inbox/compose 同檔讀比較順)

#### 3a. 結構

```go
type wbThreadModel struct {
    deps     Deps
    cpID     int64                   // counterparty user.id
    cpUserID string                  // 對方目前 handle (顯示用)
    items    []*store.WaterBalloon
    scroll   int
    width, height int
    loadErr  error
}
```

#### 3b. 建構流程 — 鏡像 `screen_mail.go:149-162` 的 mark-read-on-open 模式

1. `Users().GetByID(ctx, counterpartyID)` 抓 cp handle (失敗則 `loadErr`)
2. `ListConversation(ctx, viewerID, counterpartyID, 200)`
3. **`MarkConversationRead(ctx, viewerID, counterpartyID)`** ← 新的 mark-read 觸發點
4. local items 中 `to_user_id == viewer && !read_at.Valid` 的 row 同步把 `ReadAt` 補上 (避免本次 render 還顯示 NEW)

#### 3c. Update 鍵位 (對齊 list-screen 慣例, 表單例外規則不適用)

- `esc/backspace/left/h` → `ScreenWBInbox`
- `Q` → `ScreenMainMenu`
- `up/k`, `down/j` → scroll (`maxScroll()` clamp)
- `g/home`, `G/end` → 頭 / 尾
- `c`, `r` → `ScreenWBCompose` 帶 `Recipient: m.cpUserID` (Phase 2 會把這兩個鍵改成 inline input focus)

#### 3d. 即時 append (Phase 1 包含 — 約 15 行, 是 Phase 2 的承重)

```go
case WBIncomingMsg:
    if !strings.EqualFold(msg.FromUserID, m.cpUserID) { return m, nil }
    ctx := context.Background()
    if wb, err := m.deps.Store.WaterBalloons().GetByID(ctx, msg.ID); err == nil {
        m.items = append(m.items, wb)
    }
    _ = m.deps.Store.WaterBalloons().MarkRead(ctx, msg.ID)
    return m, nil
```

附帶修一個既存 UX bug: 目前 alice 在 thread 看 bob 對話時 bob 寄新訊息, alice 只看到 toast, thread 不會更新; 加完這段就會 live append。

**Phase 1 不抑制 toast** — 維持 Root 的 toast + thread 同時顯示, 已有 visual confirmation 不算干擾; toast 抑制留給 Phase 2。

#### 3e. View

每筆訊息 sender label + 時戳 + body (wrap 到 width-4):

```
  alice  2026-05-06 14:23
    hi how's it going

  you    2026-05-06 14:24
    pretty good thanks
```

Sender 標籤: 入訊用 `StyleHighlight` + `m.cpUserID` (現行 handle, 不用 `w.FromUserIDStr` 快照, 處理 rename); 出訊 `StyleDim` + "you"。Viewport 風格: skip 前 `m.scroll` 行, 印 `m.height-4` 行。空 thread 用 `(empty thread)` placeholder, 鏡像 `screen_mail.go:220-223`。

---

### 4. Routing (`internal/tui/messages.go` + `root.go`)

#### 4a. `messages.go`

```go
ScreenWBThread          // 在 ScreenWBCompose 後、ScreenOnline 前, WB 系列連續放
```

`NavigateMsg` 加新欄位 (跟 `MailThreadID` 同形 precedent):

```go
CounterpartyUserID int64 // for ScreenWBThread: which DM thread to open
```

不重用 `Recipient` (string) — 會逼每個 navigate site 多做一次 `GetByUserID` 查詢; 加一個 int64 欄位是一行的事。

#### 4b. `root.go` (插在 line 162-164 `ScreenWBInbox` / `ScreenWBCompose` 之間)

```go
case ScreenWBThread:
    sub = newWBThreadModel(m.deps, n.CounterpartyUserID)
```

---

### 5. Online list 入口 (Phase 1 一併, 約 6 行)

`internal/tui/screen_online.go` 新增 `t` 鍵: 對著選中的線上使用者開 thread。`Enter` 維持原行為 (開 compose)。Help-line 補 `t thread`。

```go
case "t":
    if len(m.users) == 0 { return m, nil }
    u := m.users[m.cursor]
    return m, func() tea.Msg { return NavigateMsg{To: ScreenWBThread, CounterpartyUserID: u.UserID} }
```

---

### 6. 測試

**Repo (`internal/store/waterballoons_test.go`):**

- `TestWaterBalloons_ListConversation_BothDirections` — 雙向 row 都回傳, ASC by id
- `TestWaterBalloons_ListConversation_FiltersOtherUsers` — 三人之間, 查 (alice, bob) 不漏 carol
- `TestWaterBalloons_ListConversation_Limit` — limit=3 取最舊 3 筆 (記下將來改 DESC + offset)
- `TestWaterBalloons_ListCounterpartiesFor_Grouping` — 多個 counterparty 各一 row
- `TestWaterBalloons_ListCounterpartiesFor_UnreadCount` — 對的人 unread, 不對的人 0
- `TestWaterBalloons_ListCounterpartiesFor_LastFromMe` — 最後一封自己寄出時 `LastFromMe=true`
- `TestWaterBalloons_ListCounterpartiesFor_UnreadFirst` — 有未讀者排前 (即使 last_at 較舊)
- `TestWaterBalloons_ListCounterpartiesFor_RenamedCounterparty` — 對方 rename 後, `UserIDStr` 反映現行 handle
- `TestWaterBalloons_MarkConversationRead_OnlyInbound` — 只動 inbound, 不誤動 outbound rows
- `TestWaterBalloons_MarkConversationRead_Idempotent` — 連呼兩次無誤
- `TestWaterBalloons_GetByID` — round-trip

**TUI (新增 `internal/tui/screen_wb_test.go`, fixture 仿 `screen_mail_test.go:14-27`):**

- `TestWBInbox_LoadsCounterparties`
- `TestWBInbox_NoAutoMarkRead` ← regression for §2 line 33-35 移除
- `TestWBInbox_BackKeys` (esc/backspace/left/h/Q → ScreenMainMenu)
- `TestWBInbox_OpenThread` (enter/space/l/right → `ScreenWBThread` 帶 `CounterpartyUserID`)
- `TestWBInbox_ComposeKey` (c → `ScreenWBCompose` no recipient)
- `TestWBInbox_NoopsWhenEmpty`
- `TestWBThread_LoadsBothDirections`
- `TestWBThread_OpeningMarksReadInbound` ← unread=2 in, 進入後 0; outbound 不被誤動
- `TestWBThread_BackKeys` / `TestWBThread_QuitKey`
- `TestWBThread_ComposeKey` (c, r 都帶 `Recipient=alice.UserID`)
- `TestWBThread_LiveAppendOnIncoming` (`WBIncomingMsg` from cp → items grew + DB read)
- `TestWBThread_LiveAppendIgnoresOtherCounterparty`

全部 `go test -race ./...`。

---

### 7. Sequencing (建議 commit 切分)

每步單獨可 build + 綠燈, 利於 review:

1. Repo + 測試 (新方法新型別; inbox 仍用舊 query)
2. `messages.go` 加 const + 欄位; `root.go` 加 case (功能未串通但可編譯)
3. `wbThreadModel` 構造 + 測試
4. Inbox 改寫 + 測試 (含 mark-read 行為變更的 regression)
5. Thread 的 `WBIncomingMsg` live-append + 測試
6. (Optional) `screen_online.go` 的 `t` 鍵 + 測試

---

## Phase 2: DM 視窗 (sketch, 不在本次落地)

`wbThreadModel` 升級成 split-pane:

- 上半 scrollback (沿用 Phase 1 的 view + scroll)
- 下半 `textinput` (一行 input, 240 char limit 同 wb compose)
- `Tab` 切換 scrollback / input focus; `j/k` 只在 scrollback focused 時滾動
- `Enter` (input focused) → 走現行 `wbComposeModel.submit` 同樣的 `Insert` + `Broker.Send` 路徑, 成功後 `m.items = append(...)` 即時顯示, 清空 input
- Toast 抑制: `Root.Update` 收 `WBIncomingMsg` 時若 active screen 是符合對方的 `wbThreadModel`, 跳過 toast (避免重複)
- Online list 的 `t` (Phase 1 已加) 自然成為「跟某人開 chat」入口

---

## 需要修改的關鍵檔案

| 檔案 | 改動性質 |
|------|---------|
| `internal/store/waterballoons.go` | 加 `WBCounterparty`, `ListCounterpartiesFor`, `ListConversation`, `MarkConversationRead`, `GetByID` |
| `internal/store/waterballoons_test.go` | 新增 11 條測試 |
| `internal/tui/screen_wb.go` | inbox 改寫 + 新 `wbThreadModel` |
| `internal/tui/screen_wb_test.go` | **新檔** — TUI 鍵位 / mark-read regression / live-append |
| `internal/tui/messages.go` | `ScreenWBThread` const + `CounterpartyUserID int64` 欄位 |
| `internal/tui/root.go` | navigate switch 新 case |
| `internal/tui/screen_online.go` | (optional) `t` 鍵入口 |

**不動**: SQL migrations, broker, server.go drain 邏輯, compose 流程, toast 行為。

---

## Verification (兩個 SSH session 手動測)

```bash
make hostkey                       # 若還沒做
make run                           # term 0
ssh alice@localhost -p 2222        # term A
ssh bob@localhost   -p 2222        # term B
```

1. **雙向歷史**: B 在 inbox `c` 寫給 alice "hi from bob"; A 收到 toast。A 反向寄 "hey bob"。再來幾筆。
2. **離線未讀**: 中斷 A; B 連寄兩封給 alice。
3. **Reconnect**: 重連 alice — toast replay (`server.go` Drain unread) 兩封。
4. **新 inbox**: A 按 Ctrl+U → 應看見 1 列 `bob`, unread=0 (replay 已標讀), preview 是最新 body。
5. **Drill in**: A Enter 進 bob thread → 全 5 封 ASC 排, you/bob 標籤對, j/k 可滾。
6. **Read regression**: A 退出再開 — 無 toast 補播 (DB 都已讀)。
7. **Live append**: A 待在 bob thread; B 寄 "live test" → A 同時看到 toast **和** thread 底部新 row。`sqlite3 data/bbs.db "SELECT id,body,read_at FROM water_balloons ORDER BY id DESC LIMIT 5"` 確認 read_at 有值。
8. **Roll-up**: 第三人 carol 寄一封給 alice → A 的 inbox 變兩列, carol 在 bob 之上 (unread-first)。
9. **Rename 容忍**: `sqlite3 data/bbs.db "UPDATE users SET user_id='bob2' WHERE user_id='bob'"` → A 的 inbox 該列顯示 `bob2` (JOIN 取現行 handle)。
10. **Online list `t`**: A 開 online list, 對 bob 按 `t` → 直接進 thread。

也跑 `make test-race` 確認 11 條 repo 測試 + 12 條 TUI 測試全綠。

---

## 已評估的邊界情況 / 已知事項

- **Self-WB 未阻擋** (`screen_wb.go:234`): 寄給自己會成功, CTE 把自己當 cp; 出格但無害。**Phase 1 不修**, 寫 TODO 留在 `wbComposeModel.submit` 上方。
- **未實作 user delete**: 現行 codebase 無 `DELETE FROM users`; 將來加上時 `ListCounterpartiesFor` 的 JOIN 會默默不顯示對話。在新 SQL 加 TODO 註記。
- **`from_userid` 快照 vs 現行 handle**: 一律走 JOIN `users` 拿現行 handle (inbox 標題 + thread sender label), 不信任 row 上的快照欄位。
- **Pagination**: Phase 1 limit=200 截掉最舊。SQL 加 TODO 留將來改 DESC + offset。
