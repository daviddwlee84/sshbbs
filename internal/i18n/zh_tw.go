package i18n

// zhTW is the canonical translation table. Every key declared in keys.go
// MUST have a non-empty entry here — i18n_test.go enforces this so the
// fallback path in T() never has to render «key».
var zhTW = map[string]string{
	CommonBack: "返回 Back",

	ScreenUserSettingsLocale:     "語言 Language",
	ScreenUserSettingsLocaleHint: "interface language (zh-TW / en)",

	ScreenLocaleSettingsTitle:      " 語言 Language ",
	ScreenLocaleSettingsIntro:      "選擇介面語言：",
	ScreenLocaleSettingsOptionZH:   "繁體中文 (zh-TW)",
	ScreenLocaleSettingsOptionEN:   "English (en)",
	ScreenLocaleSettingsNoteGlyphs: "註：英文模式會把推/噓/爆顯示為 👍/👎/💥 (寬度相同，不會跑版)",
	ScreenLocaleSettingsDirty:      "(尚未儲存)",
	ScreenLocaleSettingsFlashSaved: "✓ 已儲存語言偏好",
	ScreenLocaleSettingsHelpLine:   "↑/↓ j/k move · Enter/Space select · Ctrl+S save · Esc cancel",
	ScreenLocaleSettingsHeaderHelp: "Locale settings",
}
