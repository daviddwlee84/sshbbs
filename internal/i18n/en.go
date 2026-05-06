package i18n

// en is the English translation table. Keys missing or empty here fall
// back to zhTW via T(); a partial table is therefore valid and is the
// expected state during the PR 1 → PR 2 → PR 3 phasing (PR 1 ships en
// nearly empty so non-glyph UI still renders zh-TW; PR 2 fills the
// rest). The corresponding glyph-level swaps (推/噓/爆/[鎖]/[箭]) live in
// glyphs.go and don't go through this table.
var en = map[string]string{
	CommonBack: "Back",

	ScreenUserSettingsLocale:     "Language",
	ScreenUserSettingsLocaleHint: "interface language (zh-TW / en)",

	ScreenLocaleSettingsTitle:      " Language ",
	ScreenLocaleSettingsIntro:      "Choose interface language:",
	ScreenLocaleSettingsOptionZH:   "Traditional Chinese (zh-TW)",
	ScreenLocaleSettingsOptionEN:   "English (en)",
	ScreenLocaleSettingsNoteGlyphs: "Note: English mode renders 推/噓/爆 as 👍/👎/💥 (same display width — no layout shift).",
	ScreenLocaleSettingsDirty:      "(unsaved)",
	ScreenLocaleSettingsFlashSaved: "✓ Locale saved",
	ScreenLocaleSettingsHelpLine:   "↑/↓ j/k move · Enter/Space select · Ctrl+S save · Esc cancel",
	ScreenLocaleSettingsHeaderHelp: "Locale settings",
}
