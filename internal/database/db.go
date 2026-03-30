package database

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite connection.
type DB struct {
	sql *sql.DB
}

// Open opens (or creates) the SQLite database at path and configures it.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite only supports one writer; cap the pool.
	sqlDB.SetMaxOpenConns(1)

	db := &DB{sql: sqlDB}
	if err := db.applyPragmas(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (d *DB) applyPragmas() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := d.sql.Exec(p); err != nil {
			return fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	return nil
}

// Migrate creates all tables, triggers, and indexes.
func (d *DB) Migrate() error {
	stmts := []string{
		createLanes,
		createCards,
		createCardsTrigger,
		createCardsIndex,
	}
	for _, stmt := range stmts {
		if _, err := d.sql.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.sql.Close()
}
