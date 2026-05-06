package i18n

// zhTW is the canonical translation table. Every key declared in keys.go
// MUST have a non-empty entry here — i18n_test.go enforces this so the
// fallback path in T() never has to render «key».
var zhTW = map[string]string{
	CommonBack:    "返回 Back",
	CommonLoading: "載入中…",

	// Main menu
	ScreenMainMenuTitle:         " SSH-BBS · %s (%s) ",
	ScreenMainMenuLastLoginLine: "上次登入 %s · 累計登入 %d 次 · 發文 %d 篇",
	ScreenMainMenuMidHeader:     " 主選單 Main Menu ",
	ScreenMainMenuItemBoards:    "看板列表 Boards",
	ScreenMainMenuItemWB:        "水球 Water Balloons",
	ScreenMainMenuItemOnline:    "線上使用者 Online",
	ScreenMainMenuItemMail:      "信箱 Mail",
	ScreenMainMenuItemSettings:  "個人設定 User settings",
	ScreenMainMenuItemAdmin:     "管理 Admin",
	ScreenMainMenuItemQuit:      "離線 Quit",
	ScreenMainMenuHintBoards:    "瀏覽看板與文章",
	ScreenMainMenuHintWB:        "與線上使用者私訊",
	ScreenMainMenuHintOnline:    "查看誰在線上",
	ScreenMainMenuHintMail:      "持久化信件 / 對話串",
	ScreenMainMenuHintSettings:  "密碼 / bio / 通知 / 語言",
	ScreenMainMenuHintAdmin:     "管理使用者角色",
	ScreenMainMenuHintQuit:      "中斷 SSH 連線",
	ScreenMainMenuHelpLine:      "↑/↓ j/k move · Enter/→/l 選擇 · 1-%d 直跳 · ? 說明 · q 離線",

	// User settings
	ScreenUserSettingsTitle:        " 個人設定 User Settings ",
	ScreenUserSettingsAccountLine:  "帳號 %s · 角色 %s",
	ScreenUserSettingsNoBio:        "(尚未填寫 bio)",
	ScreenUserSettingsItemPassword: "修改密碼 Change password",
	ScreenUserSettingsItemBio:      "修改 Bio Edit bio",
	ScreenUserSettingsItemNotify:   "通知設定 Notification settings",
	ScreenUserSettingsItemBack:     "返回 Back",
	ScreenUserSettingsHintPassword: "目前 → 新 → 確認",
	ScreenUserSettingsHintBio:      "自由格式的個人簡介",
	ScreenUserSettingsHintNotify:   "Webhook 目標與事件開關",
	ScreenUserSettingsHintBack:     "回到主選單",
	ScreenUserSettingsLocale:       "語言 Language",
	ScreenUserSettingsLocaleHint:   "介面語言 (zh-TW / en)",
	ScreenUserSettingsHelpLine:     "↑/↓ j/k move · Enter/→/l 選擇 · 1-5 直跳 · Esc/←/h 返回",

	// Locale settings
	ScreenLocaleSettingsTitle:      " 語言 Language ",
	ScreenLocaleSettingsIntro:      "選擇介面語言：",
	ScreenLocaleSettingsOptionZH:   "繁體中文 (zh-TW)",
	ScreenLocaleSettingsOptionEN:   "English (en)",
	ScreenLocaleSettingsNoteGlyphs: "註：英文模式會把推/噓/爆顯示為 👍/👎/💥 (寬度相同，不會跑版)",
	ScreenLocaleSettingsDirty:      "(尚未儲存)",
	ScreenLocaleSettingsFlashSaved: "✓ 已儲存語言偏好",
	ScreenLocaleSettingsHelpLine:   "↑/↓ j/k move · Enter/Space 選擇 · Ctrl+S 儲存 · Esc 取消",
	ScreenLocaleSettingsHeaderHelp: "Locale settings",

	// Online
	ScreenOnlineTitle:         " 線上使用者 Online (%d) ",
	ScreenOnlineEmpty:         "(目前沒有其他人在線)",
	ScreenOnlineEmptyHelpLine: "Esc/←/h 返回 · Q 回主選單",
	ScreenOnlineHelpLine:      "↑/↓ j/k move · Enter/→/l 丟水球 · t 對話 thread · Esc/←/h 返回 · Q 離線",

	// Board list
	ScreenBoardListTitle:             " 看板列表 Boards ",
	ScreenBoardListSearchPlaceholder: "搜尋看板 / search boards",
	ScreenBoardListSearchPrompt:      "搜尋 / : ",
	ScreenBoardListSearchInProgress:  "(%d 筆符合 · Enter 套用 · Esc 取消)",
	ScreenBoardListSearchActive:      "[搜尋: %s · %d 筆 · / 修改 · Esc 清除]",
	ScreenBoardListNoMatch:           "(沒有符合的看板)",
	ScreenBoardListNoBoards:          "(尚未建立看板)",
	ScreenBoardListLoadFailed:        "⚠ 載入失敗：%s",
	ScreenBoardListHelpLine:          "↑/↓ j/k move · Enter/→/l 開啟 · / 搜尋 · ? 說明 · Esc/←/h 返回 · Ctrl+C 中斷",

	// Board view
	ScreenBoardViewTitleNamed:        " 看板 %s · %s ",
	ScreenBoardViewTitleBare:         " 看板 ",
	ScreenBoardViewSearchPlaceholder: "搜尋標題 / search title",
	ScreenBoardViewSearchPrompt:      "搜尋 / 標題: ",
	ScreenBoardViewSearchInProgress:  "(Enter 套用 · Esc 取消)",
	ScreenBoardViewSearchActive:      "[搜尋: %s · %d 筆 · / 修改 · Esc 清除]",
	ScreenBoardViewNoArticles:        "(沒有符合的文章)",
	ScreenBoardViewSortByScore:       "[排序: 推文量↓]",
	ScreenBoardViewHelpLine:          "↑/↓ j/k move · Enter/→/l 開啟 · / 搜尋 · s 排序 · p 發文 · ? 說明 · Esc/←/h 返回",

	// Article view
	ScreenArticleViewTitle:              " 文章 #%d · %s ",
	ScreenArticleViewCommentsPrefix:     "留言:  ",
	ScreenArticleViewCommentsArrowsOnly: "[箭] 僅開放箭頭",
	ScreenArticleViewCommentsLocked:     "[鎖] 已關閉留言",
	ScreenArticleViewPushesHeader:       "── 推文 (%d) ──",
	ScreenArticleViewConfirmDeleteArt:   "確定刪除這篇文章? (y/N)",
	ScreenArticleViewConfirmDeletePush:  "確定刪除推文 #%d? (y/N)",
	ScreenArticleViewModeBanner:         " 留言模式 ",
	ScreenArticleViewModeOptions:        "1 開放  2 僅箭頭  3 鎖文  Esc 取消",
	ScreenArticleViewHelpBase:           "j/k 卷動 · y 匯出",
	ScreenArticleViewHelpPushKinds:      " · + 推 · - 噓 · = → · r 回文",
	ScreenArticleViewHelpSelectPush:     " · p/P 選推文",
	ScreenArticleViewHelpEdit:           " · E 編輯",
	ScreenArticleViewHelpModeToggle:     " · M 留言模式",
	ScreenArticleViewHelpDeletePush:     " · D 刪除推文",
	ScreenArticleViewHelpDeleteArt:      " · D 刪除文章",
	ScreenArticleViewErrPushBodyEmpty:   "推/噓 必須附上推文內容",

	// Post compose / reply
	ScreenPostComposeTitleNew:    " 發表新文章 New Post ",
	ScreenPostComposeTitleReply:  " 回文 Reply ",
	ScreenPostComposeReplyPrefix: "回覆 #%d  %s · %s",
	ScreenPostComposeWroteSuffix: "寫道",
	ScreenPostComposeTitlePh:     "標題 title",
	ScreenPostComposeBodyPh:      "內容 body…",
	ScreenPostComposeFMHintMissing: "找不到 frontmatter（請貼上開頭含 --- ... --- 的 markdown）",
	ScreenPostComposeFMHintPaste:   "貼上 markdown（含 --- frontmatter ---）後再按 Ctrl+I",

	ScreenArticleEditTitle:   " 編輯文章 Edit Post ",
	ScreenArticleEditTitlePh: "標題 title",
	ScreenArticleEditBodyPh:  "內容 body…",

	// Article export
	ScreenArticleExportTitle:          " 匯出 markdown · Export ",
	ScreenArticleExportGuestNoWrite:   "guest 不能寫檔 (read-only)",
	ScreenArticleExportWroteFile:      "已寫入 %s",
	ScreenArticleExportOSC52Truncated: "⚠ %dB 超過 OSC52 上限，可能被終端機截斷；改按 3 寫檔",
	ScreenArticleExportOSC52Copied:    "已複製到剪貼簿（需終端機支援 OSC 52）",
	ScreenArticleExportHelpLine:       "1 純內文 · 2 含留言 · 3 寫檔 · c 剪貼簿 (OSC52) · j/k 卷動 · Esc 返回",

	// Register / Password / Bio / Banner / Splash
	ScreenRegisterTitle:         "  === 註冊新帳號 ===",
	ScreenRegisterSuccess:       "\n  ✓ 註冊成功！",
	ScreenRegisterRetryHint:     "\n\n  請重新連線：\n\n    ",
	ScreenRegisterDoneHint:      "\n\n  按任意鍵結束。\n",
	ScreenRegisterFieldUserID:   "帳號 user id (3-12, 字母開頭, 英數+底線)",
	ScreenRegisterFieldPassword: "密碼 password (≥ 6)",
	ScreenRegisterFieldNickname: "暱稱 nickname",
	ScreenRegisterNicknamePh:    "愛麗絲",

	ScreenPasswordChangeTitle:        "  === 修改密碼 Change password ===",
	ScreenPasswordChangeTitleMust:    "  === 修改密碼 (首次登入必須修改) ===",
	ScreenPasswordChangePhCurrent:    "目前密碼",
	ScreenPasswordChangePhNew:        "新密碼 (≥ 6)",
	ScreenPasswordChangePhConfirm:    "再次輸入新密碼",
	ScreenPasswordChangeLabelCurrent: "目前密碼 current",
	ScreenPasswordChangeLabelNew:     "新密碼 new (≥ 6)",
	ScreenPasswordChangeLabelConfirm: "再次輸入 confirm",
	ScreenPasswordChangeErrWrong:     "目前密碼錯誤",
	ScreenPasswordChangeErrMismatch:  "新密碼與確認不一致",
	ScreenPasswordChangeErrSame:      "新密碼不可與目前密碼相同",
	ScreenPasswordChangeOKToMenu:     "✓ 已更新，將進入主選單…",
	ScreenPasswordChangeOK:           "✓ 已更新",

	ScreenBioEditTitle:       " 修改 Bio Edit bio ",
	ScreenBioEditPlaceholder: "你的 bio…（最多 1024 字元，可換行）",

	ScreenBoardBannerEditTitle:    " 編輯看板 banner · Edit Banner ",
	ScreenBoardBannerEditPh:       "貼上 ANSI / ASCII art… (空字串會清掉 banner)",
	ScreenBoardBannerEditHelpLine: "Ctrl+S 儲存 · Esc 取消 · 空 body 等於清掉 banner",

	ScreenBoardSplashTitleNamed: " 看板 banner · %s ",
	ScreenBoardSplashTitleBare:  " 看板 banner ",

	// Mail / WB
	ScreenMailInboxTitle:        " 信箱 Mail ",
	ScreenMailThreadTitle:       " 信件 Thread #%d ",
	ScreenMailComposeTitleNew:   " 寫信 New Mail ",
	ScreenMailComposeTitleReply: " 回信 Reply ",
	ScreenMailWroteSuffix:       "寫道",

	ScreenWBInboxTitle:   " 水球 Water Balloons ",
	ScreenWBComposeTitle: " 丟水球 Send Water Balloon ",
	ScreenWBThreadTitle:  " 對話 with %s ",

	// Notify settings
	ScreenNotifyTitle:           " 通知設定 Notification settings ",
	ScreenNotifyEventsHeader:    "事件 Events",
	ScreenNotifyTargetsHeader:   "通知目標 Webhook targets",
	ScreenNotifyNoTargets:       "  (尚未設定 target — 按 a 新增)",
	ScreenNotifyAddTarget:       "  + 新增 target Add new target",
	ScreenNotifyLabelOnPush:     "推/噓 我的文章 (push)",
	ScreenNotifyLabelOnWB:       "收到水球 (wb)",
	ScreenNotifyLabelOnMail:     "收到站內信 (mail)",
	ScreenNotifyLabelOnReply:    "有人回文 Re: (reply)",
	ScreenNotifyLabelOffline:    "僅在離線時通知 only-when-offline",
	ScreenNotifyHintOnPush:      "有人推/噓我的文章",
	ScreenNotifyHintOnWB:        "私訊 DM",
	ScreenNotifyHintOnMail:      "持久化信件",
	ScreenNotifyHintOnReply:     "有人回了我的文章",
	ScreenNotifyHintOffline:     "有 live session 時不送出",
	ScreenNotifyEditLabelPh:     "label (任意便於辨識的名稱)",
	ScreenNotifyEditURLPh:       "Discord webhook URL / https://ntfy.sh/<topic> / 任何 HTTP(S) 端點",
	ScreenNotifyFlashPrefsSaved: "✓ 已儲存事件偏好",
	ScreenNotifyFlashEnabled:    "✓ 已啟用 target #%d",
	ScreenNotifyFlashDisabled:   "✓ 已停用 target #%d",
	ScreenNotifyFlashDeleted:    "✓ 已刪除 target",
	ScreenNotifyFlashAdded:      "✓ 已新增 target #%d",
	ScreenNotifyFlashUpdated:    "✓ 已更新 target #%d",
	ScreenNotifyHelpLine:        "j/k move · Space toggle/select · Ctrl+S 儲存 · a 新增 · e 編輯 · t 啟停 · T 測試 · d 刪除 · Esc 返回",

	// Admin / users
	ScreenAdminUsersTitle:        " 使用者管理 Admin Users ",
	ScreenAdminUsersToastAlready: "%s 已是 %s",
	ScreenAdminUsersToastNoLast:  "無法解除最後一名管理員 (cannot demote the last admin)",

	// Help overlay
	HelpOverlayTitle:  " 鍵盤捷徑說明 Help ",
	HelpGlobalWBInbox: "open 水球 inbox (logged-in only)",

	// Errors
	ErrorGuestReadOnly:      "guest 為唯讀帳號 (read-only)",
	ErrorAdminOnly:          "管理員專用 (admin only)",
	ErrorPermDenied:         "權限不足",
	ErrorCommentsLocked:     "本文已鎖定留言",
	ErrorCommentsArrowsOnly: "本文僅開放箭頭留言",

	// Notify webhook titles
	NotifyPushTitle:  "[BBS] %s %s 了你的文章",
	NotifyWBTitle:    "[BBS] %s 丟了水球給你",
	NotifyMailTitle:  "[BBS] %s 寄信給你: %s",
	NotifyReplyTitle: "[BBS] %s 回了你的文章",
	NotifyReplyBody:  "%s\n\n原文: %s",
}
