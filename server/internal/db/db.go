// Package db opens SQLite with the mandatory PRAGMAs (§4.0, §7.1) and runs
// versioned migrations. Two pools are used: a single-connection WRITE pool
// (serializes all writes — Go is the sole writer) and a multi-connection READ
// pool (WAL allows concurrent readers).
package db

import (
	"context"
	"database/sql"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"
)

// pragmas are applied on every connection via the DSN. foreign_keys and
// busy_timeout are per-connection (default OFF / 0); journal_mode=WAL persists
// at the database level once set.
const pragmas = "_pragma=foreign_keys(1)" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(1)"

// DB bundles the write and read handles.
type DB struct {
	Write *sql.DB
	Read  *sql.DB
	path  string
}

// Open opens (and migrates) the database at path.
func Open(path string) (*DB, error) {
	dsn := path + "?" + pragmas

	w, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1) // serialize writes — root out "database is locked" (§7.1)
	w.SetConnMaxIdleTime(time.Hour)
	if err := w.Ping(); err != nil {
		_ = w.Close()
		return nil, err
	}

	r, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = w.Close()
		return nil, err
	}
	r.SetMaxOpenConns(4) // concurrent readers under WAL
	if err := r.Ping(); err != nil {
		_ = w.Close()
		_ = r.Close()
		return nil, err
	}

	d := &DB{Write: w, Read: r, path: path}
	if err := Migrate(context.Background(), w); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

// Close closes both handles.
func (d *DB) Close() error {
	var firstErr error
	if d.Read != nil {
		if err := d.Read.Close(); err != nil {
			firstErr = err
		}
	}
	if d.Write != nil {
		if err := d.Write.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Path returns the database file path.
func (d *DB) Path() string { return d.path }

// Checkpoint truncates the WAL to reclaim space (§7.1 maintenance). Best run at
// low load or after an ingest batch.
func (d *DB) Checkpoint(ctx context.Context) error {
	_, err := d.Write.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}
