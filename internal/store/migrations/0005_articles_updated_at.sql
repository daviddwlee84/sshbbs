-- Track edits to articles. NULL = never edited; the original CreatedAt is the
-- only authoritative timestamp. The article-edit screen and ArticleRepo.Update
-- set this to CURRENT_TIMESTAMP on every successful update.
--
-- Ordering: ListByBoard intentionally still sorts by created_at, so an edited
-- article does NOT bubble up to the top — bookmark/next-prev semantics stay
-- intact.

ALTER TABLE articles ADD COLUMN updated_at DATETIME;
