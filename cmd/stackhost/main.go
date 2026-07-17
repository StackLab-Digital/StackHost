package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type app struct {
	db            *sql.DB
	sessionSecret string
}

func main() {
	addr := getenv("STACKHOST_HTTP_ADDR", ":8080")
	dataDir := getenv("STACKHOST_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		panic(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dataDir, "stackhost.db?_foreign_keys=on"))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err = migrate(db); err != nil {
		panic(err)
	}
	a := &app{db: db, sessionSecret: getenv("STACKHOST_SESSION_SECRET", "development-only-change-me")}
	s := &http.Server{Addr: addr, Handler: a.routes(), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		slog.Info("StackHost listening", "addr", addr)
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server stopped", "error", err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Shutdown(shutdown)
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func migrate(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, role TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, last_login_at TEXT); CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, token_hash TEXT NOT NULL, expires_at TEXT NOT NULL, created_at TEXT NOT NULL, last_seen_at TEXT NOT NULL, revoked_at TEXT, user_agent TEXT, ip_address TEXT); CREATE TABLE IF NOT EXISTS projects(id INTEGER PRIMARY KEY, name TEXT NOT NULL, slug TEXT UNIQUE NOT NULL, description TEXT, status TEXT NOT NULL DEFAULT 'active', created_at TEXT NOT NULL, updated_at TEXT NOT NULL); CREATE TABLE IF NOT EXISTS applications(id INTEGER PRIMARY KEY, project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE, name TEXT NOT NULL, slug TEXT NOT NULL, source_type TEXT NOT NULL, docker_stack_name TEXT, status TEXT NOT NULL DEFAULT 'unknown', created_at TEXT NOT NULL, updated_at TEXT NOT NULL); CREATE TABLE IF NOT EXISTS audit_logs(id INTEGER PRIMARY KEY, user_id INTEGER REFERENCES users(id), action TEXT NOT NULL, resource_type TEXT, resource_id TEXT, metadata TEXT, ip_address TEXT, created_at TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token_hash); CREATE INDEX IF NOT EXISTS idx_apps_project ON applications(project_id);`)
	return err
}
func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health/ready", a.ready)
	mux.HandleFunc("/api/v1/setup/status", a.setupStatus)
	mux.HandleFunc("/api/v1/setup/admin", a.setupAdmin)
	mux.HandleFunc("/api/v1/auth/login", a.login)
	mux.HandleFunc("/api/v1/auth/logout", a.logout)
	mux.HandleFunc("/api/v1/me", a.auth(a.me))
	mux.HandleFunc("/api/v1/dashboard", a.auth(a.dashboard))
	mux.HandleFunc("/api/v1/projects", a.auth(a.projects))
	mux.HandleFunc("/api/v1/infrastructure", a.auth(a.infrastructure))
	mux.HandleFunc("/", a.spa)
	return security(mux)
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
func (a *app) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.db.Ping(); err != nil {
		http.Error(w, `{"status":"not_ready"}`, 503)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
func jsonError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg}})
}
func (a *app) setupStatus(w http.ResponseWriter, r *http.Request) {
	var n int
	_ = a.db.QueryRow("SELECT count(*) FROM users WHERE role='admin'").Scan(&n)
	json.NewEncoder(w).Encode(map[string]bool{"needs_setup": n == 0})
}
func (a *app) setupAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var n int
	a.db.QueryRow("SELECT count(*) FROM users WHERE role='admin'").Scan(&n)
	if n > 0 {
		jsonError(w, 409, "setup_complete", "O administrador já foi criado.")
		return
	}
	var in struct{ Name, Email, Password string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" || !strings.Contains(in.Email, "@") || len(in.Password) < 8 {
		jsonError(w, 422, "validation_failed", "Revise os campos informados.")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := a.db.Exec("INSERT INTO users(name,email,password_hash,role,created_at,updated_at) VALUES(?,?,?,?,?,?)", in.Name, strings.ToLower(strings.TrimSpace(in.Email)), hash, "admin", now, now)
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível criar o administrador.")
		return
	}
	id, _ := res.LastInsertId()
	a.db.Exec("INSERT INTO audit_logs(user_id,action,resource_type,resource_id,created_at) VALUES(?,?,?,?,?)", id, "onboarding", "user", fmt.Sprint(id), now)
	a.createSession(w, r, id)
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
func (a *app) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var in struct{ Email, Password string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, 422, "validation_failed", "Revise os campos informados.")
		return
	}
	var id int64
	var hash string
	err := a.db.QueryRow("SELECT id,password_hash FROM users WHERE email=?", strings.ToLower(strings.TrimSpace(in.Email))).Scan(&id, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		jsonError(w, 401, "invalid_credentials", "E-mail ou senha inválidos.")
		return
	}
	a.db.Exec("UPDATE users SET last_login_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), id)
	a.createSession(w, r, id)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
func (a *app) createSession(w http.ResponseWriter, r *http.Request, id int64) {
	token := fmt.Sprintf("%d-%d-%s", id, time.Now().UnixNano(), a.sessionSecret)
	now := time.Now().UTC()
	a.db.Exec("INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at,last_seen_at) VALUES(?,?,?,?,?,?)", token, id, token, now.Add(24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339))
	http.SetCookie(w, &http.Cookie{Name: "stackhost_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: os.Getenv("STACKHOST_COOKIE_SECURE") == "true", MaxAge: 86400})
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("stackhost_session"); err == nil {
		a.db.Exec("UPDATE sessions SET revoked_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "stackhost_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
func (a *app) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("stackhost_session")
		if err != nil {
			jsonError(w, 401, "unauthorized", "Faça login para continuar.")
			return
		}
		var id int64
		var exp string
		err = a.db.QueryRow("SELECT user_id,expires_at FROM sessions WHERE id=? AND revoked_at IS NULL", c.Value).Scan(&id, &exp)
		if err != nil {
			jsonError(w, 401, "unauthorized", "Faça login para continuar.")
			return
		}
		t, _ := time.Parse(time.RFC3339, exp)
		if time.Now().After(t) {
			jsonError(w, 401, "unauthorized", "Sessão expirada.")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), userKey{}, id))
		next(w, r)
	}
}

type userKey struct{}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	var u struct {
		ID                int64 `json:"id"`
		Name, Email, Role string
	}
	a.db.QueryRow("SELECT id,name,email,role FROM users WHERE id=?", r.Context().Value(userKey{})).Scan(&u.ID, &u.Name, &u.Email, &u.Role)
	json.NewEncoder(w).Encode(u)
}
func (a *app) dashboard(w http.ResponseWriter, r *http.Request) {
	var projects, apps int
	a.db.QueryRow("SELECT count(*) FROM projects").Scan(&projects)
	a.db.QueryRow("SELECT count(*) FROM applications").Scan(&apps)
	json.NewEncoder(w).Encode(map[string]any{"projects": projects, "applications": apps, "infrastructure": map[string]any{"docker": "unknown", "swarm": "unknown"}})
}
func (a *app) projects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "GET" {
		rows, _ := a.db.Query("SELECT id,name,slug,coalesce(description,''),status,created_at,updated_at FROM projects ORDER BY id DESC")
		defer rows.Close()
		out := []any{}
		for rows.Next() {
			var id int
			var name, slug, desc, status, created, updated string
			rows.Scan(&id, &name, &slug, &desc, &status, &created, &updated)
			out = append(out, map[string]any{"id": id, "name": name, "slug": slug, "description": desc, "status": status, "created_at": created, "updated_at": updated})
		}
		json.NewEncoder(w).Encode(out)
		return
	}
	if r.Method != "POST" {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var in struct{ Name, Slug, Description string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
		jsonError(w, 422, "validation_failed", "O nome é obrigatório.")
		return
	}
	if in.Slug == "" {
		in.Slug = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(in.Name), " ", "-"))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := a.db.Exec("INSERT INTO projects(name,slug,description,status,created_at,updated_at) VALUES(?,?,?,?,?,?)", in.Name, in.Slug, in.Description, "active", now, now)
	if err != nil {
		jsonError(w, 409, "already_exists", "O slug já está em uso.")
		return
	}
	id, _ := res.LastInsertId()
	json.NewEncoder(w).Encode(map[string]any{"id": id, "name": in.Name, "slug": in.Slug, "status": "active"})
}
func (a *app) infrastructure(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{"docker": map[string]any{"available": false, "message": "Docker não está conectado."}, "swarm": map[string]any{"active": false, "message": "O host não está conectado a um Swarm."}, "nodes": []any{}, "services": []any{}})
}
func (a *app) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	root := getenv("STACKHOST_WEB_DIR", "./web/dist")
	path := filepath.Join(root, filepath.Clean(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err == nil {
		http.ServeFile(w, r, index)
		return
	}
	http.Error(w, "Frontend não compilado. Execute npm run build em web/.", http.StatusNotFound)
}
