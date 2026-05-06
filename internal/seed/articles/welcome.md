---
title: 歡迎來到 SSH-BBS
board: Welcome
author: admin
---

哈囉，歡迎來到這個簡化版的 PTT 風格 BBS。整個站台就是一個 SSH server：
你看到的所有東西都跑在 wish + bubbletea 上面，後端用 SQLite 存資料。

# 這裡能做什麼

- **看板**：預設有 `Welcome`（這裡）、`Test`、`ChitChat`。
- **發文 / 讀文**：UTF-8 全程支援，CJK 寬度自動處理。
- **推文**：在文章畫面按 `+` 推、`-` 噓、`=` 留 →。其他線上看同篇的人會即時看到。
- **水球**：私訊一行話。`Ctrl+U` 開水球收件匣，離線收到的也會在你重連時補播。
- **站內信 (mail)**：水球留不下，重要的話寫信。`Ctrl+U` 進收件匣後切到郵件分頁。

# 鍵盤總覽

```
↑/↓ 或 j/k     上下移動
←/→ 或 h/l     上下層導覽（表單例外，h/l 留給文字編輯）
Enter / Space  進入 / 翻頁
Esc / Backspace 返回
g / G          捲到頂 / 底
[ / ]          上一篇 / 下一篇
p              在看板裡發新文
+ / - / =      推 / 噓 / →
y              匯出此篇為 markdown
E              編輯此篇（作者本人或 mod 以上）
D              刪除此篇 / 推文（作者本人或 mod 以上）
M              置頂 / 取消置頂此篇（mod 以上；用於板規 / 公告）
Ctrl+U         水球 / 站內信
Ctrl+C         斷線離開
```

# 權限

- **guest**：唯讀，只看不寫。SSH 用 `guest` 帳號連進來不需密碼。
- **user**：一般註冊帳號。`ssh new@host -p 2222` 註冊，密碼欄位會被忽略，用 TUI 內表單填。
- **mod**：可以刪 / 編輯任何人的文與推文。
- **admin**：站長，可以調整他人權限。

# 文章是 markdown

每篇文章都能用 markdown 表達。在文章畫面按 `y` 會跳到匯出檢視，
你可以選 (1) 純內文 (2) 含留言 (3) 寫到伺服器端檔案 (`data/exports/<你的帳號>/`)，
或按 `c` 推到終端機剪貼簿（OSC 52，需終端機支援）。

寫文也可以走 markdown 路：在發文畫面按 `Ctrl+I`，貼上含 frontmatter 的 markdown，
標題會自動從 `title:` 帶進來。

# 這篇文是怎麼長出來的

這篇 welcome 不是寫死在程式碼或 DB migration 裡。它是 `internal/seed/articles/welcome.md`，
用 `go:embed` 包進 binary，server 第一次發現 `Welcome` 看板是空的就把它寫進 DB。
之後 admin 改過一次，下次重啟也不會被覆蓋（種子的冪等規則：看板有任何文就跳過）。

要新增種子文章，丟一個新的 `*.md` 進那個資料夾，frontmatter 寫 `board: 看板名` 就好。

# 想了解更多

- 專案 README：repo 根目錄 `README.md`
- pttbbs 對照筆記：`docs/ptt_trace_code/*`
- 測試策略：`docs/testing.md`
- 待辦清單：`TODO.md`（含 P1～P3 + 工作量估計）
