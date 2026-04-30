package auth_test

import (
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/auth"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

func TestMustChangePasswordRemoteBlocked(t *testing.T) {
	cases := []struct {
		name string
		user *store.User
		host string
		want bool
	}{
		{
			"admin must-change non-loopback IPv4 — blocked",
			&store.User{Role: store.RoleAdmin, MustChangePassword: true},
			"203.0.113.5:22",
			true,
		},
		{
			"admin must-change non-loopback IPv6 — blocked",
			&store.User{Role: store.RoleAdmin, MustChangePassword: true},
			"[2001:db8::1]:22",
			true,
		},
		{
			"admin must-change loopback IPv4 — allowed",
			&store.User{Role: store.RoleAdmin, MustChangePassword: true},
			"127.0.0.1:1234",
			false,
		},
		{
			"admin must-change loopback IPv6 — allowed",
			&store.User{Role: store.RoleAdmin, MustChangePassword: true},
			"[::1]:5678",
			false,
		},
		{
			"admin must-change literal localhost — allowed",
			&store.User{Role: store.RoleAdmin, MustChangePassword: true},
			"localhost:2222",
			false,
		},
		{
			"admin password rotated — never blocked",
			&store.User{Role: store.RoleAdmin, MustChangePassword: false},
			"203.0.113.5:22",
			false,
		},
		{
			"non-admin user — never blocked",
			&store.User{Role: store.RoleUser, MustChangePassword: true},
			"203.0.113.5:22",
			false,
		},
		{
			"nil user — false",
			nil,
			"203.0.113.5:22",
			false,
		},
		{
			"empty host — fail-closed, treated as non-loopback",
			&store.User{Role: store.RoleAdmin, MustChangePassword: true},
			"",
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := auth.MustChangePasswordRemoteBlocked(tc.user, tc.host); got != tc.want {
				t.Errorf("got %v, want %v (host=%q)", got, tc.want, tc.host)
			}
		})
	}
}
