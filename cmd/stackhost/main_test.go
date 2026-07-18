package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	eventhub "github.com/StackLab-Digital/StackHost/internal/events"
	"github.com/StackLab-Digital/StackHost/internal/migrations"
	"github.com/StackLab-Digital/StackHost/internal/secure"
)

func testApp(t *testing.T) *app {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := migrations.Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cipher, _ := secure.New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	return &app{db: db, sessionSecret: "test", cipher: cipher, events: eventhub.New(4), loginAttempts: make(map[string]attempt)}
}

func TestOnboardingAndSession(t *testing.T) {
	a := testApp(t)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/setup/admin", strings.NewReader(`{"name":"Admin","email":"admin@example.com","password":"password123"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.setupAdmin(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status = %d", w.Code)
	}
	if len(w.Result().Cookies()) != 1 {
		t.Fatal("expected session cookie")
	}
	w = httptest.NewRecorder()
	a.setupAdmin(w, httptest.NewRequest(http.MethodPost, "/api/v1/setup/admin", strings.NewReader(`{"name":"Other","email":"other@example.com","password":"password123"}`)))
	if w.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d", w.Code)
	}
}

func TestMeUsesLowercaseJSONFields(t *testing.T) {
	a := testApp(t)
	_, _ = a.db.Exec("INSERT INTO users(id,name,email,password_hash,role,created_at,updated_at) VALUES(1,'Admin','admin@example.com','hash','admin','now','now')")
	w := httptest.NewRecorder()
	a.me(w, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil).WithContext(context.WithValue(context.Background(), userKey{}, int64(1))))
	var payload map[string]any
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "name", "email", "role"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing lowercase field %q: %#v", key, payload)
		}
	}
	if _, ok := payload["Name"]; ok {
		t.Fatalf("unexpected uppercase fields: %#v", payload)
	}
}

func TestProjectAndApplicationPersistence(t *testing.T) {
	a := testApp(t)
	setup := httptest.NewRecorder()
	a.setupAdmin(setup, httptest.NewRequest(http.MethodPost, "/api/v1/setup/admin", strings.NewReader(`{"name":"Admin","email":"admin@example.com","password":"password123"}`)))
	cookie := setup.Result().Cookies()[0]
	create := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Website"}`))
	create.AddCookie(cookie)
	w := httptest.NewRecorder()
	a.auth(a.projects)(w, create)
	if w.Code != http.StatusOK {
		t.Fatalf("project status = %d", w.Code)
	}
	createApp := httptest.NewRequest(http.MethodPost, "/api/v1/projects/1/applications", strings.NewReader(`{"name":"Frontend","source_type":"image"}`))
	createApp.AddCookie(cookie)
	w = httptest.NewRecorder()
	a.auth(a.projectRoute)(w, createApp)
	if w.Code != http.StatusOK {
		t.Fatalf("application status = %d", w.Code)
	}
	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	listReq.AddCookie(cookie)
	a.auth(a.projects)(list, listReq)
	var projects []map[string]any
	if err := json.NewDecoder(list.Body).Decode(&projects); err != nil || len(projects) != 1 {
		t.Fatalf("projects list = %#v, err=%v", projects, err)
	}
}

func TestProjectsHandlesQueryFailure(t *testing.T) {
	a := testApp(t)
	_ = a.db.Close()
	w := httptest.NewRecorder()
	a.projects(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), `"code":"internal_error"`) {
		t.Fatalf("projects failure = %d %s", w.Code, w.Body.String())
	}
}

