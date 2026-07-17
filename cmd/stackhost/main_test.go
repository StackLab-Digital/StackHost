package main

import (
	"database/sql"
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
	return &app{db: db, sessionSecret: "test", events: make(chan map[string]any, 4)}
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
}
