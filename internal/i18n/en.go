package i18n

// en is the English translation table. Keys missing or empty here fall
// back to zhTW via T(). Glyph-level locale switches (推/噓/爆/[鎖]/[箭])
// are NOT in this table — they live in glyphs.go and are exposed via
// PushGlyph / ScoreExploded / CommentsModeBadge so the round-trip-safe
// canonical zh-TW glyph stays in store.PushKind.Glyph().
var en = map[string]string{
	CommonBack:    "Back",
	CommonLoading: "loading…",

	// Main menu
	ScreenMainMenuTitle:         " SSH-BBS · %s (%s) ",
	ScreenMainMenuLastLoginLine: "last login %s · %d logins · %d posts",
	ScreenMainMenuMidHeader:     " Main Menu ",
	ScreenMainMenuItemBoards:    "Boards",
	ScreenMainMenuItemWB:        "Water Balloons",
	ScreenMainMenuItemOnline:    "Online users",
	ScreenMainMenuItemMail:      "Mail",
	ScreenMainMenuItemSettings:  "User settings",
	ScreenMainMenuItemAdmin:     "Admin",
	ScreenMainMenuItemQuit:      "Quit",
	ScreenMainMenuHintBoards:    "browse and read articles",
	ScreenMainMenuHintWB:        "private messages with online users",
	ScreenMainMenuHintOnline:    "see who's logged in",
	ScreenMainMenuHintMail:      "persistent threaded mail",
	ScreenMainMenuHintSettings:  "password / bio / notifications / language",
	ScreenMainMenuHintAdmin:     "manage user roles",
	ScreenMainMenuHintQuit:      "disconnect",
	ScreenMainMenuHelpLine:      "↑/↓ j/k move · Enter/→/l choose · 1-%d jump · ? help · q quit",

	// User settings
	ScreenUserSettingsTitle:        " User Settings ",
	ScreenUserSettingsAccountLine:  "user %s · role %s",
	ScreenUserSettingsNoBio:        "(no bio yet)",
	ScreenUserSettingsItemPassword: "Change password",
	ScreenUserSettingsItemBio:      "Edit bio",
	ScreenUserSettingsItemNotify:   "Notification settings",
	ScreenUserSettingsItemBack:     "Back",
	ScreenUserSettingsHintPassword: "current → new → confirm",
	ScreenUserSettingsHintBio:      "free-form profile blurb",
	ScreenUserSettingsHintNotify:   "webhook targets + per-event toggles",
	ScreenUserSettingsHintBack:     "main menu",
	ScreenUserSettingsLocale:       "Language",
	ScreenUserSettingsLocaleHint:   "interface language (zh-TW / en)",
	ScreenUserSettingsHelpLine:     "↑/↓ j/k move · Enter/→/l choose · 1-5 jump · Esc/←/h back",

	// Locale settings
	ScreenLocaleSettingsTitle:      " Language ",
	ScreenLocaleSettingsIntro:      "Choose interface language:",
	ScreenLocaleSettingsOptionZH:   "Traditional Chinese (zh-TW)",
	ScreenLocaleSettingsOptionEN:   "English (en)",
	ScreenLocaleSettingsNoteGlyphs: "Note: English mode renders 推/噓/爆 as 👍/👎/💥 (same display width — no layout shift).",
	ScreenLocaleSettingsDirty:      "(unsaved)",
	ScreenLocaleSettingsFlashSaved: "✓ Locale saved",
	ScreenLocaleSettingsHelpLine:   "↑/↓ j/k move · Enter/Space select · Ctrl+S save · Esc cancel",
	ScreenLocaleSettingsHeaderHelp: "Locale settings",

	// Online
	ScreenOnlineTitle:         " Online (%d) ",
	ScreenOnlineEmpty:         "(nobody else online)",
	ScreenOnlineEmptyHelpLine: "Esc/←/h back · Q quit to menu",
	ScreenOnlineHelpLine:      "↑/↓ j/k move · Enter/→/l whisper · t chat thread · Esc/←/h back · Q quit",

	// Board list
	ScreenBoardListTitle:             " Boards ",
	ScreenBoardListSearchPlaceholder: "search boards",
	ScreenBoardListSearchPrompt:      "search / : ",
	ScreenBoardListSearchInProgress:  "(%d match · Enter apply · Esc cancel)",
	ScreenBoardListSearchActive:      "[search: %s · %d match · / edit · Esc clear]",
	ScreenBoardListNoMatch:           "(no matching boards)",
	ScreenBoardListNoBoards:          "(no boards yet)",
	ScreenBoardListLoadFailed:        "⚠ load failed: %s",
	ScreenBoardListHelpLine:          "↑/↓ j/k move · Enter/→/l open · / search · ? help · Esc/←/h back · Ctrl+C disconnect",

	// Board view
	ScreenBoardViewTitleNamed:        " Board %s · %s ",
	ScreenBoardViewTitleBare:         " Board ",
	ScreenBoardViewSearchPlaceholder: "search title",
	ScreenBoardViewSearchPrompt:      "search / title: ",
	ScreenBoardViewSearchInProgress:  "(Enter apply · Esc cancel)",
	ScreenBoardViewSearchActive:      "[search: %s · %d match · / edit · Esc clear]",
	ScreenBoardViewNoArticles:        "(no matching articles)",
	ScreenBoardViewSortByScore:       "[sort: by score↓]",
	ScreenBoardViewHelpLine:          "↑/↓ j/k move · Enter/→/l open · / search · s sort · p post · ? help · Esc/←/h back",

	// Article view
	ScreenArticleViewTitle:              " Article #%d · %s ",
	ScreenArticleViewCommentsPrefix:     "comments:  ",
	ScreenArticleViewCommentsArrowsOnly: "[A] arrows-only",
	ScreenArticleViewCommentsLocked:     "[L] locked",
	ScreenArticleViewPushesHeader:       "── Reactions (%d) ──",
	ScreenArticleViewConfirmDeleteArt:   "Delete this article? (y/N)",
	ScreenArticleViewConfirmDeletePush: "Delete reaction #%d? (y/N)",
	ScreenArticleViewModeBanner:         " Comments mode ",
	ScreenArticleViewModeOptions:        "1 open  2 arrows-only  3 locked  Esc cancel",
	ScreenArticleViewHelpBase:           "j/k scroll · y export",
	ScreenArticleViewHelpPushKinds:      " · + 👍 · - 👎 · = → · r reply",
	ScreenArticleViewHelpSelectPush:     " · p/P pick reaction",
	ScreenArticleViewHelpEdit:           " · E edit",
	ScreenArticleViewHelpModeToggle:     " · M comments-mode",
	ScreenArticleViewHelpDeletePush:     " · D delete reaction",
	ScreenArticleViewHelpDeleteArt:      " · D delete article",
	ScreenArticleViewErrPushBodyEmpty:   "comment required for 👍/👎",

	// Post compose / reply
	ScreenPostComposeTitleNew:    " New Post ",
	ScreenPostComposeTitleReply:  " Reply ",
	ScreenPostComposeReplyPrefix: "Re: #%d  %s · %s",
	ScreenPostComposeWroteSuffix: "wrote",
	ScreenPostComposeTitlePh:     "title",
	ScreenPostComposeBodyPh:      "body…",
	ScreenPostComposeFMHintMissing: "no frontmatter found (paste markdown starting with --- ... --- block)",
	ScreenPostComposeFMHintPaste:   "paste markdown (with --- frontmatter ---) then press Ctrl+I",

	ScreenArticleEditTitle:   " Edit Post ",
	ScreenArticleEditTitlePh: "title",
	ScreenArticleEditBodyPh:  "body…",

	// Article export
	ScreenArticleExportTitle:          " Export markdown ",
	ScreenArticleExportGuestNoWrite:   "guest accounts cannot write files (read-only)",
	ScreenArticleExportWroteFile:      "wrote %s",
	ScreenArticleExportOSC52Truncated: "⚠ %dB exceeds OSC52 limit; may be truncated by the terminal — press 3 to write to disk instead",
	ScreenArticleExportOSC52Copied:    "copied to clipboard (requires terminal OSC 52 support)",
	ScreenArticleExportHelpLine:       "1 plain · 2 with reactions · 3 write file · c clipboard (OSC52) · j/k scroll · Esc back",

	// Register / Password / Bio / Banner / Splash
	ScreenRegisterTitle:         "  === Register new account ===",
	ScreenRegisterSuccess:       "\n  ✓ Registered successfully!",
	ScreenRegisterRetryHint:     "\n\n  Reconnect with:\n\n    ",
	ScreenRegisterDoneHint:      "\n\n  Press any key to disconnect.\n",
	ScreenRegisterFieldUserID:   "user id (3-12, letter-leading, alnum + underscore)",
	ScreenRegisterFieldPassword: "password (≥ 6)",
	ScreenRegisterFieldNickname: "nickname",
	ScreenRegisterNicknamePh:    "Alice",

	ScreenPasswordChangeTitle:        "  === Change password ===",
	ScreenPasswordChangeTitleMust:    "  === Change password (required on first login) ===",
	ScreenPasswordChangePhCurrent:    "current password",
	ScreenPasswordChangePhNew:        "new password (≥ 6)",
	ScreenPasswordChangePhConfirm:    "new password (again)",
	ScreenPasswordChangeLabelCurrent: "current",
	ScreenPasswordChangeLabelNew:     "new (≥ 6)",
	ScreenPasswordChangeLabelConfirm: "confirm",
	ScreenPasswordChangeErrWrong:     "current password is wrong",
	ScreenPasswordChangeErrMismatch:  "new password and confirmation do not match",
	ScreenPasswordChangeErrSame:      "new password must differ from the current one",
	ScreenPasswordChangeOKToMenu:     "✓ updated — entering main menu…",
	ScreenPasswordChangeOK:           "✓ updated",

	ScreenBioEditTitle:       " Edit bio ",
	ScreenBioEditPlaceholder: "your bio… (≤ 1024 chars; newlines OK)",

	ScreenBoardBannerEditTitle:    " Edit board banner ",
	ScreenBoardBannerEditPh:       "paste ANSI / ASCII art… (empty body clears the banner)",
	ScreenBoardBannerEditHelpLine: "Ctrl+S save · Esc cancel · empty body clears the banner",

	ScreenBoardSplashTitleNamed: " Banner · %s ",
	ScreenBoardSplashTitleBare:  " Banner ",

	// Mail / WB
	ScreenMailInboxTitle:        " Mail ",
	ScreenMailThreadTitle:       " Thread #%d ",
	ScreenMailComposeTitleNew:   " New Mail ",
	ScreenMailComposeTitleReply: " Reply ",
	ScreenMailWroteSuffix:       "wrote",

	ScreenWBInboxTitle:   " Water Balloons ",
	ScreenWBComposeTitle: " Send Water Balloon ",
	ScreenWBThreadTitle:  " Chat with %s ",

	// Notify settings
	ScreenNotifyTitle:           " Notification settings ",
	ScreenNotifyEventsHeader:    "Events",
	ScreenNotifyTargetsHeader:   "Webhook targets",
	ScreenNotifyNoTargets:       "  (no targets yet — press a to add)",
	ScreenNotifyAddTarget:       "  + Add new target",
	ScreenNotifyLabelOnPush:     "👍/👎 on my article (push)",
	ScreenNotifyLabelOnWB:       "received water balloon (wb)",
	ScreenNotifyLabelOnMail:     "received mail (mail)",
	ScreenNotifyLabelOnReply:    "someone replied Re: (reply)",
	ScreenNotifyLabelOffline:    "only when offline",
	ScreenNotifyHintOnPush:      "someone reacted to my article",
	ScreenNotifyHintOnWB:        "private message",
	ScreenNotifyHintOnMail:      "persistent threaded mail",
	ScreenNotifyHintOnReply:     "someone posted a Re: reply on my article",
	ScreenNotifyHintOffline:     "skip dispatch when I have a live session",
	ScreenNotifyEditLabelPh:     "label (any easy-to-recognise name)",
	ScreenNotifyEditURLPh:       "Discord webhook URL / https://ntfy.sh/<topic> / any HTTP(S) endpoint",
	ScreenNotifyFlashPrefsSaved: "✓ event preferences saved",
	ScreenNotifyFlashEnabled:    "✓ enabled target #%d",
	ScreenNotifyFlashDisabled:   "✓ disabled target #%d",
	ScreenNotifyFlashDeleted:    "✓ deleted target",
	ScreenNotifyFlashAdded:      "✓ added target #%d",
	ScreenNotifyFlashUpdated:    "✓ updated target #%d",
	ScreenNotifyHelpLine:        "j/k move · Space toggle/select · Ctrl+S save prefs · a add · e edit · t enable/disable · T test · d delete · Esc back",

	// Admin / users
	ScreenAdminUsersTitle:        " Admin Users ",
	ScreenAdminUsersToastAlready: "%s is already %s",
	ScreenAdminUsersToastNoLast:  "cannot demote the last admin",

	// Help overlay
	HelpOverlayTitle:  " Keyboard shortcuts ",
	HelpGlobalWBInbox: "open Water Balloon inbox (logged-in only)",

	// Errors
	ErrorGuestReadOnly:      "guest accounts are read-only",
	ErrorAdminOnly:          "admin only",
	ErrorPermDenied:         "permission denied",
	ErrorCommentsLocked:     "comments are locked on this article",
	ErrorCommentsArrowsOnly: "this article only allows arrow comments",

	// Notify webhook titles
	NotifyPushTitle:  "[BBS] %s %s your post",
	NotifyWBTitle:    "[BBS] %s sent you a water balloon",
	NotifyMailTitle:  "[BBS] %s mailed you: %s",
	NotifyReplyTitle: "[BBS] %s replied to your post",
	NotifyReplyBody:  "%s\n\nIn reply to: %s",
}
