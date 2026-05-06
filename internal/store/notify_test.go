package store_test

import (
	"context"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestNotify_GetPrefsDefaults(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	got, err := st.Notify().GetPrefs(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetPrefs: %v", err)
	}
	want := store.DefaultNotifyPrefs()
	if got != want {
		t.Errorf("GetPrefs (no row) = %+v, want defaults %+v", got, want)
	}
}

func TestNotify_SetPrefsRoundTrip(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	ctx := context.Background()

	want := store.NotifyPrefs{
		OnPush: false, OnWB: true, OnMail: false, OnReply: true, OnlyWhenOffline: true,
	}
	if err := st.Notify().SetPrefs(ctx, u.ID, want); err != nil {
		t.Fatalf("SetPrefs: %v", err)
	}
	got, err := st.Notify().GetPrefs(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetPrefs: %v", err)
	}
	if got != want {
		t.Errorf("GetPrefs = %+v, want %+v", got, want)
	}

	// Upsert overwrites the existing row, not duplicates it.
	want2 := store.NotifyPrefs{OnPush: true}
	if err := st.Notify().SetPrefs(ctx, u.ID, want2); err != nil {
		t.Fatalf("SetPrefs second: %v", err)
	}
	got2, _ := st.Notify().GetPrefs(ctx, u.ID)
	if got2 != want2 {
		t.Errorf("GetPrefs after upsert = %+v, want %+v", got2, want2)
	}
}

func TestNotify_TargetCRUD(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	other := storetest.MustUser(t, st, "bob", "")
	ctx := context.Background()

	// Add two targets — one enabled by default, one we'll disable.
	id1, err := st.Notify().AddTarget(ctx, u.ID, "discord", "https://discord.com/api/webhooks/abc")
	if err != nil {
		t.Fatalf("AddTarget: %v", err)
	}
	id2, err := st.Notify().AddTarget(ctx, u.ID, "ntfy", "http://ntfy.sh/xyz")
	if err != nil {
		t.Fatalf("AddTarget 2: %v", err)
	}

	// List returns both, oldest first.
	all, err := st.Notify().ListTargets(ctx, u.ID)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListTargets = %d rows err=%v", len(all), err)
	}
	if all[0].ID != id1 || all[1].ID != id2 {
		t.Errorf("order = [%d,%d], want [%d,%d]", all[0].ID, all[1].ID, id1, id2)
	}

	// SetTargetEnabled gates on user_id — other can't disable alice's row.
	if err := st.Notify().SetTargetEnabled(ctx, id2, other.ID, false); err != store.ErrNotifyTargetNotFound {
		t.Errorf("cross-user disable: err = %v, want ErrNotifyTargetNotFound", err)
	}
	// Owner can.
	if err := st.Notify().SetTargetEnabled(ctx, id2, u.ID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}

	// ListEnabledTargets only returns the enabled one.
	enabled, _ := st.Notify().ListEnabledTargets(ctx, u.ID)
	if len(enabled) != 1 || enabled[0].ID != id1 {
		t.Errorf("ListEnabledTargets = %+v, want only id1=%d", enabled, id1)
	}

	// Update preserves enabled state when caller passes it.
	if err := st.Notify().UpdateTarget(ctx, id1, u.ID, "discord-personal", "https://example.com/hook", true); err != nil {
		t.Fatalf("UpdateTarget: %v", err)
	}
	all, _ = st.Notify().ListTargets(ctx, u.ID)
	if all[0].Label != "discord-personal" || all[0].URL != "https://example.com/hook" {
		t.Errorf("after update: %+v", all[0])
	}

	// Cross-user delete fails.
	if err := st.Notify().DeleteTarget(ctx, id1, other.ID); err != store.ErrNotifyTargetNotFound {
		t.Errorf("cross-user delete: err = %v", err)
	}
	if err := st.Notify().DeleteTarget(ctx, id1, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	all, _ = st.Notify().ListTargets(ctx, u.ID)
	if len(all) != 1 || all[0].ID != id2 {
		t.Errorf("after delete = %+v", all)
	}
}

func TestNotify_AddTargetRejectsBadURL(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	cases := []string{
		"",
		"discord://abc",
		"ftp://example.com",
		"example.com/no-scheme",
	}
	for _, badURL := range cases {
		if _, err := st.Notify().AddTarget(context.Background(), u.ID, "x", badURL); err == nil {
			t.Errorf("AddTarget(%q): err = nil, want validation error", badURL)
		}
	}
}

func TestUsers_SetBio(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	ctx := context.Background()
	if u.Bio != "" {
		t.Errorf("default Bio = %q, want empty", u.Bio)
	}
	if err := st.Users().SetBio(ctx, u.ID, "I write Go.\nMostly."); err != nil {
		t.Fatalf("SetBio: %v", err)
	}
	got, err := st.Users().GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Bio != "I write Go.\nMostly." {
		t.Errorf("Bio = %q", got.Bio)
	}
}
