# Plan: `make watch` 開發用熱重啟模式

## Context

**問題**：目前 `make run` 直接 `go run ./cmd/sshbbs ...`，每次改 Go 檔案都要 `Ctrl-C` → `make run` 重來，dev iteration 很慢。使用者問：能否做出類似 `npm run dev` 的 watch 模式？Go 編譯式語言會不會做不到？

**答案**：完全做得到。Go 確實沒有 Erlang/Elixir 那種「不中斷連線熱換碼」，但「watch source → rebuild → restart」這套流程跟 `nodemon` 是同等成熟的 dev pattern，社群標準工具是 [`air-verse/air`](https://github.com/air-verse/air)。

**這個專案的限制**：
- SSH client 在每次 rebuild 時**會被斷線**（無法避免；Go binary 一旦結束 process 就沒了）。但因為：
  - `data/bbs.db`（SQLite）跨重啟保留 → 文章、推文、登入狀態都還在
  - `.ssh/host_ed25519` 跨重啟保留 → client 不會看到 host-key-mismatch
  - `cmd/sshbbs/main.go:96-131` 已實作 graceful shutdown：收到 SIGINT 後 3s drain 視窗 + 對所有 live `tea.Program` 廣播 `tea.Quit()` + deferred `st.Close()` flush WAL
  - 使用者只需重新 `ssh alice@localhost -p 2222` 就能接續看到新 build
- 換句話說，現有 graceful shutdown pipeline 已經把 hot-reload 該有的清理都做了，watcher 只要會 SIGINT + 等 ≥3s + 啟新 process 就行 — air 預設就支援。

**目標體驗**：
```
make watch      # 啟動 watcher，背景跑 build + run
# 在另一個 terminal 改 internal/tui/screen_article_list.go
# air 自動偵測 → go build → SIGINT 舊 process → 啟動新 process
# 重新 ssh 進去就看到新 UI
```

## 推薦做法：`air` 透過 Go 1.24+ `go tool` directive 宣告

不用額外 `go install`，不污染全域 GOBIN，版本 pin 在 `go.mod` / `go.sum`，`git clone` 後 `make watch` 直接可用。

### 變更檔案

#### 1. `go.mod` — 加入 air 為 tool dependency

執行一次（會由 ship 階段執行；plan 階段不執行）：
```bash
go get -tool github.com/air-verse/air@latest
```

這會在 `go.mod` 結尾加入 `tool github.com/air-verse/air` directive 並補上對應的 `require`。`go.sum` 會自動更新。

之後 `go tool air` 即可呼叫，不需 PATH 上裝 air。

#### 2. 新增 `.air.toml`（repo root）

```toml
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/sshbbs ./cmd/sshbbs"
  bin = "./tmp/sshbbs"
  args_bin = ["-addr=:2222", "-db=data/bbs.db", "-hostkey=.ssh/host_ed25519"]
  delay = 500                  # debounce in ms
  send_interrupt = true        # SIGINT first, not SIGKILL
  kill_delay = "4s"            # 4s > main.go 的 3s drainTimeout，給 graceful shutdown 喘息
  stop_on_error = false
  include_ext = ["go", "sql"]  # sql 也要 watch — internal/store/migrations 是 go:embed
  exclude_dir = [
    "tmp", "data", ".ssh", "dist",
    ".specstory", ".claude", "scripts",
    "docs", "backlog", "pitfalls",
    "testdata",
  ]
  exclude_regex = ["_test\\.go$"]   # 測試檔不觸發 server 重啟（用 make test 即可）

[log]
  time = true

[misc]
  clean_on_exit = true
```

**設計重點**：
- `kill_delay: "4s"` 必須 ≥ `main.go:97` 的 `drainTimeout = 3 * time.Second`，否則 air 會在 graceful shutdown 還在跑時就 SIGKILL，可能讓 SQLite WAL 沒 checkpoint
- `send_interrupt = true` 讓 air 發 SIGINT 而非預設的 SIGTERM；現有 main.go `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` 兩種都收，但 SIGINT 跟使用者 `Ctrl-C` 路徑完全一致，比較好除錯
- `include_ext` 包含 `sql`：`internal/store/migrate.go` 用 `go:embed migrations/*.sql`，SQL 改了必須 rebuild
- `exclude_regex` 排除 `_test.go`：避免寫測試時無謂重啟 server
- `exclude_dir` 排除大量 agent / build artifact 目錄，避免 `.specstory/history/*.md` 寫入時觸發 rebuild

#### 3. `Makefile` — 加 `watch` target

在 `.PHONY` 行加 `watch`，並在 `run` 後面新增：
```makefile
watch: hostkey
	@mkdir -p tmp
	go tool air -c .air.toml
```

依賴 `hostkey` 確保第一次跑也會生 host key（與 `run` 一致）。

#### 4. `.gitignore` — 加 `/tmp/`

在 build artifacts 區塊（line 1–5）加：
```
/tmp/
```

air 的預設 build 輸出目錄。

#### 5. `README.md` — 開發章節補一段

定位（grep 確認）：找到目前介紹 `make run` 的段落，補上：
> **Watch mode (auto-rebuild on save)**: `make watch` — uses `air` (declared as a Go tool dep, no install needed). Edits to `.go` / `.sql` under `cmd/` and `internal/` trigger SIGINT → graceful drain → rebuild → relaunch. Active SSH clients will be disconnected; `data/bbs.db` and `.ssh/host_ed25519` persist, so just reconnect.

#### 6. `CLAUDE.md` — Common commands 區塊補 `make watch`

在現有 ` ```bash ` block（`make hostkey` / `make run` / `make build` 那段）加上一行：
```bash
make watch              # auto-rebuild on .go/.sql changes; SIGINT-then-restart
```

### 不做的事（避免 scope 蔓延）

- **不**加 `make watch-test` 之類的 test watcher — `go test` 已經夠快，留給使用者自己組合 `find ... | entr` 即可
- **不**改 `cmd/sshbbs/main.go` 的 shutdown 邏輯 — 已經為 graceful drain 寫好了，air 直接用
- **不**做 in-process hot-reload（`plugin` package / `yaegi` interpreter）— 工程量遠大於 value，且會破壞 wish + bubbletea 的 program 生命週期
- **不**廣播「server restarting」訊息給活著的 session — 會讓 main.go 多一個耦合點，且 `tea.Quit()` 之後 view 已經 unmount，看不到 toast；使用者看到的 UX 跟手動 Ctrl-C 一樣
- **不**加 `TODO.md` 條目 — 這是直接實作的功能，不是 backlog

### Critical files

- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/Makefile` — 加 `watch` target
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/.air.toml` — 新檔
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/.gitignore` — 加 `/tmp/`
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/go.mod` — 由 `go get -tool` 自動編輯
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/README.md` — dev 章節補一段
- `/Volumes/Data/Program/tries/2026-04-29-ssh-bbs/CLAUDE.md` — Common commands 區塊補一行

### 驗證步驟（end-to-end）

1. **冷啟動**：
   ```bash
   make watch
   ```
   應看到 air 印 banner，`go build` 編完後 server log `sshbbs ... listening on :2222`。

2. **改一個 view**（最快驗證）：編輯 `internal/tui/screen_main_menu.go`，改一個 menu label 文字，存檔。
   - air 應印 `building...` → `running...`
   - 5 秒內新 process 應上線

3. **連線確認**：
   ```bash
   ssh alice@localhost -p 2222
   ```
   應看到改後的文字。再改一次、存檔、重新 `ssh`，應看到再次更新。

4. **Graceful drain 驗證**：保持一個 SSH session 連線中，存一次 `.go` 檔。
   - air 印 `interrupt`
   - 該 client 應在 ≤ 4 秒內收到 server 主動斷線（不是 SIGKILL 的不乾淨斷線）
   - server log 應出現 `draining 1 active session(s)` 與 `all sessions drained`（main.go:111, 121）

5. **SQL migration watch**：在 `internal/store/migrations/` 加一個 dummy `0099_test.sql`（內容 `-- noop`），存檔。air 應重 build；server boot 時 `migrate.apply()` 會 skip 已 applied 的並印新 version。**驗證後刪除這個檔案**。

6. **測試檔不觸發**：編輯任何 `*_test.go`，air 應**不**重 build（被 `exclude_regex` 擋下）。

7. **乾淨退出**：在 `make watch` terminal 按 `Ctrl-C`。air 應 forward SIGINT 給 child，main.go 走 `<-ctx.Done()` 路徑，最後 `clean_on_exit = true` 把 `tmp/` 清掉。
