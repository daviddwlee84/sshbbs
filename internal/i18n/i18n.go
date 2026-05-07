// Package i18n is the BBS's tiny translation layer. Two locales for now —
// zh-TW (default, canonical) and en — with strings declared as Go map
// literals per locale (see zh_tw.go, en.go). The English table is allowed
// to be partial; missing keys fall back to the zh-TW table so a half-done
// translation never empties the UI.
//
// Glyph-level locale switches (push kind 推/噓/→ → 👍/👎/→, score 爆 → 💥,
// comments-mode badges [鎖]/[箭] → [L]/[A]) live in glyphs.go and are kept
// out of the generic key/value table because they're consumed by the TUI
// render path and have a width-parity invariant the tests assert.
//
// Hard rule: callers in internal/markdown and the markdown export pipeline
// MUST NOT consult this package. The export format is canonical zh-TW so
// the parser at internal/markdown/parsePushLine round-trips. See
// internal/store/pushes.go's Glyph() doc for the invariant.
package i18n

import "fmt"

// Locale is the BCP-47-ish tag persisted in users.locale. Keep the set
// small and explicit so a typo in the DB or a stale row doesn't silently
// switch a user to an unrecognised language — Normalize maps unknowns
// back to Default.
type Locale string

const (
	LocaleZHTW Locale = "zh-TW"
	LocaleEN   Locale = "en"
)

// Default is the fallback locale used when:
//   - a User is nil (register / pre-login screens)
//   - users.locale stores an unrecognised value (forward compatibility)
//   - a key is missing in another locale's table (fallback chain)
//
// It defaults to zh-TW (the project's canonical locale) but operators
// can flip it to en at server startup via SetDefault — typically driven
// by the -locale flag or BBS_LOCALE env var in cmd/sshbbs/main.go. Set
// once before any sessions accept traffic; treated as read-only after
// that, so no mutex is needed.
var Default Locale = LocaleZHTW

// SetDefault overrides Default if loc is a recognised locale; ignores
// unknowns (rather than panicking) so a typo in BBS_LOCALE on a
// production deploy doesn't crash startup.
func SetDefault(loc Locale) {
	if Valid(string(loc)) {
		Default = loc
	}
}

// Valid reports whether s names a recognised locale. The locale-settings
// screen calls this before persisting; everywhere else uses Normalize.
func Valid(s string) bool {
	switch Locale(s) {
	case LocaleZHTW, LocaleEN:
		return true
	}
	return false
}

// Normalize coerces a raw string (typically users.locale or the empty
// string from a nil User on the register screen) to a known locale.
// Unrecognised values become Default rather than erroring — a stale row
// from a future "this locale was removed" migration shouldn't break the UI.
func Normalize(s string) Locale {
	if Valid(s) {
		return Locale(s)
	}
	return Default
}

// tableFor returns the string table for loc, or nil if loc isn't one we
// know about. The lookup goes through this rather than a public map so
// callers can't mutate the tables at runtime.
func tableFor(loc Locale) map[string]string {
	switch loc {
	case LocaleZHTW:
		return zhTW
	case LocaleEN:
		return en
	}
	return nil
}

// T looks up key in loc's table, falling back to Default's table when the
// key is missing or empty in loc. If both tables miss, the key is returned
// wrapped in «» so an unlocalised render is grep-able in the running UI
// (instead of crashing or rendering an empty string).
func T(loc Locale, key string) string {
	if tbl := tableFor(loc); tbl != nil {
		if v, ok := tbl[key]; ok && v != "" {
			return v
		}
	}
	if loc != Default {
		if tbl := tableFor(Default); tbl != nil {
			if v, ok := tbl[key]; ok && v != "" {
				return v
			}
		}
	}
	return "«" + key + "»"
}

// Tf is fmt.Sprintf over T. Format directives (%s, %d, etc.) live inside
// the locale string so word order can flow naturally per language —
// ("%s 推了你的文章" vs "%s 👍 your post"). i18n_test.go asserts both
// locales declare the same directive count so a translator dropping a %s
// is caught at test time, not when a notification fires.
func Tf(loc Locale, key string, args ...any) string {
	return fmt.Sprintf(T(loc, key), args...)
}
