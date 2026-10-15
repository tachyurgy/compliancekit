// Package migrate applies the embedded *.up.sql files in order and records
// each one in a schema_migrations table, so boot-time migration is
// idempotent. It replaces the external golang-migrate CLI the project used
// while it was on Postgres: with SQLite and a single binary there is no
// separate migration host, so the binary migrates itself.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
)

// Up applies every *.up.sql file in fsys that is not yet recorded in
// schema_migrations. Each file runs inside its own transaction together with
// the bookkeeping insert, so a failed migration leaves no partial state.
// Returns the number of migrations applied.
func Up(ctx context.Context, db *sql.DB, fsys fs.FS, log *slog.Logger) (int, error) {
	if log == nil {
		log = slog.Default()
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
		    version    TEXT PRIMARY KEY,
		    applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		return 0, fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return 0, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return 0, fmt.Errorf("migrate: scan: %w", err)
		}
		applied[v] = true
	}
	rows.Close()

	// Guard against pointing the binary at a database that was migrated by
	// the old external tool: tables exist but nothing is recorded here.
	if len(applied) == 0 {
		var n int
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='providers'`).Scan(&n)
		if n > 0 {
			return 0, fmt.Errorf("migrate: database has tables but no schema_migrations rows; refusing to re-apply (baseline it first)")
		}
	}

	names, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return 0, fmt.Errorf("migrate: glob: %w", err)
	}
	// Lexicographic order == numeric order for zero-padded names
	// (000026_ sorts before 000026b_ because '_' < 'b').
	sort.Strings(names)

	count := 0
	for _, name := range names {
		version := strings.TrimSuffix(name, ".up.sql")
		if applied[version] {
			continue
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return count, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		if err := applyOne(ctx, db, version, string(body)); err != nil {
			return count, err
		}
		log.Info("migration applied", "version", version)
		count++
	}
	return count, nil
}

func applyOne(ctx context.Context, db *sql.DB, version, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate: begin %s: %w", version, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("migrate: apply %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
		return fmt.Errorf("migrate: record %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate: commit %s: %w", version, err)
	}
	return nil
}
