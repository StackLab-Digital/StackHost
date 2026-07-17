package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StackLab-Digital/StackHost/internal/secure"
)

func testApp(t *testing.T) *app {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cipher, _ := secure.New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	return &app{db: db, sessionSecret: "test", cipher: cipher, events: make(chan map[string]any, 4), loginAttempts: make(map[string]attempt)}
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
		Projects     int   `json:"projects"`
		Applications int   `json:"applications"`
		Activity     []any `json:"activity"`
	}
	if err := json.NewDecoder(dashboard.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Projects != 1 || payload.Applications != 1 || len(payload.Activity) == 0 {
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
