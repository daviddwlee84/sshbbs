# article_revisions snapshot table for 板規 amendment history (法律增修條文)

**Status**: P3 — deferred, not currently scheduled
**Effort**: L
**Related**: `TODO.md`, `internal/store/articles.go`, `internal/seed/articles/welcome-rules.md`, the 2026-05-06 pin-articles ship entry

## Context

The 2026-05-06 "置頂文章 / 板規" ship gave us multi-pin articles per board (mod-toggleable, seed-configurable). One board's pinned 板規 article serves as its rulebook; admins amend the rules over time by editing the article body. The user surfaced an explicit interest in modeling this as **法律增修條文** — Taiwan's pattern of preserving old wordings of a law and tracking which amendment changed which clause.

We shipped a **light convention** instead of building this:

- The seed `internal/seed/articles/welcome-rules.md` includes a `## 修訂紀錄` section listing `vX.Y (YYYY-MM-DD) — 變更摘要`.
- Admins edit the pinned article via the existing `E` shortcut and add a new entry to that section.
- No schema, no extra UI; relies on `articles.updated_at` for the most-recent timestamp and on `git log` of the seed file for the long-tail history of admin-edited content.

This works for low-traffic 板規 churn but has known gaps:

1. **No diff** — readers can't see what wording changed between v1.0 and v1.1; only the summary the admin chose to write.
2. **No who** — `updated_at` records *when* but not *who* edited (the article author stays the original admin even if a different mod amended).
3. **No audit trail** — a malicious mod can rewrite history by editing the `## 修訂紀錄` section itself; nothing in the DB stops it.

## Investigation

Not started. The convention has only existed since 2026-05-06; no real-world drift between rule versions yet.

## Options considered

| Option | Pros | Cons |
|---|---|---|
| A. New `article_revisions(article_id, version, title, body, edited_by, edited_at)` table; `ArticleRepo.Update` writes a snapshot on every edit of a *pinned* article | Diff-able; auditable; bounded growth (only pinned articles snapshot); reuses existing edit flow | New table; new index; new TUI screen to view history (otherwise data is unreachable); `Update` becomes two writes under `writeMu` |
| B. Same table, but snapshot **all** article edits, not just pinned | Uniform; no `IF pinned THEN snapshot` branch | Storage growth scales with edit traffic; recommendation push-edits would dominate; mostly noise for non-規 articles |
| C. `git`-as-history: serialize each pinned-article edit to an on-disk markdown file under `data/rules-history/<board>/<article>/<rev>.md`; commit each | Zero schema change; uses `markdown.Format` round-trip we already have | Adds a `data/` directory dependency; backups become more complex; `git` from inside the BBS process is a new failure mode |
| D. Stay with the markdown convention, accept the gaps | Zero work; matches user's "看看是不是有什麼規則可以定一下" exploratory framing | The three gaps above remain |

Likely answer if/when this reactivates: **A** with a hard scope of pinned articles only, plus a new mod-only "view history" screen reachable from the article view (e.g., shift+H). Storage cost is bounded because the only frequent edit-target is the rulebook itself, which is small.

## Open questions

- Should mods be able to *delete* old revisions (compliance with right-to-be-forgotten requests) or are revisions immutable once written?
- Does the history view need its own ANSI render or can it reuse the article-view glamour pipeline per-revision?
- If a 板規 is unpinned, do we keep its prior revisions or garbage-collect them? (Probably keep — unpinning is a moderation action and the trail still has audit value.)

## Triggers to reactivate

Promote out of P3 when **any** of these become real:

- An admin/mod actually edits a 板規 in production and someone asks "what did the old version say?"
- A user disputes a moderation decision and the answer hinges on which version of the rule was in force at the violation time.
- The light `## 修訂紀錄` convention drifts (mods stop adding entries, or contradicting entries appear) — that's the signal the convention can't bear the load alone.

Until then, the 修訂紀錄 markdown section in `welcome-rules.md` is the canonical pattern; agents who add new pinned 板規 should mirror it.
