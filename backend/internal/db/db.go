package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Open returns a *sql.DB connected to the SQLite file at dsn.
//
// dsn accepts either a bare path ("ck.db", "/var/lib/compliancekit/ck.db")
// or a full SQLite URL ("file:ck.db?_foo=bar"). The following pragmas are
// applied on every new connection via a connect-hook URL:
//
//	journal_mode=WAL        // concurrent readers + single writer without locking
//	synchronous=NORMAL      // safe under WAL, much faster than FULL
//	foreign_keys=ON         // SQLite disables FK enforcement unless asked
//	busy_timeout=5000       // block up to 5s on writer contention before erroring
//	temp_store=MEMORY       // keep temp btrees off the filesystem
//
// The pool is tuned for SQLite's single-writer model: one open connection
// for writes, a handful for reads. Writes serialize inside the DB anyway;
// a large pool would only manufacture `SQLITE_BUSY`.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	pragmas := []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=foreign_keys(ON)",
		"_pragma=busy_timeout(5000)",
		"_pragma=temp_store(MEMORY)",
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	full := dsn + sep + strings.Join(pragmas, "&")

	pool, err := sql.Open("sqlite", full)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(4)
	pool.SetConnMaxIdleTime(5 * time.Minute)
	pool.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// Tx runs fn inside a transaction. Commits on nil error, rolls back on error or panic.
func Tx(ctx context.Context, pool *sql.DB, fn func(*sql.Tx) error) (err error) {
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		if cerr := tx.Commit(); cerr != nil {
			err = fmt.Errorf("db: commit: %w", cerr)
		}
	}()
	if err = fn(tx); err != nil {
		return fmt.Errorf("db: tx fn: %w", err)
	}
	return nil
}
