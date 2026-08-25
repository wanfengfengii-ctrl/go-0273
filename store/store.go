// Package store provides the SQLite-backed persistence layer shared by every
// business package. It opens and migrates the database, wraps transactions,
// and implements the Store interfaces declared by the catalog, task, sample,
// measure, and pathogen packages against a single schema. Startup recovery
// re-reads open tasks, active leases and pending device calls so that a
// restarted process resumes deterministically from the database.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB is a thin wrapper around *sql.DB that owns the schema and provides
// transactional helpers used by all concrete Store implementations.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at path, applies pragmas that
// enable foreign-key enforcement and serializable write behavior, and runs
// migrations. Pass ":memory:" for an in-memory database used by tests.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single connection keeps :memory: and transaction semantics stable for
	// tests; WAL is enabled for file-backed databases.
	if _, err := conn.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if path != ":memory:" {
		if _, err := conn.Exec(`PRAGMA journal_mode = WAL`); err != nil {
			conn.Close()
			return nil, fmt.Errorf("enable wal: %w", err)
		}
	}
	db := &DB{DB: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

// Close closes the underlying database.
func (db *DB) Close() error { return db.DB.Close() }

// Tx runs fn inside a single database transaction, committing on success and
// rolling back on any error. It is the unit of atomicity for every business
// write: no partial samples, leases, cells, reveals, evidence, or decisions
// can survive a failed transaction.
func (db *DB) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Commit()
		return err
	}
	return tx.Commit()
}
