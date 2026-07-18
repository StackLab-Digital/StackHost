package deployment

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/StackLab-Digital/StackHost/internal/migrations"
	_ "github.com/mattn/go-sqlite3"
)

func deploymentTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "deployments.db")+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := migrations.Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO projects(id,name,slug,status,created_at,updated_at) VALUES(1,'Demo','demo','active','now','now')`); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 4; id++ {
		if _, err := db.Exec(`INSERT INTO applications(id,project_id,name,slug,source_type,status,created_at,updated_at) VALUES(?,1,?,?,?,'not_deployed','now','now')`, id, "App", "app"+string(rune('0'+id)), "compose"); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSQLStoreCreateUpdateGetAndList(t *testing.T) {
	db := deploymentTestDB(t)
	store := NewSQLStore(db)
	first, err := store.Create(context.Background(), NewDeployment{ApplicationID: 1, RuntimeMode: RuntimeStandalone, SourceRevision: 2, StackName: "demo", TriggerType: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(context.Background(), NewDeployment{ApplicationID: 1, RuntimeMode: RuntimeSwarm, SourceRevision: 3, StackName: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	started, finished := time.Now().UTC().Add(-time.Second), time.Now().UTC()
	if err := store.Update(context.Background(), first.ID, DeploymentUpdate{Status: StatusSucceeded, Output: "published", StartedAt: &started, FinishedAt: &finished}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusSucceeded || got.Output != "published" || got.StartedAt == nil || got.FinishedAt == nil || got.TriggerType != "manual" {
		t.Fatalf("unexpected deployment: %#v", got)
	}
	items, err := store.ListByApplication(context.Background(), 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != second.ID || items[1].ID != first.ID || items[0].TriggerType != "manual" {
		t.Fatalf("unexpected list: %#v", items)
	}
	if _, err := store.Get(context.Background(), 999); err != ErrNotFound {
		t.Fatalf("missing deployment error = %v", err)
	}
}

func TestSQLStoreRecoversEveryTransientStatus(t *testing.T) {
	db := deploymentTestDB(t)
	store := NewSQLStore(db)
	statuses := []Status{StatusQueued, StatusPreparing, StatusDeploying, StatusWaiting, StatusSucceeded}
	ids := make([]int64, 0, len(statuses))
	for _, status := range statuses {
		item, err := store.Create(context.Background(), NewDeployment{ApplicationID: 1, RuntimeMode: RuntimeStandalone, SourceRevision: 1, StackName: "demo"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
		if status != StatusQueued {
			if err := store.Update(context.Background(), item.ID, DeploymentUpdate{Status: status}); err != nil {
				t.Fatal(err)
			}
		}
	}
	count, err := store.InterruptTransient(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("recovered = %d, want 4", count)
	}
	for index, id := range ids {
		item, err := store.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		want := StatusInterrupted
		if statuses[index] == StatusSucceeded {
			want = StatusSucceeded
		}
		if item.Status != want {
			t.Fatalf("status %s recovered as %s", statuses[index], item.Status)
		}
		if want == StatusInterrupted && (item.FinishedAt == nil || item.ErrorCode != "stackhost_restarted") {
			t.Fatalf("interrupted metadata missing: %#v", item)
		}
	}
}

func TestSQLStoreDeploymentCascadesWithApplication(t *testing.T) {
	db := deploymentTestDB(t)
	store := NewSQLStore(db)
	item, err := store.Create(context.Background(), NewDeployment{ApplicationID: 1, RuntimeMode: RuntimeStandalone, SourceRevision: 1, StackName: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM applications WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), item.ID); err != ErrNotFound {
		t.Fatalf("deployment survived application deletion: %v", err)
	}
}
