package tui

import (
	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/notify"
)

// notifyWB fires a KindWB event for a delivered water balloon. Skipped when
// the dispatcher is nil (tests / register-only sessions). Self-WB is
// expected to be filtered at the call site (see CLAUDE.md: self-WB is a
// memo and intentionally bypasses Broker.Send).
//
// Title renders in the RECIPIENT's locale — they read the toast on their
// phone. recipientLocale falls back to i18n.Default on any DB error.
func notifyWB(deps Deps, toUserID int64, fromUserID, body string) {
	if deps.Notify == nil {
		return
	}
	loc := recipientLocale(deps, toUserID)
	deps.Notify.Dispatch(notify.Event{
		Kind:       notify.KindWB,
		ToUserID:   toUserID,
		FromUserID: fromUserID,
		Title:      i18n.Tf(loc, i18n.NotifyWBTitle, fromUserID),
		Body:       body,
	})
}

// notifyMail fires a KindMail event for delivered mail. Self-mail is
// expected to be filtered at the call site. Title renders in the
// recipient's locale (see notifyWB).
func notifyMail(deps Deps, toUserID int64, fromUserID, subject, body string) {
	if deps.Notify == nil {
		return
	}
	// Cap the body excerpt — mail body can be 4 KB and most webhook
	// receivers truncate aggressively anyway.
	excerpt := body
	if len(excerpt) > 280 {
		excerpt = excerpt[:280] + "…"
	}
	loc := recipientLocale(deps, toUserID)
	deps.Notify.Dispatch(notify.Event{
		Kind:       notify.KindMail,
		ToUserID:   toUserID,
		FromUserID: fromUserID,
		Title:      i18n.Tf(loc, i18n.NotifyMailTitle, fromUserID, subject),
		Body:       excerpt,
	})
}
