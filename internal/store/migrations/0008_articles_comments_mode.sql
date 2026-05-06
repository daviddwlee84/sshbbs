-- Per-article comment moderation. 'open' (default) lets anyone non-guest
-- push/boo/arrow as before. 'arrows_only' blocks 推/噓 but still lets users
-- leave neutral 箭頭 follow-ups — useful for 板規 / FAQ articles that should
-- not become ranking targets but where clarifying comments are still welcome.
-- 'locked' rejects all three kinds outright.
--
-- States are mutually exclusive ('locked' is strictly stronger than
-- 'arrows_only'); the CHECK constraint plus a single-column enum keeps the
-- precedence rule unambiguous. Mod-only mutator: ArticleRepo.SetCommentsMode
-- mirrors SetPinned's permission shape (role >= mod, author NOT admitted).
--
-- Existing pushes are NOT retroactively deleted when a mode tightens — the
-- gate only blocks new pushes — so recommend_score stays stable across mode
-- flips. No index: the column is read inline with article rows during
-- PushRepo.Create's existing tx (writeMu held).

ALTER TABLE articles ADD COLUMN comments_mode TEXT NOT NULL DEFAULT 'open'
    CHECK (comments_mode IN ('open', 'arrows_only', 'locked'));
