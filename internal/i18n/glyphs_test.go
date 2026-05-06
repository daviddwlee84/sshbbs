package i18n

import (
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

// TestPushGlyph_WidthParity guards the en-mode emoji swap: 推→👍, 噓→👎,
// →→→, 爆→💥. Each pair must measure the same display width under
// runewidth, otherwise the board-list and article-view push columns
// shift by one cell when a user toggles locale. Catches a future
// "improvement" to a 1-cell ASCII glyph that would silently break
// the table layout.
func TestPushGlyph_WidthParity(t *testing.T) {
	for _, k := range []store.PushKind{
		store.PushKindPush,
		store.PushKindBoo,
		store.PushKindArrow,
	} {
		zh := PushGlyph(LocaleZHTW, k)
		en := PushGlyph(LocaleEN, k)
		zw := runewidth.StringWidth(zh)
		ew := runewidth.StringWidth(en)
		if zw != ew {
			t.Errorf("kind %q: zh=%q (w=%d) vs en=%q (w=%d) — widths must match for column alignment",
				k, zh, zw, en, ew)
		}
	}
}

func TestScoreExploded_WidthParity(t *testing.T) {
	zh := ScoreExploded(LocaleZHTW)
	en := ScoreExploded(LocaleEN)
	zw := runewidth.StringWidth(zh)
	ew := runewidth.StringWidth(en)
	if zw != ew {
		t.Errorf("ScoreExploded: zh=%q (w=%d) vs en=%q (w=%d) — widths must match", zh, zw, en, ew)
	}
	if zw != 2 {
		t.Errorf("ScoreExploded zh-TW width = %d, want 2 (column reserves 2 cells for this glyph)", zw)
	}
}

// Canonical zh-TW glyph delegation: PushGlyph(zh, k) must equal
// store.PushKind.Glyph(). Locks in the round-trip invariant — if anyone
// edits PushGlyph and breaks this, markdown export silently changes too.
func TestPushGlyph_ZHIsCanonical(t *testing.T) {
	for _, k := range []store.PushKind{
		store.PushKindPush,
		store.PushKindBoo,
		store.PushKindArrow,
	} {
		got := PushGlyph(LocaleZHTW, k)
		want := k.Glyph()
		if got != want {
			t.Errorf("kind %q: PushGlyph(zh) = %q, store.Glyph() = %q — must match for canonical round-trip", k, got, want)
		}
	}
}

func TestCommentsModeBadge(t *testing.T) {
	cases := []struct {
		mode   store.CommentsMode
		zh, en string
	}{
		{store.CommentsModeOpen, "", ""},
		{store.CommentsModeLocked, "[鎖]", "[L]"},
		{store.CommentsModeArrowsOnly, "[箭]", "[A]"},
	}
	for _, tc := range cases {
		if got := CommentsModeBadge(LocaleZHTW, tc.mode); got != tc.zh {
			t.Errorf("CommentsModeBadge(zh, %q) = %q, want %q", tc.mode, got, tc.zh)
		}
		if got := CommentsModeBadge(LocaleEN, tc.mode); got != tc.en {
			t.Errorf("CommentsModeBadge(en, %q) = %q, want %q", tc.mode, got, tc.en)
		}
	}
}
