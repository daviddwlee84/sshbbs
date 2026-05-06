package seed

import (
	"context"
	"embed"
	"fmt"
	"log"

	"github.com/daviddwlee84/sshbbs/internal/store"
)

//go:embed banners/*.txt
var bannersFS embed.FS

// defaultBannerFilename is the per-banner fallback used for any board that
// has no per-board file under internal/seed/banners/. The plan calls this
// "_default", but go:embed's wildcard rules around leading-underscore
// filenames are subtle, so we use a plain name. It cannot collide with a
// real board because the default seed boards are Welcome/Test/ChitChat.
const defaultBannerFilename = "default.txt"

// Banners scans every board and writes boards.banner from
// internal/seed/banners/<Name>.txt where the column is empty. Falls back
// to default.txt for boards without a per-board file. Idempotent: once a
// banner is non-empty it's left alone, so admin/mod edits survive
// restarts (same contract as seed.Articles, but keyed on the single
// `banner` column rather than "board has any article").
//
// Banners are written with a synthetic admin identity so UpdateBanner's
// permission gate passes. Unknown admins log+return; per-board read or
// write failures log+continue so one bad seed file doesn't block startup.
func Banners(ctx context.Context, st *store.Store, adminUserID string) error {
	admin, err := st.Users().GetByUserID(ctx, adminUserID)
	if err != nil {
		return fmt.Errorf("seed.Banners: lookup admin %q: %w", adminUserID, err)
	}
	boards, err := st.Boards().List(ctx)
	if err != nil {
		return fmt.Errorf("seed.Banners: list boards: %w", err)
	}

	// Read the fallback once. nil means "no fallback shipped"; boards
	// without a per-board file are then left alone.
	defaultBody, _ := bannersFS.ReadFile("banners/" + defaultBannerFilename)

	for _, board := range boards {
		if board.Banner != "" {
			continue
		}
		body, err := bannersFS.ReadFile("banners/" + board.Name + ".txt")
		if err != nil {
			if defaultBody == nil {
				continue
			}
			body = defaultBody
		}
		if err := st.Boards().UpdateBanner(ctx, board.ID, admin.ID, store.RoleAdmin, string(body)); err != nil {
			log.Printf("seed.Banners: update %q: %v", board.Name, err)
			continue
		}
		log.Printf("seed.Banners: seeded banner for board %q", board.Name)
	}
	return nil
}
