package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	return &app{db: db, sessionSecret: "test", events: make(chan map[string]any, 4), loginAttempts: make(map[string]attempt)}
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
