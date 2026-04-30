// Package markdown is the round-trip serializer/parser for articles.
//
// An article (with optional pushes) renders to a self-describing
// human-readable document with YAML-ish frontmatter:
//
//	---
//	title: 歡迎來到 SSH-BBS
//	board: Welcome
//	author: admin
//	created_at: 2026-04-30T12:34:56Z
//	updated_at: 2026-04-30T13:00:00Z
//	score: 0
//	id: 1
//	---
//
//	[arbitrary body — markdown or plain text]
//
//	<!-- sshbbs:pushes -->
//	- 推 [alice] 2026-04-30T12:35:01Z  great post
//	- 噓 [bob]   2026-04-30T12:35:14Z  disagree
//	- → [carol]  2026-04-30T12:35:20Z  fyi
//
// The format is intentionally narrow: each frontmatter line is
// `key: value`, no nesting, no quoting. Push lines are
// `- <glyph> [<author>] <RFC3339-ts>  <body>` with a TWO-space separator
// between timestamp and body.
//
// The package is a leaf — pure functions over store types, no DB access.
package markdown

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

const (
	frontmatterDelim = "---"

	// PushesSentinel marks the start of the pushes block. Public so callers
	// (UI, tests, docs) can warn users not to embed this exact string in
	// their article body. If a body contains the sentinel, Format will
	// suffix it with the literal-marker so Parse can restore the original.
	PushesSentinel = "<!-- sshbbs:pushes -->"

	// pushesLiteralMarker is appended to a sentinel that legitimately
	// appears inside the body. Parse strips it back out.
	pushesLiteralMarker = "<!-- sshbbs:literal -->"
)

// FormatOpts controls the output of Format.
type FormatOpts struct {
	// IncludePushes appends the pushes section after the body.
	IncludePushes bool
	// BoardName is rendered into the frontmatter. Pass the board name
	// (the package can't look it up — store types don't carry it).
	BoardName string
}

// Parsed is the round-trip target. It deliberately does NOT return a
// *store.Article because Parse runs without DB context: the caller must
// resolve BoardName → BoardID and AuthorName → AuthorID before INSERT.
type Parsed struct {
	Title      string
	BoardName  string
	AuthorName string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ScoreHint  int64
	IDHint     int64
	Body       string
	Pushes     []ParsedPush
	// Extra preserves any frontmatter key we don't recognize, so future
	// schema additions don't drop data on a round-trip through an older
	// build.
	Extra map[string]string
}

// ParsedPush mirrors store.Push but with author as a name (not user_id) and
// without a DB-side ID; the caller decides whether/how to materialize it.
type ParsedPush struct {
	Kind      store.PushKind
	Author    string
	CreatedAt time.Time
	Body      string
}

// Format renders an article (plus optional pushes) to the canonical text
// form. Returns an error only on a nil article; otherwise it always
// produces a valid, parseable document.
func Format(a *store.Article, pushes []*store.Push, opts FormatOpts) (string, error) {
	if a == nil {
		return "", fmt.Errorf("markdown.Format: article is nil")
	}
	var b strings.Builder
	b.WriteString(frontmatterDelim + "\n")
	writeFrontmatter(&b, "title", sanitizeFM(a.Title))
	if opts.BoardName != "" {
		writeFrontmatter(&b, "board", sanitizeFM(opts.BoardName))
	}
	if a.AuthorUserID != "" {
		writeFrontmatter(&b, "author", sanitizeFM(a.AuthorUserID))
	}
	if !a.CreatedAt.IsZero() {
		writeFrontmatter(&b, "created_at", a.CreatedAt.UTC().Format(time.RFC3339))
	}
	if a.UpdatedAt.Valid {
		writeFrontmatter(&b, "updated_at", a.UpdatedAt.Time.UTC().Format(time.RFC3339))
	}
	writeFrontmatter(&b, "score", strconv.FormatInt(a.RecommendScore, 10))
	if a.ID > 0 {
		writeFrontmatter(&b, "id", strconv.FormatInt(a.ID, 10))
	}
	b.WriteString(frontmatterDelim + "\n\n")

	body := a.Body
	// Disambiguate any literal sentinel inside the body so Parse won't
	// mis-cleave at it. Best-effort: if the user already has the
	// literal-marker after the sentinel, we leave it alone.
	if strings.Contains(body, PushesSentinel) {
		body = strings.ReplaceAll(body, PushesSentinel+pushesLiteralMarker, "\x00LITERAL_MARKED\x00")
		body = strings.ReplaceAll(body, PushesSentinel, PushesSentinel+pushesLiteralMarker)
		body = strings.ReplaceAll(body, "\x00LITERAL_MARKED\x00", PushesSentinel+pushesLiteralMarker)
	}
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}

	if opts.IncludePushes && len(pushes) > 0 {
		b.WriteString("\n" + PushesSentinel + "\n")
		for _, p := range pushes {
			ts := p.CreatedAt.UTC().Format(time.RFC3339)
			// Push body is single-line by DB convention; flatten just in case.
			pb := strings.ReplaceAll(p.Body, "\n", " ")
			fmt.Fprintf(&b, "- %s [%s] %s  %s\n", p.Kind.Glyph(), p.UserUserID, ts, pb)
		}
	}

	return b.String(), nil
}

