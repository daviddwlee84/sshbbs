# sshbbs demo recording — driven by VHS (https://github.com/charmbracelet/vhs)
#
# Render with:
#   ./scripts/record-demo.sh
#
# The wrapper script handles environment setup (rebuilds the binary, wipes
# data/bbs.db, registers `alice`, seeds two demo articles, boots the
# server) before invoking `vhs scripts/demo.tape`. The tape itself only
# drives a real ssh client. Produces docs/demo.gif (embedded in README)
# and docs/demo.webm (higher-quality fallback for external links).

Output docs/demo.gif
Output docs/demo.webm

Set Shell        "bash"
Set FontSize     14
Set Width        1100
Set Height       700
Set Padding      14
Set Theme        "Catppuccin Mocha"
Set TypingSpeed  60ms
Set PlaybackSpeed 1.0

# Off-camera: clear scrollback so the recording starts on a blank prompt.
Hide
Type "clear"
Enter
Sleep 200ms
Show

# ── login as alice ─────────────────────────────────────────────────
Type "ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null alice@localhost -p 2222"
Enter
Sleep 1500ms
Type "pw123456"
Enter
# Main menu takes a moment to render after auth.
Sleep 2s

# ── main menu → boards (vim 'l' = enter, 'h' = back) ───────────────
Type "l"
Sleep 1800ms

# ── board list: 'j' moves cursor to Test (default order: ChitChat, Test, Welcome)
Type "j"
Sleep 900ms
Type "l"
Sleep 1800ms

# ── board view: cursor sits on the newest article; 'l' opens it
Type "l"
Sleep 2200ms

# ── article view: '+' opens the inline 推 input
Type "+"
Sleep 600ms
Type "vim navigation rocks!"
Sleep 500ms
Enter
Sleep 1800ms

# ── '-' for 噓 with a comment
Type "-"
Sleep 500ms
Type "(just kidding)"
Sleep 500ms
Enter
# Score should now be 0 (one push + one boo).
Sleep 2s

# ── 'h' walks back: article → board → board list → main menu
Type "h"
Sleep 1300ms
Type "h"
Sleep 1300ms
Type "h"
Sleep 1500ms

# ── Ctrl+U opens the water-balloon inbox from anywhere
Ctrl+U
Sleep 1800ms

# ── 'h' back to main menu, 'q' to disconnect cleanly
Type "h"
Sleep 1000ms
Type "q"
Sleep 1500ms
