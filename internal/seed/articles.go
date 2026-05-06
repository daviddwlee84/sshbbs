// Package seed loads default content (currently: welcome / intro articles)
// from embedded markdown files into the database on first startup.
//
// The contract is intentionally weak: a board's emptiness is sampled once
// at the start of each seed run. If the board was empty, EVERY matching
// embedded file is inserted (so welcome.md + welcome-rules.md can co-seed
// the same fresh board). If the board was non-empty, no files are inserted
// — an admin who edited the seeded welcome article does not see their
// changes overwritten, nor see new seed files appear behind them, on the
// next restart.
//
// To add more seed articles, drop another *.md file under internal/seed/articles/
// with frontmatter naming the target board (and optionally `pinned: true`
// to mark it as a 板規 / 置頂 article):
//
//	---
//	title: ...
//	board: Welcome
//	pinned: true   # optional, defaults to false
//	---
//
//	body...
//
// `go:embed` will pick it up at the next build.
package seed

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"

	"github.com/daviddwlee84/sshbbs/internal/markdown"
	"github.com/daviddwlee84/sshbbs/internal/store"
)

//go:embed articles/*.md
var articlesFS embed.FS

// Articles parses every embedded *.md file and inserts it into the board
// named by its frontmatter, attributing all of them to adminUserID.
//
// Idempotency: emptiness is sampled ONCE per board at the start of this
// run. A board that was empty at sample time receives ALL its matching
// seed files (so welcome.md + welcome-rules.md can co-seed the Welcome
// board); a board that was non-empty at sample time is skipped entirely
// (so an admin who edited the seeded content doesn't see new files appear
// behind them on the next restart).
//
// Failure to parse or insert any single file logs and continues — a bad
// seed file should not block startup. Missing boards are also skipped
// with a log line.
func Articles(ctx context.Context, st *store.Store, adminUserID string) error {
	admin, err := st.Users().GetByUserID(ctx, adminUserID)
	if err != nil {
		return fmt.Errorf("seed.Articles: lookup admin %q: %w", adminUserID, err)
	}

	entries, err := fs.ReadDir(articlesFS, "articles")
	if err != nil {
		return fmt.Errorf("seed.Articles: read embedded fs: %w", err)
	}
	// Stable iteration order so logs are deterministic.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	// Cache "has-articles?" per board so several seed files targeting the
	// same board only hit the DB once.
	boardEmpty := map[int64]bool{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		body, err := articlesFS.ReadFile("articles/" + e.Name())
		if err != nil {
			log.Printf("seed.Articles: read %s: %v", e.Name(), err)
			continue
		}
		parsed, err := markdown.Parse(string(body))
		if err != nil {
			log.Printf("seed.Articles: parse %s: %v", e.Name(), err)
			continue
		}
		if parsed.BoardName == "" {
			log.Printf("seed.Articles: %s has no `board:` frontmatter, skipping", e.Name())
			continue
		}
		if parsed.Title == "" {
			log.Printf("seed.Articles: %s has no `title:` frontmatter, skipping", e.Name())
			continue
		}

		board, err := st.Boards().GetByName(ctx, parsed.BoardName)
		if err != nil {
			if errors.Is(err, store.ErrBoardNotFound) {
				log.Printf("seed.Articles: %s targets board %q which doesn't exist; skipping",
					e.Name(), parsed.BoardName)
				continue
			}
			return fmt.Errorf("seed.Articles: lookup board %q: %w", parsed.BoardName, err)
		}

		empty, cached := boardEmpty[board.ID]
		if !cached {
			existing, err := st.Articles().ListByBoard(ctx, board.ID, 1)
			if err != nil {
				return fmt.Errorf("seed.Articles: list %q: %w", parsed.BoardName, err)
			}
			empty = len(existing) == 0
			boardEmpty[board.ID] = empty
		}
		if !empty {
			continue
		}

		art, err := st.Articles().Create(ctx, board.ID, admin.ID, admin.UserID, parsed.Title, parsed.Body)
		if err != nil {
			log.Printf("seed.Articles: insert %s: %v", e.Name(), err)
			continue
		}
		// `pinned: true` frontmatter → second write to flip pinned_at. Two
		// statements (Create + SetPinned) are fine here: both go through
		// writeMu, so the article briefly exists unpinned during seed and
		// then is pinned. No reader can observe the gap because seed runs
		// before the SSH listener accepts connections.
		if parsed.Pinned {
			if err := st.Articles().SetPinned(ctx, art.ID, admin.ID, admin.Role, true); err != nil {
				log.Printf("seed.Articles: pin %s: %v", e.Name(), err)
				// Best-effort: leave the article seeded even if the pin failed.
			}
		}
		// Note: do NOT flip boardEmpty[board.ID] here. The cache must keep
		// reflecting the START-of-run state so multiple seed files targeting
		// the same fresh board can co-seed. The board's "seeded" state is
		// re-evaluated only on the next process restart.
		log.Printf("seed.Articles: seeded %s into board %q (pinned=%v)", e.Name(), parsed.BoardName, parsed.Pinned)
	}
	return nil
}
