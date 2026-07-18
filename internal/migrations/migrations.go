package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Migration is one atomic database schema change.
type Migration struct {
	Version int
	Name    string
	Up      func(context.Context, *sql.Tx) error
}

var migrations = []Migration{
	{Version: 1, Name: "initial_schema", Up: initialSchema},
	{Version: 2, Name: "application_sources", Up: applicationSources},
	{Version: 3, Name: "environment_settings", Up: environmentSettings},
	{Version: 4, Name: "deployments", Up: deployments},
	{Version: 5, Name: "application_domains", Up: applicationDomains},
	{Version: 6, Name: "backups", Up: backups},
	{Version: 7, Name: "runtime_metrics", Up: runtimeMetrics},
	{Version: 8, Name: "notifications", Up: notifications},
	{Version: 9, Name: "backup_settings", Up: backupSettings},
}

// Run applies every pending migration in version order.
func Run(ctx context.Context, db *sql.DB) error {
	return run(ctx, db, migrations)
}

// Latest returns the schema version expected by this binary.
func Latest() int {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}

// IsCurrent reports whether every migration expected by this binary is applied.
func IsCurrent(ctx context.Context, db *sql.DB) (bool, error) {
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return false, err
	}
	for version := range applied {
		if version < 1 || version > Latest() {
			return false, fmt.Errorf("unknown schema migration version %d", version)
		}
	}
	for _, migration := range migrations {
		if !applied[migration.Version] {
			return false, nil
		}
	}
	return true, nil
}

func run(ctx context.Context, db *sql.DB, list []Migration) error {
	if err := validate(list); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}
	latest := list[len(list)-1].Version
	for version := range applied {
		if version < 1 || version > latest {
			return fmt.Errorf("database schema version %d is not supported by this binary", version)
		}
	}
	for _, migration := range list {
		if applied[migration.Version] {
			continue
		}
		if err := apply(ctx, db, migration); err != nil {
			return err
		}
		slog.Info("database migration applied", "version", migration.Version, "name", migration.Name)
	}
	return nil
}

func validate(list []Migration) error {
	if len(list) == 0 {
		return fmt.Errorf("no database migrations configured")
	}
	for i, migration := range list {
		if migration.Version != i+1 {
			return fmt.Errorf("database migration version %d is out of order", migration.Version)
		}
		if migration.Name == "" || migration.Up == nil {
			return fmt.Errorf("database migration version %d is incomplete", migration.Version)
		}
	}
	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema_migrations: %w", err)
	}
	return applied, nil
}

func apply(ctx context.Context, db *sql.DB, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %d %s: begin: %w", migration.Version, migration.Name, err)
	}
	defer tx.Rollback()
	if err := migration.Up(ctx, tx); err != nil {
		return fmt.Errorf("migration %d %s: %w", migration.Version, migration.Name, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, migration.Version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("migration %d %s: record: %w", migration.Version, migration.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %d %s: commit: %w", migration.Version, migration.Name, err)
	}
	return nil
}

func initialSchema(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, role TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, last_login_at TEXT);
		CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, token_hash TEXT NOT NULL, expires_at TEXT NOT NULL, created_at TEXT NOT NULL, last_seen_at TEXT NOT NULL, revoked_at TEXT, user_agent TEXT, ip_address TEXT);
		CREATE TABLE IF NOT EXISTS projects(id INTEGER PRIMARY KEY, name TEXT NOT NULL, slug TEXT UNIQUE NOT NULL, description TEXT, status TEXT NOT NULL DEFAULT 'active', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS applications(id INTEGER PRIMARY KEY, project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE, name TEXT NOT NULL, slug TEXT NOT NULL, source_type TEXT NOT NULL, docker_stack_name TEXT, status TEXT NOT NULL DEFAULT 'unknown', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS audit_logs(id INTEGER PRIMARY KEY, user_id INTEGER REFERENCES users(id), action TEXT NOT NULL, resource_type TEXT, resource_id TEXT, metadata TEXT, ip_address TEXT, created_at TEXT NOT NULL);
		CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token_hash);
		CREATE INDEX IF NOT EXISTS idx_apps_project ON applications(project_id);
	`)
	return err
}

func applicationSources(ctx context.Context, tx *sql.Tx) error {
	columns := []struct {
		name       string
		definition string
	}{
		{"description", "TEXT NOT NULL DEFAULT ''"},
		{"configuration_status", "TEXT NOT NULL DEFAULT 'draft'"},
		{"configured_at", "TEXT"},
		{"last_validated_at", "TEXT"},
		{"source_revision", "INTEGER NOT NULL DEFAULT 0"},
	}
	for _, column := range columns {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pragma_table_info('applications') WHERE name=?)`, column.name).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			if _, err := tx.ExecContext(ctx, "ALTER TABLE applications ADD COLUMN "+column.name+" "+column.definition); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE applications SET configuration_status='draft' WHERE status='unknown';
		CREATE TABLE IF NOT EXISTS application_sources(
			id INTEGER PRIMARY KEY,
			application_id INTEGER NOT NULL UNIQUE REFERENCES applications(id) ON DELETE CASCADE,
			source_type TEXT NOT NULL,
			encrypted_payload BLOB NOT NULL,
			encryption_nonce BLOB NOT NULL,
			payload_version INTEGER NOT NULL DEFAULT 1,
			checksum TEXT NOT NULL,
			validation_status TEXT NOT NULL DEFAULT 'draft',
			validation_errors TEXT NOT NULL DEFAULT '[]',
			validation_warnings TEXT NOT NULL DEFAULT '[]',
			summary_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_application_sources_application ON application_sources(application_id);
	`)
	return err
}

func environmentSettings(ctx context.Context, tx *sql.Tx) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS environment_settings(id INTEGER PRIMARY KEY, runtime_mode TEXT NOT NULL DEFAULT 'standalone', updated_at TEXT NOT NULL);
		INSERT OR IGNORE INTO environment_settings(id, runtime_mode, updated_at) VALUES(1, 'standalone', ?);
	`, now)
	return err
}

