package tui

import (
	"strings"
	"testing"
)

func TestStripCSI_RemovesSGR(t *testing.T) {
	got := stripCSI("\x1b[31mABC\x1b[0m")
	if got != "ABC" {
		t.Errorf("got %q, want %q", got, "ABC")
	}
}

func TestStripCSI_NoEscape_PassThrough(t *testing.T) {
	got := stripCSI("plain ascii")
	if got != "plain ascii" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestStripCSI_MultipleRuns(t *testing.T) {
	got := stripCSI("\x1b[31mR\x1b[0m\x1b[32mG\x1b[0m\x1b[34mB\x1b[0m")
	if got != "RGB" {
		t.Errorf("got %q, want RGB", got)
	}
}

func TestBannerVisualWidth_ASCII(t *testing.T) {
	if w := bannerVisualWidth("hello"); w != 5 {
		t.Errorf("hello: got %d, want 5", w)
	}
}

func TestBannerVisualWidth_CJKDoubleWidth(t *testing.T) {
	if w := bannerVisualWidth("\x1b[31m中文\x1b[0m"); w != 4 {
		t.Errorf("color+CJK: got %d, want 4 (CSI excluded, CJK each width 2)", w)
	}
}

func TestBannerVisualWidth_OnlyCSI(t *testing.T) {
	if w := bannerVisualWidth("\x1b[31m\x1b[0m"); w != 0 {
		t.Errorf("only CSI: got %d, want 0", w)
	}
}

func TestBannerTruncate_FitsExactly(t *testing.T) {
	got := bannerTruncate("hello", 5)
	if got != "hello" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestBannerTruncate_FitsWithSlack(t *testing.T) {
	got := bannerTruncate("hi", 10)
	if got != "hi" {
		t.Errorf("got %q, want %q", got, "hi")
	}
}

func TestBannerTruncate_ASCIIOverflow(t *testing.T) {
	got := bannerTruncate("ABCDEFGHIJ", 5)
	// 4 visible chars + "…" = 5 total.
	if got != "ABCD…" {
		t.Errorf("got %q, want ABCD…", got)
	}
}

func TestBannerTruncate_PreservesCSIAndAddsReset(t *testing.T) {
	got := bannerTruncate("\x1b[31mABCDEFGHIJ\x1b[0m", 5)
	// Walks: CSI red, then "ABCD" (4 chars), then "…".
	// Output already has the embedded reset since we pass through CSI runs;
	// since the reset is the LAST thing in the source, it appears after
	// where the budget exhausts — so it's not included in output. We must
	// add a defensive reset to prevent color leak.
	if !strings.HasPrefix(got, "\x1b[31m") {
		t.Errorf("missing leading red CSI; got %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("missing trailing reset; got %q", got)
	}
	if !strings.Contains(got, "ABCD…") {
		t.Errorf("missing visible body 'ABCD…'; got %q", got)
	}
}

func TestBannerTruncate_FitsWithCSI_ForcesReset(t *testing.T) {
	// Banner that fits exactly but author forgot to reset. We add one so
	// the next line of UI isn't tinted.
	got := bannerTruncate("\x1b[31mABC", 10)
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("expected forced reset; got %q", got)
	}
}

func TestBannerTruncate_FitsWithCSI_KeepsExistingReset(t *testing.T) {
	// Already ends with reset — we must not double it.
	got := bannerTruncate("\x1b[31mABC\x1b[0m", 10)
	if strings.HasSuffix(got, "\x1b[0m\x1b[0m") {
		t.Errorf("doubled reset; got %q", got)
	}
}

func TestBannerTruncate_CJKBudget(t *testing.T) {
	// "中文" = 4 cells. Budget 5 → fits all (visual width 4 ≤ 5).
	if got := bannerTruncate("中文", 5); got != "中文" {
		t.Errorf("CJK fit: got %q, want 中文", got)
	}
	// Budget 3 → must truncate. Reserve 1 for ellipsis → budget 2 → 1 CJK char fits (width 2). Then "…".
	if got := bannerTruncate("中文", 3); got != "中…" {
		t.Errorf("CJK truncate: got %q, want 中…", got)
	}
}

func TestBannerTruncate_ZeroWidthReturnsEmpty(t *testing.T) {
	if got := bannerTruncate("anything", 0); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
