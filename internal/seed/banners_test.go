package seed_test

import (
	"context"
	"strings"
	"testing"

	"github.com/daviddwlee84/sshbbs/internal/seed"
	"github.com/daviddwlee84/sshbbs/internal/store"
	"github.com/daviddwlee84/sshbbs/internal/store/storetest"
)

func TestBanners_SeedsEmptyBoards(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	admin := mustAdmin(t, st)

	if err := seed.Banners(ctx, st, admin); err != nil {
		t.Fatalf("seed.Banners: %v", err)
	}

	for _, name := range []string{"Welcome", "Test", "ChitChat"} {
		t.Run(name, func(t *testing.T) {
			b, err := st.Boards().GetByName(ctx, name)
			if err != nil {
				t.Fatalf("GetByName: %v", err)
			}
			if b.Banner == "" {
				t.Errorf("board %q got empty banner; want seeded content", name)
			}
		})
	}
}

func TestBanners_LeavesEditedBannersAlone(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	admin := mustAdmin(t, st)
	adminUser, _ := st.Users().GetByUserID(ctx, admin)

	welcome, _ := st.Boards().GetByName(ctx, "Welcome")
	const userBanner = "[user-edited banner]"
	if err := st.Boards().UpdateBanner(ctx, welcome.ID, adminUser.ID, store.RoleAdmin, userBanner); err != nil {
		t.Fatalf("manual UpdateBanner: %v", err)
	}

	if err := seed.Banners(ctx, st, admin); err != nil {
		t.Fatalf("seed.Banners: %v", err)
	}
	got, _ := st.Boards().GetByName(ctx, "Welcome")
	if got.Banner != userBanner {
		t.Errorf("Banner = %q, want %q (seed must skip non-empty)", got.Banner, userBanner)
	}
}

func TestBanners_IdempotentOverRestarts(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	admin := mustAdmin(t, st)

	for i := 0; i < 3; i++ {
		if err := seed.Banners(ctx, st, admin); err != nil {
			t.Fatalf("run #%d: %v", i, err)
		}
	}
	// First run wrote, subsequent runs no-op'd. Banner should match the
	// embedded Welcome.txt (which we know contains the marker below).
	w, _ := st.Boards().GetByName(ctx, "Welcome")
	if !strings.Contains(w.Banner, "Welcome to SSH-BBS") {
		t.Errorf("Welcome banner missing seeded marker; got %q", w.Banner)
	}
}

func TestBanners_FallbackUsedForUnseededBoard(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	admin := mustAdmin(t, st)
	adminUser, _ := st.Users().GetByUserID(ctx, admin)

	// Insert a brand-new board with no per-board seed file. SeedDefaults
	// won't include it since it only knows the three defaults — we add it
	// directly with a raw INSERT to keep the test minimal.
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO boards (name, title) VALUES ('Random', 'random')`); err != nil {
		t.Fatalf("insert random board: %v", err)
	}

	if err := seed.Banners(ctx, st, admin); err != nil {
		t.Fatalf("seed.Banners: %v", err)
	}
	got, _ := st.Boards().GetByName(ctx, "Random")
	if got.Banner == "" {
		t.Fatalf("Random board got empty banner; expected fallback")
	}
	// Fallback file (default.txt) carries the "BBS" marker. If the test
	// breaks because the seed file content was tweaked, update both
	// here and the file in lockstep.
	if !strings.Contains(got.Banner, "BBS") {
		t.Errorf("expected fallback marker in banner, got %q", got.Banner)
	}
	_ = adminUser // silence unused-variable lint when test is trimmed
}

func TestBanners_AdminMissing(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := st.Boards().SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	if err := seed.Banners(ctx, st, "admin"); err == nil {
		t.Error("expected error when admin user missing")
	}
}
