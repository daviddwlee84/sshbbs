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

		if ctx.User() == auth.ReservedUsernameNew {
			ctx.SetValue(ctxKeyRegister, true)
			return true
		}
		user, err := auth.VerifyLogin(context.Background(), st, ctx.User(), password, host)
		if err != nil {
			log.Info("auth failed", "user", ctx.User(), "host", host, "err", err)
			return false
		}
		ctx.SetValue(ctxKeyUserID, user.ID)
		log.Info("auth ok", "user", user.UserID, "host", host)
		return true
	}
}
