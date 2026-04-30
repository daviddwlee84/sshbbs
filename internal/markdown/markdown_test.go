package markdown_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/daviddwlee84/sshbbs/internal/markdown"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func sampleArticle() *store.Article {
	return &store.Article{
		ID:             42,
		BoardID:        7,
		AuthorID:       3,
		AuthorUserID:   "alice",
		Title:          "Hello, BBS",
		Body:           "first line\nsecond line\n",
		RecommendScore: 5,
		CreatedAt:      time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      sql.NullTime{Time: time.Date(2026, 4, 30, 13, 0, 0, 0, time.UTC), Valid: true},
	}
}

func samplePushes() []*store.Push {
	return []*store.Push{
		{
			ArticleID:  42,
			UserUserID: "bob",
			Kind:       store.PushKindPush,
			Body:       "great",
			CreatedAt:  time.Date(2026, 4, 30, 12, 5, 0, 0, time.UTC),
		},
		{
			ArticleID:  42,
			UserUserID: "carol",
			Kind:       store.PushKindBoo,
			Body:       "meh",
			CreatedAt:  time.Date(2026, 4, 30, 12, 6, 0, 0, time.UTC),
		},
		{
			ArticleID:  42,
			UserUserID: "dave",
			Kind:       store.PushKindArrow,
			Body:       "fyi just looking",
			CreatedAt:  time.Date(2026, 4, 30, 12, 7, 0, 0, time.UTC),
		},
	}
}