func TestApplicationCreationPersistsRuntimeAndConfigurationState(t *testing.T) {
	a := testApp(t)
	_, _ = a.db.Exec("INSERT INTO projects(id,name,slug,status,created_at,updated_at) VALUES(1,'Demo','demo','active','now','now')")
	tests := []struct {
		name            string
		applicationName string
		body            string
		configuration   string
		revision        int
		hasConfiguredAt bool
	}{
		{name: "configured", applicationName: "Web", body: `{"name":"Web","source_type":"compose","source":{"compose_yaml":"services:\n  web:\n    image: nginx:alpine"}}`, configuration: "configured", revision: 1, hasConfiguredAt: true},
		{name: "draft", applicationName: "Draft", body: `{"name":"Draft","source_type":"compose","save_as_draft":true,"source":{}}`, configuration: "draft", revision: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			a.applications(w, httptest.NewRequest(http.MethodPost, "/api/v1/projects/1/applications", strings.NewReader(test.body)), 1)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"not_deployed"`) {
				t.Fatalf("create application = %d %s", w.Code, w.Body.String())
			}
			var status, configuration string
			var configuredAt, validatedAt sql.NullString
			var revision int
			if err := a.db.QueryRow("SELECT status,configuration_status,source_revision,configured_at,last_validated_at FROM applications WHERE name=?", test.applicationName).Scan(&status, &configuration, &revision, &configuredAt, &validatedAt); err != nil {
				t.Fatal(err)
			}
			if status != "not_deployed" || configuration != test.configuration || revision != test.revision || configuredAt.Valid != test.hasConfiguredAt || validatedAt.Valid != test.hasConfiguredAt {
				t.Fatalf("persisted state = status:%s configuration:%s revision:%d configured:%v validated:%v", status, configuration, revision, configuredAt.Valid, validatedAt.Valid)
			}
		})
	}
}

func TestApplicationListSeparatesRuntimeAndConfigurationState(t *testing.T) {
	a := testApp(t)
	_, _ = a.db.Exec("INSERT INTO projects(id,name,slug,status,created_at,updated_at) VALUES(1,'Demo','demo','active','now','now')")
	_, _ = a.db.Exec("INSERT INTO applications(id,project_id,name,slug,source_type,status,configuration_status,created_at,updated_at) VALUES(1,1,'Web','web','compose','not_deployed','configured','now','now')")
	w := httptest.NewRecorder()
	a.applications(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/applications", nil), 1)
	var items []struct {
		Status              string `json:"status"`
		ConfigurationStatus string `json:"configuration_status"`
	}
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil || len(items) != 1 {
		t.Fatalf("applications = %#v, err=%v", items, err)
	}
	if items[0].Status != "not_deployed" || items[0].ConfigurationStatus != "configured" {
		t.Fatalf("application states = %#v", items[0])
	}
}

func TestApplicationDuplicationPreservesEncryptedSource(t *testing.T) {
	a := testApp(t)
	_, _ = a.db.Exec("INSERT INTO users(id,name,email,password_hash,role,created_at,updated_at) VALUES(1,'Admin','admin@example.com','hash','admin','now','now')")
	_, _ = a.db.Exec("INSERT INTO projects(id,name,slug,status,created_at,updated_at) VALUES(1,'Demo','demo','active','now','now')")
	_, _ = a.db.Exec("INSERT INTO applications(id,project_id,name,description,slug,source_type,status,configuration_status,source_revision,created_at,updated_at) VALUES(1,1,'Web','demo app','web','compose','not_deployed','configured',2,'now','now')")
	_, _ = a.db.Exec("INSERT INTO application_sources(application_id,source_type,encrypted_payload,encryption_nonce,payload_version,checksum,validation_status,validation_errors,validation_warnings,summary_json,created_at,updated_at) VALUES(1,'compose',X'01',X'02',1,'checksum','configured','[]','[]','{}','now','now')")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/applications/1/duplicate", nil).WithContext(context.WithValue(context.Background(), userKey{}, int64(1)))
	w := httptest.NewRecorder()
	a.applicationRouteV2(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("duplicate status = %d %s", w.Code, w.Body.String())
	}
	var newID int64
	if err := a.db.QueryRow("SELECT id FROM applications WHERE id<>1").Scan(&newID); err != nil {
		t.Fatal(err)
	}
	var sourceRevision int
	var payload []byte
	if err := a.db.QueryRow("SELECT source_revision FROM applications WHERE id=?", newID).Scan(&sourceRevision); err != nil || sourceRevision != 2 {
		t.Fatalf("copied revision = %d, err=%v", sourceRevision, err)
	}
	if err := a.db.QueryRow("SELECT encrypted_payload FROM application_sources WHERE application_id=?", newID).Scan(&payload); err != nil || len(payload) != 1 || payload[0] != 1 {
		t.Fatalf("copied source = %x, err=%v", payload, err)
	}
}

func TestLoginInvalidAndLogout(t *testing.T) {
	a := testApp(t)
	setup := httptest.NewRecorder()
	a.setupAdmin(setup, httptest.NewRequest(http.MethodPost, "/api/v1/setup/admin", strings.NewReader(`{"name":"Admin","email":"admin@example.com","password":"password123"}`)))
	bad := httptest.NewRecorder()
	a.login(bad, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"admin@example.com","password":"wrong"}`)))
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("invalid login status = %d", bad.Code)
	}
	cookie := setup.Result().Cookies()[0]
	logout := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(cookie)
	a.logout(logout, req)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d", logout.Code)
	}
	var logoutEvents int
	if err := a.db.QueryRow("SELECT count(*) FROM audit_logs WHERE action='logout'").Scan(&logoutEvents); err != nil || logoutEvents != 1 {
		t.Fatalf("logout audit count = %d, err=%v", logoutEvents, err)
	}
	me := httptest.NewRecorder()
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(cookie)
	a.auth(a.me)(me, meReq)
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", me.Code)
	}
}

