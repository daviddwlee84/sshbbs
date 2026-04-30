package server

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"

	"github.com/daviddwlee84/sshbbs/internal/auth"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

// Context keys for values stashed by PasswordAuth and read by teaHandler.
type ctxKey string

const (
	ctxKeyUserID     ctxKey = "user_id"      // int64; set on real login
	ctxKeyRegister   ctxKey = "register"     // bool; true if SSH user == "new"
	ctxKeyRemoteHost ctxKey = "remote_host"  // string; client IP
)

func PasswordAuth(st *store.Store) ssh.PasswordHandler {
	return func(ctx ssh.Context, password string) bool {
		host := ""
		if a := ctx.RemoteAddr(); a != nil {
			host = a.String()
		}
		ctx.SetValue(ctxKeyRemoteHost, host)

		switch ctx.User() {
		case auth.ReservedUsernameNew:
			ctx.SetValue(ctxKeyRegister, true)
			return true
		case auth.ReservedUsernameGuest:
			// Read-only spectator: short-circuit to the seeded guest row,
			// no password check. Refuse if seeding never ran or the row
			// was tampered to a non-guest role.
			u, err := st.Users().GetByUserID(context.Background(), auth.ReservedUsernameGuest)
			if err != nil || u.Role != store.RoleGuest {
				log.Warn("guest login refused", "host", host, "err", err)
				return false
			}
			ctx.SetValue(ctxKeyUserID, u.ID)
			log.Info("auth ok (guest)", "host", host)
			return true
		}
		user, err := auth.VerifyLogin(context.Background(), st, ctx.User(), password, host)
		if err != nil {
			log.Info("auth failed", "user", ctx.User(), "host", host, "err", err)
			return false
		}
		if auth.MustChangePasswordRemoteBlocked(user, host) {
			log.Warn("admin remote login blocked while must_change_password=1",
				"user", user.UserID, "host", host)
			return false
		}
		ctx.SetValue(ctxKeyUserID, user.ID)
		log.Info("auth ok", "user", user.UserID, "host", host)
		return true
	}
}
