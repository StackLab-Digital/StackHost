package migrations

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestRunUpgradesEverySupportedVersion(t *testing.T) {
	for _, test := range []struct {
		name    string
		version int
	}{
		{name: "empty", version: 0},
		{name: "version 1", version: 1},
		{name: "version 2", version: 2},
		{name: "version 3", version: 3},
		{name: "current", version: 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openTestDB(t)
			if test.version > 0 {
				if err := run(context.Background(), db, migrations[:test.version]); err != nil {
					t.Fatalf("prepare version %d: %v", test.version, err)
				}
			}
			if err := Run(context.Background(), db); err != nil {
				t.Fatalf("run migrations: %v", err)
			}
			assertCurrentSchema(t, db)
		})
	}
}

func TestRunIsRepeatable(t *testing.T) {
	db := openTestDB(t)
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	assertCurrentSchema(t, db)
}

func TestDeploymentsMigrationNormalizesLegacyRuntimeStatus(t *testing.T) {
	db := openTestDB(t)
	if err := run(context.Background(), db, migrations[:3]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects(id,name,slug,status,created_at,updated_at) VALUES(1,'Demo','demo','active','now','now')`); err != nil {
		t.Fatal(err)
	}
	statuses := []string{"unknown", "configured", "draft", "invalid", "running", "stopped", "failed"}
	for index, status := range statuses {
		if _, err := db.Exec(`INSERT INTO applications(id,project_id,name,slug,source_type,status,created_at,updated_at) VALUES(?,1,?,?,?,?,'now','now')`, index+1, status, status, "compose", status); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for index, previous := range statuses {
		var got string
		if err := db.QueryRow(`SELECT status FROM applications WHERE id=?`, index+1).Scan(&got); err != nil {
			t.Fatal(err)
		}
		want := previous
		if previous == "unknown" || previous == "configured" || previous == "draft" || previous == "invalid" {
			want = "not_deployed"
		}
		if got != want {
			t.Fatalf("legacy status %q became %q, want %q", previous, got, want)
		}
	}
}

func TestRunRollsBackFailedMigration(t *testing.T) {
	db := openTestDB(t)
	boom := errors.New("forced failure")
	list := []Migration{
		{Version: 1, Name: "before_failure", Up: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `CREATE TABLE before_failure(id INTEGER PRIMARY KEY)`)
			return err
		}},
		{Version: 2, Name: "failure", Up: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `CREATE TABLE rolled_back(id INTEGER PRIMARY KEY)`); err != nil {
				return err
			}
			return boom
		}},
		{Version: 3, Name: "after_failure", Up: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `CREATE TABLE after_failure(id INTEGER PRIMARY KEY)`)
			return err
		}},
	}
	if err := run(context.Background(), db, list); !errors.Is(err, boom) {
		t.Fatalf("expected forced failure, got %v", err)
	}
	if !tableExists(t, db, "before_failure") {
		t.Fatal("migration before the failure should remain committed")
	}
	if tableExists(t, db, "rolled_back") || tableExists(t, db, "after_failure") {
		t.Fatal("failed migration was not rolled back or later migration ran")
	}
	assertVersions(t, db, []int{1})
}

func TestIsCurrentRequiresEveryVersion(t *testing.T) {
	db := openTestDB(t)
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=2`); err != nil {
		t.Fatal(err)
	}
	current, err := IsCurrent(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if current {
		t.Fatal("schema with a migration gap must not be current")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "stackhost.db") + "?_foreign_keys=on"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func assertCurrentSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	if Latest() != 8 {
		t.Fatalf("latest version = %d", Latest())
	}
	current, err := IsCurrent(context.Background(), db)
	if err != nil || !current {
		t.Fatalf("current = %v, err = %v", current, err)
	}
	assertVersions(t, db, []int{1, 2, 3, 4, 5, 6, 7, 8})
	for _, table := range []string{"users", "application_sources", "environment_settings", "deployments", "application_domains", "backups", "runtime_metrics", "notifications"} {
		if !tableExists(t, db, table) {
			t.Fatalf("missing table %s", table)
		}
	}
	var mode string
	if err := db.QueryRow(`SELECT runtime_mode FROM environment_settings WHERE id=1`).Scan(&mode); err != nil || mode != "standalone" {
		t.Fatalf("runtime mode = %q, err = %v", mode, err)
	}
	var indexSQL string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='index' AND name='idx_deployments_application_created'`).Scan(&indexSQL); err != nil || indexSQL == "" {
		t.Fatalf("deployments index missing: %q, err = %v", indexSQL, err)
	}
}

func assertVersions(t *testing.T, db *sql.DB, want []int) {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			t.Fatal(err)
		}
		got = append(got, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count == 1
}
