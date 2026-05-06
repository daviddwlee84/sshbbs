package tui

import (
	"fmt"

	"github.com/daviddwlee84/sshbbs/internal/notify"
)

// notifyWB fires a KindWB event for a delivered water balloon. Skipped when
// the dispatcher is nil (tests / register-only sessions). Self-WB is
// expected to be filtered at the call site (see CLAUDE.md: self-WB is a
// memo and intentionally bypasses Broker.Send).
func notifyWB(deps Deps, toUserID int64, fromUserID, body string) {
	if deps.Notify == nil {
		return
	}
	deps.Notify.Dispatch(notify.Event{
		Kind:       notify.KindWB,
		ToUserID:   toUserID,
		FromUserID: fromUserID,
		Title:      fmt.Sprintf("[BBS] %s 丟了水球給你", fromUserID),
		Body:       body,
	})
}

// notifyMail fires a KindMail event for delivered mail. Self-mail is
// expected to be filtered at the call site.
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
	deps.Notify.Dispatch(notify.Event{
		Kind:       notify.KindMail,
		ToUserID:   toUserID,
		FromUserID: fromUserID,
		Title:      fmt.Sprintf("[BBS] %s 寄信給你: %s", fromUserID, subject),
		Body:       excerpt,
	})
}
