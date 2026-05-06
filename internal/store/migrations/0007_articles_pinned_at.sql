-- Pin articles as 板規 (board rules) / important announcements. NULL = unpinned;
-- non-NULL = the UTC timestamp at which a mod+ pinned the article via
-- ArticleRepo.SetPinned. Multi-pin is supported intrinsically: any number of
-- rows on a board can hold a non-NULL pinned_at.
--
-- Ordering: ListByBoard sorts pinned-first then by created_at DESC. We do
-- NOT model an explicit pin-order column this round — admins reorder pins by
-- un/re-pinning (the latest pin gets the newest pinned_at, but display order
-- is still anchored to created_at within the pinned group; if that proves
-- limiting, a follow-up can add `pin_order INTEGER` and a swap operation).
--
-- Idempotency contract mirrors the welcome-article seed: a board with any
-- existing article is skipped entirely (see internal/seed/articles.go), so
-- pinned-by-default seed files only apply to fresh boards.

ALTER TABLE articles ADD COLUMN pinned_at DATETIME;

CREATE INDEX idx_articles_board_pinned ON articles(board_id, pinned_at);
