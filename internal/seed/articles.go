// Package seed loads default content (currently: welcome / intro articles)
// from embedded markdown files into the database on first startup.
//
// The contract is intentionally weak: a board with any existing article is
// skipped entirely. That way an admin who edits the seeded welcome article
// does not see their changes overwritten on the next restart.
//
// To add more seed articles, drop another *.md file under internal/seed/articles/
// with frontmatter naming the target board:
//
//	---
//	title: ...
//	board: Welcome
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
// named by its frontmatter, attributing all of them to adminUserID. A
// board that already has at least one article is left alone, regardless
// of how many seed files target it.
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

		if _, err := st.Articles().Create(ctx, board.ID, admin.ID, admin.UserID, parsed.Title, parsed.Body); err != nil {
			log.Printf("seed.Articles: insert %s: %v", e.Name(), err)
			continue
		}
		// Mark this board as no-longer-empty so subsequent seed files for
		// the same board are skipped.
		boardEmpty[board.ID] = false
		log.Printf("seed.Articles: seeded %s into board %q", e.Name(), parsed.BoardName)
	}
	return nil
}
