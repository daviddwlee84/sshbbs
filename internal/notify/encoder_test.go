package notify

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		url  string
		want targetFormat
	}{
		{"https://discord.com/api/webhooks/123/abc", formatDiscord},
		{"https://discordapp.com/api/webhooks/123/abc", formatDiscord},
		{"https://canary.discord.com/api/webhooks/123/abc", formatDiscord},
		{"https://ptb.discord.com/api/webhooks/123/abc", formatDiscord},
		{"HTTPS://Discord.com/API/Webhooks/123/abc", formatDiscord}, // case-insensitive
		{"https://apprise:8000/notify/mykey", formatApprise},
		{"http://localhost:8000/notify/mykey", formatApprise},
		{"https://ntfy.sh/my-topic", formatApprise},
		{"https://hooks.slack.com/services/T/B/X", formatApprise}, // not yet wired
		{"https://example.com/discord-fake/api/webhooks/", formatApprise}, // pattern is prefix-anchored
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			if got := detectFormat(tc.url); got != tc.want {
				t.Errorf("detectFormat(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestBuildDiscordRequest_PayloadShape(t *testing.T) {
	target := &store.NotifyTarget{
		ID:  1,
		URL: "https://discord.com/api/webhooks/123/abc",
	}
	ev := Event{
		Kind:     KindPush,
		ToUserID: 42,
		Title:    "[BBS] alice 推了你的文章 «Hello»",
		Body:     "→ 推 nice post\n\nsecond line",
	}
	req, err := buildDiscordRequest(context.Background(), target, ev)
	if err != nil {
		t.Fatalf("buildDiscordRequest: %v", err)
	}
	if req.Method != "POST" {
		t.Errorf("Method = %q, want POST", req.Method)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if !strings.Contains(req.URL.String(), "discord.com/api/webhooks") {
		t.Errorf("URL = %q", req.URL.String())
	}
	body, _ := io.ReadAll(req.Body)

	var got discordPayload
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, body)
	}
	if got.Username != discordUsername {
		t.Errorf("username = %q, want %q", got.Username, discordUsername)
	}
	if len(got.Embeds) != 1 {
		t.Fatalf("got %d embeds, want 1", len(got.Embeds))
	}
	em := got.Embeds[0]
	if em.Title != ev.Title {
		t.Errorf("embed title = %q, want %q", em.Title, ev.Title)
	}
	if em.Description != ev.Body {
		t.Errorf("embed description = %q, want %q", em.Description, ev.Body)
	}
	if em.Color == 0 {
		t.Errorf("embed color = 0, want non-zero accent")
	}
}

func TestBuildDiscordRequest_TruncatesLongFields(t *testing.T) {
	target := &store.NotifyTarget{URL: "https://discord.com/api/webhooks/x/y"}
	// 300 CJK runes — well over the 256-rune title limit, well under
	// the 4096-rune description limit.
	longTitle := strings.Repeat("中", 300)
	ev := Event{Title: longTitle, Body: "ok"}
	req, _ := buildDiscordRequest(context.Background(), target, ev)
	body, _ := io.ReadAll(req.Body)
	var got discordPayload
	_ = json.Unmarshal(body, &got)
	titleRunes := []rune(got.Embeds[0].Title)
	if len(titleRunes) > discordTitleMax {
		t.Errorf("title runes = %d, want <= %d", len(titleRunes), discordTitleMax)
	}
	if !strings.HasSuffix(got.Embeds[0].Title, "…") {
		t.Errorf("truncated title should end with U+2026, got %q", got.Embeds[0].Title)
	}
}

func TestBuildDiscordRequest_EmptyContentGuard(t *testing.T) {
	target := &store.NotifyTarget{URL: "https://discord.com/api/webhooks/x/y"}
	req, err := buildDiscordRequest(context.Background(), target, Event{})
	if err != nil {
		t.Fatalf("buildDiscordRequest: %v", err)
	}
	body, _ := io.ReadAll(req.Body)
	var got discordPayload
	_ = json.Unmarshal(body, &got)
	em := got.Embeds[0]
	if em.Title == "" && em.Description == "" {
		t.Errorf("empty embed would be rejected by Discord; got %+v", em)
	}
}

func TestBuildAppriseRequest_PayloadShape(t *testing.T) {
	target := &store.NotifyTarget{URL: "http://apprise:8000/notify/mykey"}
	ev := Event{Title: "T", Body: "B"}
	req, err := buildAppriseRequest(context.Background(), target, ev)
	if err != nil {
		t.Fatalf("buildAppriseRequest: %v", err)
	}
	body, _ := io.ReadAll(req.Body)
	var got struct{ Title, Body string }
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Title != "T" || got.Body != "B" {
		t.Errorf("apprise payload = %+v", got)
	}
}

func TestRedactURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "discord webhook hides token only",
			in:   "https://discord.com/api/webhooks/1111111111111111111/xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-yyyyyyyyyyyyyyyyyyyyyyyy",
			want: "https://discord.com/api/webhooks/1111111111111111111/<redacted>",
		},
		{
			name: "slack webhook hides signing key",
			in:   "https://hooks.slack.com/services/T123/B456/xVeRyS3cret",
			want: "https://hooks.slack.com/services/T123/B456/<redacted>",
		},
		{
			name: "ntfy topic is the credential",
			in:   "https://ntfy.sh/my-secret-topic",
			want: "https://ntfy.sh/<redacted>",
		},
		{
			name: "apprise key redacted",
			in:   "http://apprise:8000/notify/alice-discord",
			want: "http://apprise:8000/notify/<redacted>",
		},
		{
			name: "query string also stripped",
			in:   "https://example.com/notify?token=abc123&user=alice",
			want: "https://example.com/<redacted>",
		},
		{
			name: "root path returns unchanged",
			in:   "https://example.com/",
			want: "https://example.com/",
		},
		{
			name: "no path returns unchanged",
			in:   "https://example.com",
			want: "https://example.com",
		},
		{
			name: "malformed url returns as-is",
			in:   "not a url",
			want: "not a url",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactURL(tc.in); got != tc.want {
				t.Errorf("RedactURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"中文測試", 4, "中文測試"},
		{"中文測試", 3, "中文…"},
		{"", 5, ""},
		{"x", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := truncateRunes(tc.in, tc.max); got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}
