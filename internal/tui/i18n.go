package tui

import "github.com/daviddwlee84/sshbbs/internal/i18n"

// localeOf returns the active UI locale for a Deps. Centralises the
// nil-User guard so screens that may render before login (register,
// password-change-on-first-login) don't have to spell it out — they
// just call localeOf(m.deps) and get Default.
//
// Also runs the value through i18n.Normalize so a stale or junk
// users.locale row falls back gracefully instead of mis-rendering as
// the «key» sentinel everywhere.
func localeOf(d Deps) i18n.Locale {
	if d.User == nil {
		return i18n.Default
	}
	return i18n.Normalize(d.User.Locale)
}
