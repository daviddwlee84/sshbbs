-- Per-board banner: ANSI/ASCII-art text rendered above the article list and
-- in a full-screen splash. NULL = unseeded; the runtime seed
-- (internal/seed.Banners) fills NULL columns from embedded files on every
-- startup, mirroring the boards-default seed pattern. Once non-NULL, the
-- value belongs to whoever last edited it (admin / mod) and is never
-- overwritten — same idempotency contract as the welcome article.

ALTER TABLE boards ADD COLUMN banner TEXT;
