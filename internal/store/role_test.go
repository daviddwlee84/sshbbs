package store_test

import (
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

func TestRole_AtLeast(t *testing.T) {
	cases := []struct {
		r, min store.Role
		want   bool
	}{
		// Same tier — true.
		{store.RoleGuest, store.RoleGuest, true},
		{store.RoleUser, store.RoleUser, true},
		{store.RoleMod, store.RoleMod, true},
		{store.RoleAdmin, store.RoleAdmin, true},
		// Above the floor — true.
		{store.RoleAdmin, store.RoleGuest, true},
		{store.RoleAdmin, store.RoleUser, true},
		{store.RoleAdmin, store.RoleMod, true},
		{store.RoleMod, store.RoleUser, true},
		{store.RoleMod, store.RoleGuest, true},
		{store.RoleUser, store.RoleGuest, true},
		// Below the floor — false.
		{store.RoleGuest, store.RoleUser, false},
		{store.RoleGuest, store.RoleMod, false},
		{store.RoleGuest, store.RoleAdmin, false},
		{store.RoleUser, store.RoleMod, false},
		{store.RoleUser, store.RoleAdmin, false},
		{store.RoleMod, store.RoleAdmin, false},
		// Unknown role — fail-closed.
		{store.Role("root"), store.RoleAdmin, false},
		{store.RoleAdmin, store.Role("root"), false},
	}
	for _, tc := range cases {
		t.Run(string(tc.r)+"_at_least_"+string(tc.min), func(t *testing.T) {
			if got := tc.r.AtLeast(tc.min); got != tc.want {
				t.Errorf("Role(%q).AtLeast(%q) = %v, want %v", tc.r, tc.min, got, tc.want)
			}
		})
	}
}

func TestRole_Valid(t *testing.T) {
	cases := []struct {
		r    store.Role
		want bool
	}{
		{store.RoleGuest, true},
		{store.RoleUser, true},
		{store.RoleMod, true},
		{store.RoleAdmin, true},
		{store.Role(""), false},
		{store.Role("root"), false},
		{store.Role("ADMIN"), false}, // case-sensitive — column values are always lowercase
	}
	for _, tc := range cases {
		t.Run(string(tc.r), func(t *testing.T) {
			if got := tc.r.Valid(); got != tc.want {
				t.Errorf("Role(%q).Valid() = %v, want %v", tc.r, got, tc.want)
			}
		})
	}
}
