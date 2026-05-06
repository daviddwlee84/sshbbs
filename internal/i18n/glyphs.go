package i18n

import "github.com/daviddwlee84/sshbbs/internal/store"

// Glyph helpers. These are the locale-dependent UI substitutions: same
// concept, different glyph per locale, with display width preserved so
// column-aligned tables (board view, article push list) don't shift.
//
// CRITICAL: these functions are TUI-only. Do not use them when writing
// to disk — markdown export goes through store.PushKind.Glyph(), which
// is canonical zh-TW so internal/markdown.parsePushLine round-trips.
//
// Width parity (each pair must measure the same under
// github.com/mattn/go-runewidth, asserted by glyphs_test.go):
//   推  (2)  ↔ 👍 (2)
//   噓  (2)  ↔ 👎 (2)
//   →   (1)  ↔  →  (1)   — universal arrow, kept as-is
//   爆  (2)  ↔ 💥 (2)
//   [鎖] (4) ↔ [L] (3)   — prefix, not column-aligned; 1-cell shrink OK
//   [箭] (4) ↔ [A] (3)   — same
//   [M]  (3) ↔ [M] (3)   — universal pin marker, no swap

// PushGlyph returns the locale-appropriate glyph for a push kind, unstyled.
// Caller wraps with lipgloss styles in the TUI render path so this package
// stays free of UI dependencies.
func PushGlyph(loc Locale, k store.PushKind) string {
	if loc == LocaleEN {
		switch k {
		case store.PushKindPush:
			return "👍"
		case store.PushKindBoo:
			return "👎"
		case store.PushKindArrow:
			return "→"
		}
	}
	// Default + zh-TW + unknown → canonical CJK glyph from the store
	// layer. Keeping this delegation means the canonical-glyph invariant
	// has a single source of truth.
	return k.Glyph()
}

// PushVerb returns the glyph used inside webhook notification titles
// (e.g. "[BBS] alice 推了你的文章" / "[BBS] alice 👍 your post"). Currently
// identical to PushGlyph — kept as a separate function so a future change
// to en mode (e.g. spelled-out "upvoted"/"downvoted") doesn't need to
// touch every render call site.
func PushVerb(loc Locale, k store.PushKind) string {
	return PushGlyph(loc, k)
}

// ScoreExploded returns the glyph shown in the board-view score column
// when an article's recommend_score crosses the threshold (≥ 100, see
// internal/tui/screen_board_view.go). Empty string is never returned —
// callers gate on the threshold themselves.
func ScoreExploded(loc Locale) string {
	if loc == LocaleEN {
		return "💥"
	}
	return "爆"
}

// CommentsModeBadge returns the bracketed comments-mode marker shown as
// a prefix on board-list rows and inline in the article header. Returns
// the empty string for CommentsModeOpen (the default — no badge).
func CommentsModeBadge(loc Locale, m store.CommentsMode) string {
	switch m {
	case store.CommentsModeLocked:
		if loc == LocaleEN {
			return "[L]"
		}
		return "[鎖]"
	case store.CommentsModeArrowsOnly:
		if loc == LocaleEN {
			return "[A]"
		}
		return "[箭]"
	}
	return ""
}
