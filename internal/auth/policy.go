package auth

import (
	"net"
	"strings"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

// MustChangePasswordRemoteBlocked reports whether an SSH login attempt
// for an admin account whose password is still the bootstrap default
// should be refused because the connection came from outside the loopback
// range. The intent: if the binary is published with the baked default
// `admin`/`admin` and the operator never rotated it, a remote attacker
// can't crack it just by reading the source. Local administration via
// `ssh admin@localhost` (the documented bootstrap path) still works.
//
// The host parameter is the SSH session's RemoteAddr().String() — typically
// "127.0.0.1:1234" or "[::1]:5678". Unparseable hosts are treated as
// non-loopback (fail-closed).
func MustChangePasswordRemoteBlocked(u *store.User, host string) bool {
	if u == nil {
		return false
	}
	if u.Role != store.RoleAdmin {
		return false
	}
	if !u.MustChangePassword {
		return false
	}
	return !isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	// Strip the port if present. SplitHostPort fails on bare hosts like
	// "127.0.0.1" — fall back to using the raw string.
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	// Trim IPv6 brackets if SplitHostPort didn't (it normally does).
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	ip := net.ParseIP(h)
	if ip == nil {
		// Hostname like "localhost". Conservatively accept the literal
		// "localhost" string; anything else is non-loopback.
		return strings.EqualFold(h, "localhost")
	}
	return ip.IsLoopback()
}