// Parse reads markdown back into a Parsed. Permissive: missing frontmatter
// or pushes section degrade to zero values; the whole input becomes Body
// when no frontmatter is present.
func Parse(md string) (*Parsed, error) {
	p := &Parsed{Extra: map[string]string{}}
	rest := md

	if strings.HasPrefix(rest, frontmatterDelim+"\n") {
		rest = rest[len(frontmatterDelim)+1:]
		closeIdx := strings.Index(rest, "\n"+frontmatterDelim+"\n")
		closeAtEOF := false
		if closeIdx < 0 && strings.HasSuffix(rest, "\n"+frontmatterDelim) {
			closeIdx = len(rest) - len("\n"+frontmatterDelim)
			closeAtEOF = true
		}
		if closeIdx >= 0 {
			fm := rest[:closeIdx]
			if closeAtEOF {
				rest = ""
			} else {
				rest = rest[closeIdx+len("\n"+frontmatterDelim+"\n"):]
			}
			parseFrontmatter(p, fm)
		}
	}

	rest = strings.TrimLeft(rest, "\n")
	if strings.Contains(rest, PushesSentinel) {
		// The sentinel may be followed by literal-marker, which means it
		// belongs to the body. Walk past every literal-marked occurrence.
		head := rest
		offset := 0
		for {
			i := strings.Index(head, PushesSentinel)
			if i < 0 {
				// Fell through without a real sentinel — entire rest is body.
				p.Body = stripLiteralMarkers(strings.TrimRight(rest, "\n"))
				return p, nil
			}
			afterSentinel := i + len(PushesSentinel)
			if strings.HasPrefix(head[afterSentinel:], pushesLiteralMarker) {
				// Skip — it's part of the body. Continue searching.
				next := afterSentinel + len(pushesLiteralMarker)
				offset += next
				head = head[next:]
				continue
			}
			// Real sentinel — split.
			cut := offset + i
			body := strings.TrimRight(rest[:cut], "\n")
			p.Body = stripLiteralMarkers(body)
			parsePushes(p, rest[cut+len(PushesSentinel):])
			return p, nil
		}
	}
	p.Body = stripLiteralMarkers(strings.TrimRight(rest, "\n"))
	return p, nil
}

func writeFrontmatter(b *strings.Builder, key, val string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(val)
	b.WriteString("\n")
}

func sanitizeFM(s string) string {
	// Frontmatter values are single-line. CR/LF would break the parser;
	// flatten to space.
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func parseFrontmatter(p *Parsed, fm string) {
	for line := range strings.SplitSeq(fm, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "title":
			p.Title = val
		case "board":
			p.BoardName = val
		case "author":
			p.AuthorName = val
		case "created_at":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				p.CreatedAt = t
			}
		case "updated_at":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				p.UpdatedAt = t
			}
		case "score":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				p.ScoreHint = n
			}
		case "id":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				p.IDHint = n
			}
		default:
			p.Extra[key] = val
		}
	}
}

func parsePushes(p *Parsed, block string) {
	for line := range strings.SplitSeq(block, "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		if push, ok := parsePushLine(line[2:]); ok {
			p.Pushes = append(p.Pushes, push)
		}
	}
}

func parsePushLine(s string) (ParsedPush, bool) {
	// Format: "<glyph> [<author>] <RFC3339-ts>  <body>"
	s = strings.TrimSpace(s)
	if s == "" {
		return ParsedPush{}, false
	}

	// Glyph: take everything before the first whitespace.
	wsIdx := strings.IndexAny(s, " \t")
	if wsIdx <= 0 {
		return ParsedPush{}, false
	}
	glyph := s[:wsIdx]
	rest := strings.TrimLeft(s[wsIdx:], " \t")

	var kind store.PushKind
	switch glyph {
	case "推":
		kind = store.PushKindPush
	case "噓":
		kind = store.PushKindBoo
	case "→":
		kind = store.PushKindArrow
	default:
		return ParsedPush{}, false
	}

	if !strings.HasPrefix(rest, "[") {
		return ParsedPush{}, false
	}
	closeBracket := strings.IndexByte(rest, ']')
	if closeBracket <= 1 {
		return ParsedPush{}, false
	}
	author := rest[1:closeBracket]
	rest = strings.TrimLeft(rest[closeBracket+1:], " \t")

	// Timestamp + body separated by 2 spaces. If absent, fall back to one
	// space (lossy but tolerant of hand-edited files).
	var tsStr, body string
	if sepIdx := strings.Index(rest, "  "); sepIdx >= 0 {
		tsStr = rest[:sepIdx]
		body = strings.TrimLeft(rest[sepIdx:], " \t")
	} else if spaceIdx := strings.IndexAny(rest, " \t"); spaceIdx >= 0 {
		tsStr = rest[:spaceIdx]
		body = strings.TrimLeft(rest[spaceIdx:], " \t")
	} else {
		tsStr = rest
		body = ""
	}

	var ts time.Time
	if tsStr != "" {
		if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
			ts = t
		}
	}
	return ParsedPush{Kind: kind, Author: author, CreatedAt: ts, Body: body}, true
}

func stripLiteralMarkers(s string) string {
	return strings.ReplaceAll(s, PushesSentinel+pushesLiteralMarker, PushesSentinel)
}