func deployments(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE applications
		SET status='not_deployed'
		WHERE status IN ('unknown','configured','draft','invalid');
		CREATE TABLE IF NOT EXISTS deployments (
			id INTEGER PRIMARY KEY,
			application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
			runtime_mode TEXT NOT NULL,
			status TEXT NOT NULL,
			source_revision INTEGER NOT NULL,
			stack_name TEXT NOT NULL,
			trigger_type TEXT NOT NULL DEFAULT 'manual',
			output TEXT NOT NULL DEFAULT '',
			error_code TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_deployments_application_created
		ON deployments(application_id, created_at DESC);
	`)
	return err
}

func applicationDomains(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS application_domains (
			id INTEGER PRIMARY KEY,
			application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
			hostname TEXT NOT NULL UNIQUE,
			service_name TEXT NOT NULL,
			target_port INTEGER NOT NULL,
			primary_domain INTEGER NOT NULL DEFAULT 0,
			https_enabled INTEGER NOT NULL DEFAULT 1,
			redirect_https INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'pending',
			certificate_status TEXT NOT NULL DEFAULT 'pending',
			last_error TEXT NOT NULL DEFAULT '',
			last_checked_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_domains_application ON application_domains(application_id);
	`)
	return err
}

func backups(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS backups (
			id INTEGER PRIMARY KEY,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			path TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			error_message TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_backups_created ON backups(created_at DESC);
	`)
	return err
}

func runtimeMetrics(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS runtime_metrics (
			id INTEGER PRIMARY KEY,
			application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
			service_name TEXT NOT NULL,
			cpu_percent REAL NOT NULL DEFAULT 0,
			memory_bytes INTEGER NOT NULL DEFAULT 0,
			memory_limit_bytes INTEGER NOT NULL DEFAULT 0,
			network_rx_bytes INTEGER NOT NULL DEFAULT 0,
			network_tx_bytes INTEGER NOT NULL DEFAULT 0,
			recorded_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_runtime_metrics_application_recorded
		ON runtime_metrics(application_id, recorded_at DESC);
	`)
	return err
}

func notifications(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS notifications (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			kind TEXT NOT NULL,
			encrypted_url BLOB NOT NULL,
			encryption_nonce BLOB NOT NULL,
			events TEXT NOT NULL DEFAULT '[]',
			enabled INTEGER NOT NULL DEFAULT 1,
			last_status TEXT NOT NULL DEFAULT 'never',
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	return err
}

func backupSettings(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS backup_settings (
			id INTEGER PRIMARY KEY CHECK(id=1),
			schedule TEXT NOT NULL DEFAULT 'manual',
			retention INTEGER NOT NULL DEFAULT 7,
			updated_at TEXT NOT NULL
		);
		INSERT OR IGNORE INTO backup_settings(id,schedule,retention,updated_at) VALUES(1,'manual',7,?);
	`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
