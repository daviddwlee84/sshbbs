package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

// mailFixture builds a store with two users and returns a Deps for `bob`,
// the recipient. Optionally seeds N unread mails from alice → bob.
func mailFixture(t *testing.T, seed int) (Deps, *store.User) {
	t.Helper()
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	for i := 0; i < seed; i++ {
		_, err := st.Mail().Insert(context.Background(), alice.ID, alice.UserID, bob.ID, "subj", "body", nil)
		if err != nil {
			t.Fatalf("seed mail %d: %v", i, err)
		}
	}
	deps := Deps{Store: st, User: bob, Broker: chat.NewBroker()}
	return deps, alice
}

// =====================================================================
// Inbox
// =====================================================================

func TestMailInbox_LoadsItems(t *testing.T) {
	deps, _ := mailFixture(t, 3)
	m := newMailInboxModel(deps)
	if m.loadErr != nil {
		t.Fatalf("loadErr = %v", m.loadErr)
	}
	if len(m.items) != 3 {
		t.Errorf("got %d, want 3", len(m.items))
	}
}

func TestMailInbox_BackKeys(t *testing.T) {
	for _, key := range []string{"esc", "backspace", "left", "h", "Q"} {
		t.Run(key, func(t *testing.T) {
			deps, _ := mailFixture(t, 1)
			m := newMailInboxModel(deps)
			_, cmd := m.Update(keyOf(key))
			nav := runCmd(cmd).(NavigateMsg)
			if nav.To != ScreenMainMenu {
				t.Errorf("To = %v, want ScreenMainMenu", nav.To)
			}
		})
	}
}

func TestMailInbox_ComposeKey(t *testing.T) {
	deps, _ := mailFixture(t, 0)
	m := newMailInboxModel(deps)
	_, cmd := m.Update(keyOf("c"))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenMailCompose {
		t.Errorf("To = %v, want ScreenMailCompose", nav.To)
	}
	if nav.Recipient != "" || nav.MailID != 0 {
		t.Errorf("c should not pre-fill anything, got %+v", nav)
	}
}

// Enter / l opens the cursored message's thread.
func TestMailInbox_OpenThread(t *testing.T) {
	for _, key := range []string{"enter", " ", "right", "l"} {
		t.Run(key, func(t *testing.T) {
			deps, _ := mailFixture(t, 1)
			m := newMailInboxModel(deps)
			_, cmd := m.Update(keyOf(key))
			nav := runCmd(cmd).(NavigateMsg)
			if nav.To != ScreenMailThread {
				t.Errorf("To = %v, want ScreenMailThread", nav.To)
			}
			if nav.MailThreadID == 0 {
				t.Errorf("MailThreadID = 0, want non-zero")
			}
		})
	}
}

// `r` opens compose pre-filled with the highlighted sender + parent_id.
func TestMailInbox_ReplyKey(t *testing.T) {
	deps, alice := mailFixture(t, 1)
	m := newMailInboxModel(deps)
	_, cmd := m.Update(keyOf("r"))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenMailCompose {
		t.Errorf("To = %v, want ScreenMailCompose", nav.To)
	}
	if nav.Recipient != alice.UserID {
		t.Errorf("Recipient = %q, want %q", nav.Recipient, alice.UserID)
	}
	if nav.MailID == 0 {
		t.Errorf("MailID = 0, want parent id")
	}
}

// On an empty inbox, `r` and forward keys are no-ops.
func TestMailInbox_NoopsWhenEmpty(t *testing.T) {
	deps, _ := mailFixture(t, 0)
	m := newMailInboxModel(deps)
	for _, key := range []string{"r", "enter", "l"} {
		_, cmd := m.Update(keyOf(key))
		if runCmd(cmd) != nil {
			t.Errorf("%q on empty inbox should be no-op", key)
		}
	}
}

// =====================================================================
// Thread
// =====================================================================

func TestMailThread_OpeningMarksRead(t *testing.T) {
	deps, _ := mailFixture(t, 2)
	bob := deps.User
	// Pre-condition: 2 unread.
	n, _ := deps.Store.Mail().CountUnreadFor(context.Background(), bob.ID)
	if n != 2 {
		t.Fatalf("pre-condition failed: unread=%d", n)
	}
	// Open a thread (the constructor marks all in-thread items addressed
	// to the current user as read).
	items, _ := deps.Store.Mail().ListInboxFor(context.Background(), bob.ID, 10)
	first := items[0]
	_ = newMailThreadModel(deps, first.ThreadID)
	n, _ = deps.Store.Mail().CountUnreadFor(context.Background(), bob.ID)
	if n != 1 {
		t.Errorf("after opening one thread: unread=%d, want 1 (the other)", n)
	}
}

