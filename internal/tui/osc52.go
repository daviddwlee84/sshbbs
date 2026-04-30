package tui

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

// osc52Limit is a soft warning threshold. Many terminals (xterm in
// default config) drop OSC 52 payloads bigger than ~32-100 KiB without
// notice. Article bodies are usually short, but a long export with
// pushes can push past this. The export screen surfaces a toast when
// the rendered text exceeds the limit so the user knows to fall back to
// the file-write option.
const osc52Limit = 32 * 1024

// writeOSC52 emits the OSC 52 "set selection clipboard" escape sequence
// to the given writer. Most modern terminals honour this when allowed
// (iTerm2, kitty, wezterm, foot — yes; Terminal.app — opt-in only).
//
// Format: ESC ] 52 ; c ; <base64> ESC \
//
// Returns the number of bytes of payload (post-base64) actually written.
func writeOSC52(w io.Writer, payload string) (int, error) {
	enc := base64.StdEncoding.EncodeToString([]byte(payload))
	return fmt.Fprintf(w, "\x1b]52;c;%s\x1b\\", enc)
}

// emitClipboardOSC52 writes OSC 52 directly to os.Stdout so the SSH
// client's local terminal receives it. bubbletea programs run with
// os.Stdout wired to the SSH session's PTY, so this lands at the
// remote client unmodified.
func emitClipboardOSC52(payload string) error {
	_, err := writeOSC52(os.Stdout, payload)
	return err
}
