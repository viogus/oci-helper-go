// Package db provides a pure-Go SQLite data layer for oci-helper.
//
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single connection avoids SQLITE_BUSY under concurrent writes and keeps
	// :memory: databases (used in tests) working correctly — in-memory DBs
	// are per-connection, so pooling would give each query a different schema.
	// WAL mode already allows concurrent reads within the single connection.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	return s.runMigrations()
}

func (s *Store) DB() *sql.DB { return s.db }
