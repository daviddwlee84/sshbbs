package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestUserRepo_SetRole_Round(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	u := storetest.MustUser(t, st, "alice", "Alice")
	if u.Role != store.RoleUser {
		t.Fatalf("default Role = %q, want user", u.Role)
	}
	for _, r := range []store.Role{store.RoleMod, store.RoleAdmin, store.RoleGuest, store.RoleUser} {
		if err := st.Users().SetRole(ctx, u.ID, r); err != nil {
			t.Fatalf("SetRole(%s): %v", r, err)
		}
		fresh, _ := st.Users().GetByID(ctx, u.ID)
		if fresh.Role != r {
			t.Errorf("after SetRole(%s): got %q", r, fresh.Role)
		}
	}
}

func TestUserRepo_SetRole_Invalid(t *testing.T) {
	st := storetest.New(t)
	u := storetest.MustUser(t, st, "alice", "")
	if err := st.Users().SetRole(context.Background(), u.ID, store.Role("root")); !errors.Is(err, store.ErrInvalidRole) {
		t.Errorf("got %v, want ErrInvalidRole", err)
	}
}

func TestUserRepo_CountByRole(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	a := storetest.MustUser(t, st, "alice", "")
	b := storetest.MustUser(t, st, "bob", "")
	c := storetest.MustUser(t, st, "carol", "")

	if err := st.Users().SetRole(ctx, a.ID, store.RoleAdmin); err != nil {
		t.Fatalf("set admin: %v", err)
	}
	if err := st.Users().SetRole(ctx, b.ID, store.RoleMod); err != nil {
		t.Fatalf("set mod: %v", err)
	}
	_ = c

	cases := map[store.Role]int{
		store.RoleAdmin: 1,
		store.RoleMod:   1,
		store.RoleUser:  1, // carol stays default 'user'
		store.RoleGuest: 0,
	}
	for role, want := range cases {
		got, err := st.Users().CountByRole(ctx, role)
		if err != nil {
			t.Fatalf("CountByRole(%s): %v", role, err)
		}
		if got != want {
			t.Errorf("count(%s) = %d, want %d", role, got, want)
		}
	}
}

func TestUserRepo_ListAll_Pagination(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	// Seed 5 users.
	for _, name := range []string{"u1", "u2", "u3", "u4", "u5"} {
		_ = storetest.MustUser(t, st, name, "")
	}

	page1, err := st.Users().ListAll(ctx, 2, 0)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page 1 size = %d, want 2", len(page1))
	}
	page2, _ := st.Users().ListAll(ctx, 2, 2)
	if len(page2) != 2 {
		t.Errorf("page 2 size = %d, want 2", len(page2))
	}
	page3, _ := st.Users().ListAll(ctx, 2, 4)
	if len(page3) != 1 {
		t.Errorf("page 3 size = %d, want 1 (last)", len(page3))
	}
	page4, _ := st.Users().ListAll(ctx, 2, 6)
	if len(page4) != 0 {
		t.Errorf("page 4 size = %d, want 0 (empty)", len(page4))
	}

	// Default limit kicks in when limit<=0.
	def, _ := st.Users().ListAll(ctx, 0, 0)
	if len(def) != 5 {
		t.Errorf("default limit page size = %d, want 5", len(def))
	}
}

func TestUserRepo_SetPassword_ClearsFlag(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	u, err := st.Users().InsertSystemAccount(ctx, "admin", "$2a$12$placeholder.hash.value.for.test.placeholder.value", store.RoleAdmin, true)
	if err != nil {
		t.Fatalf("InsertSystemAccount: %v", err)
	}
	if !u.MustChangePassword {
		t.Fatal("seeded admin must have must_change_password=true")
	}
	if err := st.Users().SetPassword(ctx, u.ID, "$2a$12$new.value.for.test.placeholder.x"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	fresh, _ := st.Users().GetByID(ctx, u.ID)
	if fresh.MustChangePassword {
		t.Error("must_change_password not cleared by SetPassword")
	}
}