func TestFormat_RoundTrip_BodyOnly(t *testing.T) {
	a := sampleArticle()
	out, err := markdown.Format(a, nil, markdown.FormatOpts{BoardName: "Welcome"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if !strings.HasPrefix(out, "---\n") {
		t.Fatalf("missing frontmatter delimiter:\n%s", out)
	}
	if !strings.Contains(out, "title: Hello, BBS\n") {
		t.Errorf("missing title line in:\n%s", out)
	}
	if !strings.Contains(out, "board: Welcome\n") {
		t.Errorf("missing board line in:\n%s", out)
	}

	parsed, err := markdown.Parse(out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Title != a.Title {
		t.Errorf("title: %q want %q", parsed.Title, a.Title)
	}
	if parsed.BoardName != "Welcome" {
		t.Errorf("board: %q want Welcome", parsed.BoardName)
	}
	if parsed.AuthorName != "alice" {
		t.Errorf("author: %q want alice", parsed.AuthorName)
	}
	if parsed.IDHint != 42 {
		t.Errorf("id: %d want 42", parsed.IDHint)
	}
	if parsed.ScoreHint != 5 {
		t.Errorf("score: %d want 5", parsed.ScoreHint)
	}
	if !parsed.CreatedAt.Equal(a.CreatedAt) {
		t.Errorf("created_at: %v want %v", parsed.CreatedAt, a.CreatedAt)
	}
	if !parsed.UpdatedAt.Equal(a.UpdatedAt.Time) {
		t.Errorf("updated_at: %v want %v", parsed.UpdatedAt, a.UpdatedAt.Time)
	}
	wantBody := "first line\nsecond line"
	if parsed.Body != wantBody {
		t.Errorf("body: %q want %q", parsed.Body, wantBody)
	}
	if len(parsed.Pushes) != 0 {
		t.Errorf("pushes: got %d, want 0", len(parsed.Pushes))
	}
}

func TestFormat_RoundTrip_WithPushes(t *testing.T) {
	a := sampleArticle()
	pushes := samplePushes()
	out, err := markdown.Format(a, pushes, markdown.FormatOpts{IncludePushes: true, BoardName: "Welcome"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if !strings.Contains(out, markdown.PushesSentinel) {
		t.Fatalf("missing pushes sentinel in:\n%s", out)
	}

	parsed, err := markdown.Parse(out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(parsed.Pushes) != 3 {
		t.Fatalf("pushes: got %d want 3 in:\n%s", len(parsed.Pushes), out)
	}
	wantKinds := []store.PushKind{store.PushKindPush, store.PushKindBoo, store.PushKindArrow}
	wantAuthors := []string{"bob", "carol", "dave"}
	wantBodies := []string{"great", "meh", "fyi just looking"}
	for i, p := range parsed.Pushes {
		if p.Kind != wantKinds[i] {
			t.Errorf("[%d] kind = %q want %q", i, p.Kind, wantKinds[i])
		}
		if p.Author != wantAuthors[i] {
			t.Errorf("[%d] author = %q want %q", i, p.Author, wantAuthors[i])
		}
		if p.Body != wantBodies[i] {
			t.Errorf("[%d] body = %q want %q", i, p.Body, wantBodies[i])
		}
		want := pushes[i].CreatedAt
		if !p.CreatedAt.Equal(want) {
			t.Errorf("[%d] created_at = %v want %v", i, p.CreatedAt, want)
		}
	}
}

func TestFormat_OmitPushesWhenIncludePushesFalse(t *testing.T) {
	a := sampleArticle()
	pushes := samplePushes()
	out, _ := markdown.Format(a, pushes, markdown.FormatOpts{IncludePushes: false, BoardName: "Welcome"})
	if strings.Contains(out, markdown.PushesSentinel) {
		t.Errorf("pushes sentinel emitted despite IncludePushes=false:\n%s", out)
	}
	parsed, _ := markdown.Parse(out)
	if len(parsed.Pushes) != 0 {
		t.Errorf("got %d pushes, want 0", len(parsed.Pushes))
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	in := "just a body\nwith two lines\n"
	parsed, err := markdown.Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Title != "" {
		t.Errorf("title: %q want empty", parsed.Title)
	}
	if parsed.Body != "just a body\nwith two lines" {
		t.Errorf("body: %q", parsed.Body)
	}
}

func TestParse_BodyContainingTripleDash(t *testing.T) {
	in := "---\ntitle: T\n---\n\nhead\n---\nmid\n---\ntail\n"
	parsed, err := markdown.Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Title != "T" {
		t.Errorf("title: %q", parsed.Title)
	}
	wantBody := "head\n---\nmid\n---\ntail"
	if parsed.Body != wantBody {
		t.Errorf("body:\n%q\nwant:\n%q", parsed.Body, wantBody)
	}
}

func TestParse_UnknownFrontmatterKey(t *testing.T) {
	in := "---\ntitle: T\nweird_field: weird_value\n---\n\nbody"
	parsed, _ := markdown.Parse(in)
	if parsed.Extra["weird_field"] != "weird_value" {
		t.Errorf("extra: %v", parsed.Extra)
	}
}

func TestParse_BodyContainingPushesSentinel(t *testing.T) {
	a := sampleArticle()
	a.Body = "Look at this:\n" + markdown.PushesSentinel + "\nthat is a literal in the body\n"
	out, err := markdown.Format(a, nil, markdown.FormatOpts{BoardName: "Welcome"})
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	parsed, err := markdown.Parse(out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(parsed.Body, markdown.PushesSentinel) {
		t.Errorf("literal sentinel lost in round-trip:\n%s", parsed.Body)
	}
	if len(parsed.Pushes) != 0 {
		t.Errorf("body sentinel mistakenly parsed as pushes header: %d pushes", len(parsed.Pushes))
	}
}

func TestParse_PushesOnly_NoFrontmatter(t *testing.T) {
	in := markdown.PushesSentinel + "\n- 推 [alice] 2026-04-30T12:00:00Z  hi\n"
	parsed, _ := markdown.Parse(in)
	if parsed.Body != "" {
		t.Errorf("body should be empty, got %q", parsed.Body)
	}
	if len(parsed.Pushes) != 1 || parsed.Pushes[0].Body != "hi" {
		t.Errorf("pushes: %+v", parsed.Pushes)
	}
}

func TestParse_TitleWithColon(t *testing.T) {
	// Frontmatter parsing must split on the FIRST colon only.
	in := "---\ntitle: Welcome: A Tour\nboard: Welcome\n---\n\nbody"
	parsed, _ := markdown.Parse(in)
	if parsed.Title != "Welcome: A Tour" {
		t.Errorf("title: %q", parsed.Title)
	}
}

func TestFormat_NilArticle(t *testing.T) {
	if _, err := markdown.Format(nil, nil, markdown.FormatOpts{}); err == nil {
		t.Fatal("expected error for nil article")
	}
}

func TestParse_PushesWithSingleSpaceFallback(t *testing.T) {
	// Hand-edited file might not have the canonical 2-space separator.
	in := markdown.PushesSentinel + "\n- 推 [alice] 2026-04-30T12:00:00Z hi\n"
	parsed, _ := markdown.Parse(in)
	if len(parsed.Pushes) != 1 {
		t.Fatalf("pushes: %+v", parsed.Pushes)
	}
	got := parsed.Pushes[0]
	if got.Body != "hi" || got.Author != "alice" {
		t.Errorf("got %+v want body=hi author=alice", got)
	}
	want := mustParseTime(t, "2026-04-30T12:00:00Z")
	if !got.CreatedAt.Equal(want) {
		t.Errorf("ts: %v want %v", got.CreatedAt, want)
	}
}
