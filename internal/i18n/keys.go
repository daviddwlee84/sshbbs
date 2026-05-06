package i18n

// Translation keys. Naming convention: dotted, screen-grouped, lower-case.
// Top namespaces: common.* (cross-screen primitives), screen.<name>.*
// (per-screen labels), error.* (error toasts), notify.* (webhook titles),
// help.<screen>.* (help-overlay rows).
//
// PR 1 declares only the keys the locale-settings screen needs, plus the
// new "Locale" entry on the user-settings sub-menu. PR 2 expands this
// list as it converts the rest of the screens; until then existing
// hard-coded CJK strings stay where they are.
const (
	CommonBack = "common.back"

	ScreenUserSettingsLocale     = "screen.user_settings.locale"
	ScreenUserSettingsLocaleHint = "screen.user_settings.locale_hint"

	ScreenLocaleSettingsTitle      = "screen.locale_settings.title"
	ScreenLocaleSettingsIntro      = "screen.locale_settings.intro"
	ScreenLocaleSettingsOptionZH   = "screen.locale_settings.option_zh_label"
	ScreenLocaleSettingsOptionEN   = "screen.locale_settings.option_en_label"
	ScreenLocaleSettingsNoteGlyphs = "screen.locale_settings.note_glyphs"
	ScreenLocaleSettingsDirty      = "screen.locale_settings.dirty_marker"
	ScreenLocaleSettingsFlashSaved = "screen.locale_settings.flash_saved"
	ScreenLocaleSettingsHelpLine   = "screen.locale_settings.help_line"
	ScreenLocaleSettingsHeaderHelp = "screen.locale_settings.header_help"
)
