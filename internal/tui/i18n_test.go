package tui

import (
	"context"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/i18n"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestLocaleOf_NilUserFallsBackToDefault(t *testing.T) {
	if got := localeOf(Deps{}); got != i18n.Default {
		t.Errorf("localeOf(empty Deps) = %q, want Default %q", got, i18n.Default)
	}
}

func TestLocaleOf_HonoursUserLocale(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	// Default at create-time is zh-TW (migration 0010).
	if got := localeOf(Deps{User: u}); got != i18n.LocaleZHTW {
		t.Errorf("default = %q, want zh-TW", got)
	}
	if err := st.Users().SetLocale(context.Background(), u.ID, "en"); err != nil {
		t.Fatalf("SetLocale: %v", err)
	}
	fresh, _ := st.Users().GetByID(context.Background(), u.ID)
	if got := localeOf(Deps{User: fresh}); got != i18n.LocaleEN {
		t.Errorf("after SetLocale(en) = %q, want en", got)
	}
}

func TestLocaleOf_NormalisesUnknownStored(t *testing.T) {
	// Simulate a stale row that names a locale we no longer recognise
	// (could happen after a future "ja" support is removed). Falls back
	// to Default rather than rendering «key» everywhere.
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	if err := st.Users().SetLocale(context.Background(), u.ID, "zh-CN"); err != nil {
		t.Fatalf("SetLocale: %v", err)
	}
	fresh, _ := st.Users().GetByID(context.Background(), u.ID)
	if got := localeOf(Deps{User: fresh}); got != i18n.Default {
		t.Errorf("localeOf with unknown stored locale = %q, want Default", got)
	}
}

func TestRecipientLocale_LooksUpFromStore(t *testing.T) {
	st := storetest.New(t)
	alice := storetest.MustUser(t, st, "alice", "")
	bob := storetest.MustUser(t, st, "bob", "")
	if err := st.Users().SetLocale(context.Background(), bob.ID, "en"); err != nil {
		t.Fatalf("SetLocale: %v", err)
	}

	// alice is the sender (zh-TW); recipientLocale targets bob's id and
	// returns bob's stored locale, regardless of alice's setting. This is
	// the contract that lets a webhook notification render in the
	// recipient's preferred language even when the trigger event came
	// from another user.
	deps := Deps{Store: st, User: alice}
	if got := recipientLocale(deps, bob.ID); got != i18n.LocaleEN {
		t.Errorf("recipientLocale(bob.ID) = %q, want en", got)
	}
}

func TestRecipientLocale_FallsBackOnLookupError(t *testing.T) {
	// Nil Store → no DB to query; recipientLocale must not panic.
	if got := recipientLocale(Deps{}, 999); got != i18n.Default {
		t.Errorf("recipientLocale(nil store) = %q, want Default", got)
	}
	// Non-existent user id → store returns ErrUserNotFound; falls back.
	st := storetest.New(t)
	if got := recipientLocale(Deps{Store: st}, 999999); got != i18n.Default {
		t.Errorf("recipientLocale(missing user) = %q, want Default", got)
	}
}
