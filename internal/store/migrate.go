package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// apply runs every migration whose version number isn't already in
// schema_migrations. Filenames must be NNNN_name.sql; the leading integer is
// the version. Migrations are applied in ascending version order, each in
// its own transaction.
func apply(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := loadApplied(ctx, db)
	if err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	type migration struct {
		version  int
		filename string
	}
	var pending []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v, err := parseVersion(e.Name())
		if err != nil {
			return err
		}
		if !applied[v] {
			pending = append(pending, migration{version: v, filename: e.Name()})
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].version < pending[j].version })

	for _, m := range pending {
		body, err := migrationsFS.ReadFile("migrations/" + m.filename)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", m.filename, err)
		}
		if err := runOne(ctx, db, m.version, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", m.filename, err)
		}
	}
	return nil
}

func loadApplied(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("load applied migrations: %w", err)
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func runOne(ctx context.Context, db *sql.DB, version int, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, version); err != nil {
		return err
	}
	return tx.Commit()
}

func parseVersion(filename string) (int, error) {
	// "0001_init.sql" -> 1
	idx := strings.IndexByte(filename, '_')
	if idx <= 0 {
		return 0, fmt.Errorf("migration filename missing version prefix: %s", filename)
	}
	v, err := strconv.Atoi(filename[:idx])
	if err != nil {
		return 0, fmt.Errorf("parse version in %s: %w", filename, err)
	}
	return v, nil
}