func TestMailThread_BackKeysToInbox(t *testing.T) {
	deps, _ := mailFixture(t, 1)
	items, _ := deps.Store.Mail().ListInboxFor(context.Background(), deps.User.ID, 10)
	m := newMailThreadModel(deps, items[0].ThreadID)
	for _, key := range []string{"esc", "backspace", "left", "h"} {
		t.Run(key, func(t *testing.T) {
			_, cmd := m.Update(keyOf(key))
			nav := runCmd(cmd).(NavigateMsg)
			if nav.To != ScreenMailInbox {
				t.Errorf("To = %v, want ScreenMailInbox", nav.To)
			}
		})
	}
}

func TestMailThread_QuitKeyToMenu(t *testing.T) {
	deps, _ := mailFixture(t, 1)
	items, _ := deps.Store.Mail().ListInboxFor(context.Background(), deps.User.ID, 10)
	m := newMailThreadModel(deps, items[0].ThreadID)
	_, cmd := m.Update(keyOf("Q"))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenMainMenu {
		t.Errorf("To = %v, want ScreenMainMenu", nav.To)
	}
}

// `r` from thread view opens compose targeting the original sender.
func TestMailThread_ReplyTargetsSender(t *testing.T) {
	deps, alice := mailFixture(t, 1)
	items, _ := deps.Store.Mail().ListInboxFor(context.Background(), deps.User.ID, 10)
	m := newMailThreadModel(deps, items[0].ThreadID)
	_, cmd := m.Update(keyOf("r"))
	nav := runCmd(cmd).(NavigateMsg)
	if nav.To != ScreenMailCompose {
		t.Errorf("To = %v, want ScreenMailCompose", nav.To)
	}
	if nav.Recipient != alice.UserID {
		t.Errorf("Recipient = %q, want %q", nav.Recipient, alice.UserID)
	}
	if nav.MailID == 0 {
		t.Errorf("MailID = 0, want last-mail id for parent")
	}
}

// =====================================================================
// Compose
// =====================================================================

func TestMailCompose_SubmitInsertsMail(t *testing.T) {
	deps, _ := mailFixture(t, 0)
	m := newMailComposeModel(deps, "alice", 0)
	m.subject.SetValue("hi")
	m.body.SetValue("body")

	model, _ := m.submit()
	got := model.(mailComposeModel)
	if got.err != "" {
		t.Fatalf("submit err = %q", got.err)
	}
	if got.sent != "alice" {
		t.Errorf("sent = %q, want alice", got.sent)
	}
	// alice should now have one mail in her inbox.
	alice, _ := deps.Store.Users().GetByUserID(context.Background(), "alice")
	got2, err := deps.Store.Mail().ListInboxFor(context.Background(), alice.ID, 10)
	if err != nil || len(got2) != 1 {
		t.Errorf("alice inbox: %d (err=%v), want 1", len(got2), err)
	}
}

// Submitting with parentID set creates a reply that inherits ThreadID.
func TestMailCompose_ReplySetsParent(t *testing.T) {
	deps, alice := mailFixture(t, 1)
	bob := deps.User
	items, _ := deps.Store.Mail().ListInboxFor(context.Background(), bob.ID, 10)
	parent := items[0]

	m := newMailComposeModel(deps, alice.UserID, parent.ID)
	if m.subject.Value() == "" {
		t.Errorf("reply did not pre-fill subject")
	}
	m.body.SetValue("ok")

	_, _ = m.submit()
	// alice now has one reply in her inbox.
	repl, err := deps.Store.Mail().ListInboxFor(context.Background(), alice.ID, 10)
	if err != nil || len(repl) != 1 {
		t.Fatalf("alice inbox after reply: %d (err=%v)", len(repl), err)
	}
	if !repl[0].ParentID.Valid || repl[0].ParentID.Int64 != parent.ID {
		t.Errorf("reply ParentID = %v, want valid=%d", repl[0].ParentID, parent.ID)
	}
	if repl[0].ThreadID != parent.ThreadID {
		t.Errorf("reply ThreadID = %d, want %d", repl[0].ThreadID, parent.ThreadID)
	}
}

