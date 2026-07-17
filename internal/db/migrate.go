package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// exported so sqlite.go can access it
var migrations = []struct {
	Version int
	Name    string
	SQL     []string
}{
	{
		Version: 1,
		Name:    "initial_schema",
		SQL: []string{
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
		},
	},
	{
		Version: 2,
		Name:    "add_cf_cfg_ip_data_ssh_keys",
		SQL: []string{
			`CREATE TABLE IF NOT EXISTS cf_configs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				token TEXT NOT NULL DEFAULT '',
				email TEXT NOT NULL DEFAULT '',
				api_key TEXT NOT NULL DEFAULT '',
				zone_id TEXT NOT NULL DEFAULT '',
				enabled INTEGER NOT NULL DEFAULT 1,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE TABLE IF NOT EXISTS ip_data (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				tenant_id INTEGER REFERENCES tenants(id),
				cidr TEXT NOT NULL DEFAULT '',
				label TEXT NOT NULL DEFAULT '',
				type TEXT NOT NULL DEFAULT 'pool',
				enabled INTEGER NOT NULL DEFAULT 1,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE TABLE IF NOT EXISTS ssh_keys (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL DEFAULT '',
				public_key TEXT NOT NULL DEFAULT '',
				private_key TEXT NOT NULL DEFAULT '',
				fingerprint TEXT NOT NULL DEFAULT '',
				tenant_id INTEGER REFERENCES tenants(id),
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE TABLE IF NOT EXISTS instance_plans (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				tenant_id INTEGER REFERENCES tenants(id),
				shape TEXT NOT NULL DEFAULT '',
				image_id TEXT NOT NULL DEFAULT '',
				subnet_id TEXT NOT NULL DEFAULT '',
				availability_domain TEXT NOT NULL DEFAULT '',
				boot_volume_size_gb INTEGER NOT NULL DEFAULT 50,
				ocpus REAL NOT NULL DEFAULT 1,
				memory_gb REAL NOT NULL DEFAULT 1,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE TABLE IF NOT EXISTS users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				username TEXT NOT NULL UNIQUE,
				password_hash TEXT NOT NULL DEFAULT '',
				role TEXT NOT NULL DEFAULT 'user',
				mfa_enabled INTEGER NOT NULL DEFAULT 0,
				mfa_secret TEXT NOT NULL DEFAULT '',
				email TEXT NOT NULL DEFAULT '',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
		},
	},
	{
		Version: 3,
		Name:    "add_ip_data_geolocation",
		SQL: []string{
			`ALTER TABLE ip_data ADD COLUMN lat REAL NOT NULL DEFAULT 0`,
			`ALTER TABLE ip_data ADD COLUMN lng REAL NOT NULL DEFAULT 0`,
			`ALTER TABLE ip_data ADD COLUMN country TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE ip_data ADD COLUMN area TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE ip_data ADD COLUMN city TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE ip_data ADD COLUMN org TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE ip_data ADD COLUMN asn TEXT NOT NULL DEFAULT ''`,
		},
	},
	{
		Version: 4,
		Name:    "add_instance_region",
		SQL: []string{
			`ALTER TABLE instances ADD COLUMN region TEXT NOT NULL DEFAULT ''`,
		},
	},
}

func (s *Store) runMigrations() error {
	if _, err := s.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("pragma: %w", err)
	}

	// ensure schema_version table exists
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	// get current version
	var currentVersion int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&currentVersion); err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}

	// run pending migrations.
	// SQLite DDL auto-commits, so transactions add no safety. Instead,
	// "duplicate column" errors from ALTER TABLE ADD COLUMN are caught
	// and skipped — handles crash recovery where DDL partially applied.
	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}
		log.Printf("[migrate] v%d: %s", m.Version, m.Name)

		for _, ddl := range m.SQL {
			if _, err := s.db.Exec(ddl); err != nil {
				if strings.Contains(err.Error(), "duplicate column") {
					log.Printf("[migrate] v%d: skipping already-applied DDL: %s", m.Version, ddl)
					continue
				}
				return fmt.Errorf("migration v%d %s: %w", m.Version, m.Name, err)
			}
		}
		if _, err := s.db.Exec(`INSERT INTO schema_version (version, name) VALUES (?, ?)`, m.Version, m.Name); err != nil {
			return fmt.Errorf("record migration v%d: %w", m.Version, err)
		}
	}
	return nil
}

// Helper: begin transaction
func (s *Store) BeginTx() (*sql.Tx, error) {
	return s.db.Begin()
}
