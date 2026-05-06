package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

// buildRequest constructs the HTTP request for a single (target, event)
// pair. Format is inferred from the target URL — Discord webhook URLs
// get the Discord embed envelope, everything else gets the generic
// apprise-style {title, body} JSON. Adding a new direct integration
// (Slack, Telegram bot HTTP, ...) means adding one URL pattern + one
// encoder here, no schema or UI change required.
func buildRequest(ctx context.Context, target *store.NotifyTarget, ev Event) (*http.Request, error) {
	switch detectFormat(target.URL) {
	case formatDiscord:
		return buildDiscordRequest(ctx, target, ev)
	default:
		return buildAppriseRequest(ctx, target, ev)
	}
}

type targetFormat int

const (
	formatApprise targetFormat = iota // {title, body} — apprise-api, ntfy.sh, generic
	formatDiscord                     // embed-wrapped — direct Discord webhook URL
)

// detectFormat picks an encoder by URL prefix. Order: more specific patterns
// first; the apprise default is the catch-all.
func detectFormat(rawURL string) targetFormat {
	u := strings.ToLower(rawURL)
	switch {
	case strings.HasPrefix(u, "https://discord.com/api/webhooks/"),
		strings.HasPrefix(u, "https://discordapp.com/api/webhooks/"),
		strings.HasPrefix(u, "https://canary.discord.com/api/webhooks/"),
		strings.HasPrefix(u, "https://ptb.discord.com/api/webhooks/"):
		return formatDiscord
	}
	return formatApprise
}

// buildAppriseRequest is the default encoder: a flat {title, body} JSON
// document. caronc/apprise-api consumes this directly; ntfy.sh / Slack
// incoming-webhook / generic receivers either accept it as-is or template
// against the named fields.
func buildAppriseRequest(ctx context.Context, target *store.NotifyTarget, ev Event) (*http.Request, error) {
	body, err := json.Marshal(payload{Title: ev.Title, Body: ev.Body})
	if err != nil {
		return nil, fmt.Errorf("apprise marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sshbbs-notify/1")
	return req, nil
}

// Discord webhook envelope. Reference:
// https://discord.com/developers/docs/resources/webhook#execute-webhook
//
// Discord rejects payloads with neither `content` nor `embeds` set, so we
// always produce one embed with title + description. Field length limits
// are 256 (title) / 4096 (description) — we truncate runes (CJK-safe), not
// bytes.
type discordPayload struct {
	Username string         `json:"username,omitempty"`
	Embeds   []discordEmbed `json:"embeds,omitempty"`
}

type discordEmbed struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Color       int    `json:"color,omitempty"`
}

const (
	discordTitleMax = 256
	discordDescMax  = 4096
	discordUsername = "SSH-BBS"
	// Lipgloss-style accent purple, picked to match the BBS header style.
	discordColor = 0x9D6FFF
)

func buildDiscordRequest(ctx context.Context, target *store.NotifyTarget, ev Event) (*http.Request, error) {
	embed := discordEmbed{
		Title:       truncateRunes(ev.Title, discordTitleMax),
		Description: truncateRunes(ev.Body, discordDescMax),
		Color:       discordColor,
	}
	// Discord rejects an empty embed (no title / description / fields).
	// Should never happen given the BBS always sets a title, but guard
	// anyway so a misuse fails loudly during dev rather than silently
	// at delivery.
	if embed.Title == "" && embed.Description == "" {
		embed.Description = "(empty notification)"
	}
	p := discordPayload{
		Username: discordUsername,
		Embeds:   []discordEmbed{embed},
	}
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("discord marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sshbbs-notify/1")
	return req, nil
}

// truncateRunes caps s to at most max runes. CJK-safe (counts glyphs, not
// bytes). When trimming, replaces the tail with U+2026 HORIZONTAL ELLIPSIS
// so receivers can tell content was cut.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
