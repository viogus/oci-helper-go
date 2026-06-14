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
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			user_ocid TEXT NOT NULL DEFAULT '',
			tenancy_ocid TEXT NOT NULL DEFAULT '',
			region TEXT NOT NULL DEFAULT '',
			fingerprint TEXT NOT NULL DEFAULT '',
			key_file TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			home_region TEXT DEFAULT '',
			subscribed TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS instances (
			id TEXT PRIMARY KEY,
			tenant_id INTEGER NOT NULL REFERENCES tenants(id),
			name TEXT NOT NULL DEFAULT '',
			ocid TEXT NOT NULL DEFAULT '',
			shape TEXT NOT NULL DEFAULT '',
			ocpu REAL NOT NULL DEFAULT 0,
			memory_gb REAL NOT NULL DEFAULT 0,
			boot_volume_gb INTEGER NOT NULL DEFAULT 0,
			public_ip TEXT NOT NULL DEFAULT '',
			private_ip TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT '',
			availability_domain TEXT NOT NULL DEFAULT '',
			fault_domain TEXT NOT NULL DEFAULT '',
			image_id TEXT NOT NULL DEFAULT '',
			subnet_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tenant_id INTEGER,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			progress INTEGER NOT NULL DEFAULT 0,
			message TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '{}',
			result TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			started_at DATETIME,
			finished_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tenant_id INTEGER,
			action TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, d := range ddl {
		if _, err := s.db.Exec(d); err != nil {
			return fmt.Errorf("ddl: %w", err)
		}
	}
	return nil
}
