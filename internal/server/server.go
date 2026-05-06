package server

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"
	gossh "golang.org/x/crypto/ssh"

	"github.com/daviddwlee84/sshbbs/internal/auth"
	"github.com/daviddwlee84/sshbbs/internal/chat"
	"github.com/daviddwlee84/sshbbs/internal/notify"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/tui"
)

type Config struct {
	Addr    string
	HostKey string
}

func New(cfg Config, st *store.Store, broker *chat.Broker, notifyMgr *notify.Manager) (*ssh.Server, error) {
	return wish.NewServer(
		wish.WithAddress(cfg.Addr),
		wish.WithHostKeyPath(cfg.HostKey),
		wish.WithPasswordAuth(PasswordAuth(st)),
		withReservedNoneAuth(),
		wish.WithMiddleware(
			bubbletea.MiddlewareWithProgramHandler(makeProgramHandler(st, broker, notifyMgr), termenv.ANSI256),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
}

// withReservedNoneAuth enables SSH "none" authentication for the two
// reserved usernames where a password is meaningless: `guest` (read-only
// spectator) and `new` (in-TUI registration). Other usernames still go
// through the password callback.
//
// charmbracelet/ssh only sets NoClientAuth=true when no other handlers
// are configured (see its server.go config()), so we override its
// ServerConfigCallback to force NoClientAuth=true with a per-username
// callback. The library still adds HostKey + PasswordCallback on top of
// our config, so password auth keeps working for everyone else.
//
// Effect: `ssh guest@host -p 2222` and `ssh new@host -p 2222` connect
// with no password prompt. `ssh alice@host -p 2222` still prompts.
func withReservedNoneAuth() ssh.Option {
	return func(s *ssh.Server) error {
		s.ServerConfigCallback = func(ctx ssh.Context) *gossh.ServerConfig {
			return &gossh.ServerConfig{
				NoClientAuth: true,
				NoClientAuthCallback: func(conn gossh.ConnMetadata) (*gossh.Permissions, error) {
					switch conn.User() {
					case auth.ReservedUsernameGuest, auth.ReservedUsernameNew:
						return nil, nil
					}
					return nil, errors.New("auth required")
				},
			}
		}
		return nil
	}
}

// makeProgramHandler is the wish ProgramHandler. It owns *tea.Program
// creation so we can hand the pointer to the chat broker for live message
// delivery, and arrange Unregister when the SSH session ends.
func makeProgramHandler(st *store.Store, broker *chat.Broker, notifyMgr *notify.Manager) bubbletea.ProgramHandler {
	return func(sess ssh.Session) *tea.Program {
		ctx := sess.Context()

		// Register-mode: SSH user was "new". No DB user, no broker registration.
		// `none` auth bypasses our PasswordHandler, so ctxKeyRegister isn't set
		// when a client connects via the none-auth path — fall back to inspecting
		// sess.User() directly.
		reg, _ := ctx.Value(ctxKeyRegister).(bool)
		if reg || sess.User() == auth.ReservedUsernameNew {
			deps := tui.Deps{Store: st, Broker: broker, Notify: notifyMgr, IsRegister: true}
			m := tui.NewRoot(deps)
			return tea.NewProgram(m, append(bubbletea.MakeOptions(sess), tea.WithAltScreen())...)
		}

		uid, _ := ctx.Value(ctxKeyUserID).(int64)
		if uid == 0 {
			// SSH `none` auth path: NoClientAuthCallback succeeded for
			// guest but didn't run our PasswordHandler, so user_id was
			// never stashed. Resolve the guest row here. Any other
			// username with uid==0 means a misconfiguration — reject.
			if sess.User() == auth.ReservedUsernameGuest {
				guest, err := st.Users().GetByUserID(context.Background(), auth.ReservedUsernameGuest)
				if err == nil && guest.Role == store.RoleGuest {
					uid = guest.ID
				}
			}
			if uid == 0 {
				log.Warn("session has no user_id; rejecting", "remote", sess.RemoteAddr(), "user", sess.User())
				return nil
			}
		}
		user, err := st.Users().GetByID(context.Background(), uid)
		if err != nil {
			log.Error("load user", "id", uid, "err", err)
			return nil
		}

		deps := tui.Deps{Store: st, User: user, Broker: broker, Notify: notifyMgr}
		if user.MustChangePassword {
			deps.MustChangePassword = true
		}
		m := tui.NewRoot(deps)
		p := tea.NewProgram(m, append(bubbletea.MakeOptions(sess), tea.WithAltScreen())...)

		// Guests are read-only spectators. We deliberately don't register
		// them in the broker — multiple `ssh guest@...` clients would
		// collide on the same user.ID, they can't reply to water balloons,
		// and they don't appear in the online list.
		if user.Role == store.RoleGuest {
			log.Info("guest session", "host", deps.User.UserID)
			return p
		}

		// Hook session into the broker so other users can send to us.
		cs := &chat.Session{UserID: user.ID, UserIDStr: user.UserID, Program: p}
		broker.Register(cs)
		log.Info("session registered", "user", user.UserID, "online", len(broker.OnlineList()))

		// Drain unread water balloons from DB and replay them as toasts.
		// We do this *before* the program starts so the messages land in
		// the model's first Update(); tea.Program.Send buffers if needed.
		go func() {
			unread, err := st.WaterBalloons().ListUnreadFor(context.Background(), user.ID)
			if err != nil {
				log.Warn("drain unread WBs", "user", user.UserID, "err", err)
				return
			}
			for _, w := range unread {
				p.Send(tui.WBIncomingMsg{ID: w.ID, FromUserID: w.FromUserIDStr, Body: w.Body})
				_ = st.WaterBalloons().MarkRead(context.Background(), w.ID)
			}
		}()

		// On session end, unregister so we stop trying to deliver to a dead Program.
		go func() {
			<-ctx.Done()
			broker.Unregister(user.ID, p)
			log.Info("session unregistered", "user", user.UserID, "online", len(broker.OnlineList()))
		}()

		return p
	}
}
