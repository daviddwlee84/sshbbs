package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// Store wraps the *sql.DB plus a process-level write mutex used for
// multi-statement writes that need to be atomic at the application level
// (e.g. inserting a push and recomputing the cached recommend_score).
// SQLite WAL handles concurrent reads; this mutex avoids SQLITE_BUSY under
// modest contention without spreading sql.Tx everywhere.
type Store struct {
	db      *sql.DB
	writeMu sync.Mutex

	users          *UserRepo
	boards         *BoardRepo
	articles       *ArticleRepo
	pushes         *PushRepo
	waterBalloons  *WaterBalloonRepo
	mail           *MailRepo
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := apply(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	s := &Store{db: db}
	s.users = &UserRepo{s: s}
	s.boards = &BoardRepo{s: s}
	s.articles = &ArticleRepo{s: s}
	s.pushes = &PushRepo{s: s}
	s.waterBalloons = &WaterBalloonRepo{s: s}
	s.mail = &MailRepo{s: s}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB exposes the raw handle for callers that need it (tests, ad-hoc SQL).
// Production code should go through repos.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Users() *UserRepo                 { return s.users }
func (s *Store) Boards() *BoardRepo               { return s.boards }
func (s *Store) Articles() *ArticleRepo           { return s.articles }
func (s *Store) Pushes() *PushRepo                { return s.pushes }
func (s *Store) WaterBalloons() *WaterBalloonRepo { return s.waterBalloons }
func (s *Store) Mail() *MailRepo                  { return s.mail }