// Reply pre-fills body with the parent quoted in markdown blockquote
// form (`> ` prefix on every line plus an attribution line). Cursor
// implicitly lands at the end of the trailing blank line so the user
// can start typing immediately.
func TestMailCompose_ReplyQuotesParentBody(t *testing.T) {
	deps, alice := mailFixture(t, 0)
	// Insert a parent with a multi-line body addressed to bob (deps.User).
	bob := deps.User
	parent, err := deps.Store.Mail().Insert(
		context.Background(),
		alice.ID, alice.UserID, bob.ID,
		"original",
		"first line\nsecond line\n\nfourth line",
		nil,
	)
	if err != nil {
		t.Fatalf("seed parent: %v", err)
	}
	m := newMailComposeModel(deps, alice.UserID, parent.ID)

	got := m.body.Value()
	if !strings.Contains(got, "> "+alice.UserID+" · ") {
		t.Errorf("body missing attribution line; got:\n%s", got)
	}
	for _, line := range []string{"> first line", "> second line", "> ", "> fourth line"} {
		if !strings.Contains(got, line) {
			t.Errorf("body missing quoted line %q; got:\n%s", line, got)
		}
	}
	// Trailing blank line so cursor sits ready to type below the quote.
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("body should end in blank line; got %q", got[len(got)-min(20, len(got)):])
	}
}

// Self-mail (memo): same skip-Broker.Send pattern as wb compose.
// Verified by (a) no error / sent flag set, (b) row exists, (c) read_at
// auto-populated so it doesn't bump the unread counter on reconnect.
// Pre-fix this test would have deadlocked the runner.
func TestMailCompose_AllowsSelfMemo(t *testing.T) {
	deps, _ := mailFixture(t, 0)
	bob := deps.User
	m := newMailComposeModel(deps, bob.UserID, 0)
	m.subject.SetValue("todo")
	m.body.SetValue("buy milk")
	model, _ := m.submit()
	cm := model.(mailComposeModel)
	if cm.err != "" {
		t.Fatalf("self-mail errored: %q", cm.err)
	}
	if cm.sent != bob.UserID {
		t.Errorf("sent = %q, want %q", cm.sent, bob.UserID)
	}
	got, _ := deps.Store.Mail().ListInboxFor(context.Background(), bob.ID, 10)
	if len(got) != 1 {
		t.Fatalf("inbox = %d rows, want 1", len(got))
	}
	if !got[0].ReadAt.Valid {
		t.Error("self-mail not auto-marked read — would bump unread counter on reconnect")
	}
}

func TestMailCompose_EmptyFieldsBlockSubmit(t *testing.T) {
	deps, _ := mailFixture(t, 0)
	cases := []struct {
		name      string
		to, sub, body string
		wantErr   string
	}{
		{"no recipient", "", "subj", "body", "recipient required"},
		{"no subject", "alice", "", "body", "subject required"},
		{"no body", "alice", "subj", "", "body required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newMailComposeModel(deps, tc.to, 0)
			m.subject.SetValue(tc.sub)
			m.body.SetValue(tc.body)
			model, _ := m.submit()
			if model.(mailComposeModel).err != tc.wantErr {
				t.Errorf("err = %q, want %q", model.(mailComposeModel).err, tc.wantErr)
			}
		})
	}
}

// Submitting to a non-existent user yields a friendly error, not a panic.
func TestMailCompose_UnknownRecipient(t *testing.T) {
	deps, _ := mailFixture(t, 0)
	m := newMailComposeModel(deps, "nosuchuser", 0)
	m.subject.SetValue("s")
	m.body.SetValue("b")
	model, _ := m.submit()
	got := model.(mailComposeModel).err
	if got != "no such user: nosuchuser" {
		t.Errorf("err = %q", got)
	}
}

// Submission triggers a MailIncomingMsg send to the recipient (so an open
// inbox can refresh).
func TestMailCompose_SendsMailIncomingMsg(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	br := chat.NewBroker()
	bobRec := &recordingSender{}
	br.Register(&chat.Session{UserID: bob.ID, UserIDStr: bob.UserID, Program: bobRec})
	deps := Deps{Store: st, User: alice, Broker: br}

	m := newMailComposeModel(deps, "bob", 0)
	m.subject.SetValue("hi")
	m.body.SetValue("body")
	_, _ = m.submit()

	if len(bobRec.msgs) != 1 {
		t.Fatalf("bob got %d msgs, want 1", len(bobRec.msgs))
	}
	in, ok := bobRec.msgs[0].(MailIncomingMsg)
	if !ok {
		t.Fatalf("got %T, want MailIncomingMsg", bobRec.msgs[0])
	}
	if in.Subject != "hi" || in.FromUserID != "alice" {
		t.Errorf("MailIncomingMsg = %+v", in)
	}
}