func TestHealthEndpoints(t *testing.T) {
	a := testApp(t)
	for _, path := range []string{"/health/live", "/health/ready"} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, w.Code)
		}
	}
}

func TestReadinessRejectsMigrationGap(t *testing.T) {
	a := testApp(t)
	if _, err := a.db.Exec("DELETE FROM schema_migrations WHERE version=2"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.ready(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	var payload map[string]string
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Content-Type") != "application/json" || payload["status"] != "not_ready" || payload["reason"] != "migrations" {
		t.Fatalf("readiness = %d %#v %#v", w.Code, w.Header(), payload)
	}
}

func TestInfrastructureWithoutDocker(t *testing.T) {
	a := testApp(t)
	a.docker = nil
	w := httptest.NewRecorder()
	a.infrastructure(w, httptest.NewRequest(http.MethodGet, "/api/v1/infrastructure", nil))
	var payload map[string]any
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["available"] != false || payload["message"] == nil {
		t.Fatalf("docker fallback = %#v", payload)
	}
}

func TestValidationErrorIncludesFields(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()
	a.setupAdmin(w, httptest.NewRequest(http.MethodPost, "/api/v1/setup/admin", strings.NewReader(`{"name":"","email":"bad","password":"short"}`)))
	var payload struct {
		Error struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusUnprocessableEntity || payload.Error.Code != "validation_failed" || len(payload.Error.Fields) != 3 {
		t.Fatalf("validation payload = %#v", payload)
	}
}

func TestCleanDatabaseIntegrationFlow(t *testing.T) {
	a := testApp(t)
	setup := httptest.NewRecorder()
	a.setupAdmin(setup, httptest.NewRequest(http.MethodPost, "/api/v1/setup/admin", strings.NewReader(`{"name":"Admin","email":"admin@example.com","password":"password123"}`)))
	cookie := setup.Result().Cookies()[0]
	project := httptest.NewRecorder()
	projectReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"Production"}`))
	projectReq.AddCookie(cookie)
	a.auth(a.projects)(project, projectReq)
	if project.Code != http.StatusOK {
		t.Fatalf("integration project status = %d", project.Code)
	}
	application := httptest.NewRecorder()
	applicationReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/1/applications", strings.NewReader(`{"name":"API","source_type":"compose"}`))
	applicationReq.AddCookie(cookie)
	a.auth(a.projectRoute)(application, applicationReq)
	if application.Code != http.StatusOK {
		t.Fatalf("integration application status = %d", application.Code)
	}
	dashboard := httptest.NewRecorder()
	dashboardReq := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	dashboardReq.AddCookie(cookie)
	a.auth(a.dashboard)(dashboard, dashboardReq)
	var payload struct {
		Projects     int            `json:"projects"`
		Applications map[string]int `json:"applications"`
		Activity     []any          `json:"activity"`
	}
	if err := json.NewDecoder(dashboard.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Projects != 1 || payload.Applications["total"] != 1 || len(payload.Activity) == 0 {
		t.Fatalf("dashboard payload = %#v", payload)
	}
}

func TestInfrastructureInitRequiresAdmin(t *testing.T) {
	a := testApp(t)
	result, err := a.db.Exec("INSERT INTO users(name,email,password_hash,role,created_at,updated_at) VALUES(?,?,?,?,?,?)", "User", "user@example.com", "hash", "member", "now", "now")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/infrastructure/swarm/init", strings.NewReader(`{}`)).WithContext(context.WithValue(context.Background(), userKey{}, id))
	w := httptest.NewRecorder()
	a.infrastructureSwarmInit(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin init status = %d", w.Code)
	}
}

func TestApplicationSourceIsEncryptedAndSecretsAreMasked(t *testing.T) {
	a := testApp(t)
	_, _ = a.db.Exec("INSERT INTO users(id,name,email,password_hash,role,created_at,updated_at) VALUES(1,'Admin','admin@example.com','hash','admin','now','now')")
	_, _ = a.db.Exec("INSERT INTO projects(id,name,slug,description,status,created_at,updated_at) VALUES(1,'Demo','demo','','active','now','now')")
	_, _ = a.db.Exec("INSERT INTO applications(id,project_id,name,description,slug,source_type,status,configuration_status,created_at,updated_at) VALUES(1,1,'Web','','web','compose','draft','draft','now','now')")
	ctx := context.WithValue(context.Background(), userKey{}, int64(1))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/applications/1/source", strings.NewReader(`{"source_type":"compose","source":{"compose_yaml":"services:\n  web:\n    image: nginx:1.27-alpine","environment":[{"key":"TOKEN","value":"top-secret","secret":true}]}}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	a.applicationRouteV2(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "configured") {
		t.Fatalf("source save = %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	a.applicationRouteV2(w, httptest.NewRequest(http.MethodGet, "/api/v1/applications/1/source", nil).WithContext(ctx))
	if strings.Contains(w.Body.String(), "top-secret") || !strings.Contains(w.Body.String(), "has_value") {
		t.Fatalf("secret leaked or was not masked: %s", w.Body.String())
	}
	var encrypted string
	if err := a.db.QueryRow("SELECT hex(encrypted_payload) FROM application_sources WHERE application_id=1").Scan(&encrypted); err != nil || strings.Contains(encrypted, "top-secret") {
		t.Fatalf("encrypted payload invalid: %v", err)
	}
}

func TestChangeApplicationSourceRollsBackOnUpdateFailure(t *testing.T) {
	a := testApp(t)
	_, _ = a.db.Exec("INSERT INTO projects(id,name,slug,status,created_at,updated_at) VALUES(1,'Demo','demo','active','now','now')")
	_, _ = a.db.Exec("INSERT INTO applications(id,project_id,name,slug,source_type,status,configuration_status,source_revision,created_at,updated_at) VALUES(1,1,'Web','web','compose','not_deployed','configured',1,'now','now')")
	_, _ = a.db.Exec("INSERT INTO application_sources(application_id,source_type,encrypted_payload,encryption_nonce,checksum,validation_status,created_at,updated_at) VALUES(1,'compose',X'01',X'02','checksum','configured','now','now')")
	if _, err := a.db.Exec("CREATE TRIGGER fail_source_change BEFORE UPDATE OF source_type ON applications BEGIN SELECT RAISE(ABORT, 'forced update failure'); END"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.changeApplicationSource(w, httptest.NewRequest(http.MethodPost, "/api/v1/applications/1/source/change", strings.NewReader(`{"source_type":"image","confirm_reset":true}`)), 1)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("source change status = %d", w.Code)
	}
	var sourceType string
	var revision, sources int
	if err := a.db.QueryRow("SELECT source_type,source_revision FROM applications WHERE id=1").Scan(&sourceType, &revision); err != nil {
		t.Fatal(err)
	}
	if err := a.db.QueryRow("SELECT count(*) FROM application_sources WHERE application_id=1").Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if sourceType != "compose" || revision != 1 || sources != 1 {
		t.Fatalf("source change was partially persisted: type=%s revision=%d sources=%d", sourceType, revision, sources)
	}
}

func TestSourceValidationPreview(t *testing.T) {
	a := testApp(t)
	valid := httptest.NewRecorder()
	a.validateSourcePreview(valid, httptest.NewRequest(http.MethodPost, "/api/v1/source/validate", strings.NewReader(`{"source_type":"compose","source":{"compose_yaml":"services:\n  web:\n    image: nginx:1.27-alpine"}}`)))
	if valid.Code != http.StatusOK || !strings.Contains(valid.Body.String(), `"valid":true`) {
		t.Fatalf("valid preview = %d %s", valid.Code, valid.Body.String())
	}
	invalid := validateSource("image", sourceInput{Image: "nginx", ContainerPort: -1})
	if invalid.Valid {
		t.Fatal("expected invalid image port to be rejected")
	}
}

func TestInventoryLimitIsBounded(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/infrastructure/containers?limit=999", nil)
	if got := inventoryLimit(request); got != 200 {
		t.Fatalf("maximum inventory limit = %d", got)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/infrastructure/containers?limit=0", nil)
	if got := inventoryLimit(request); got != 50 {
		t.Fatalf("default inventory limit = %d", got)
	}
}
