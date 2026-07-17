package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	dockerreader "github.com/StackLab-Digital/StackHost/internal/docker"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type app struct {
	db            *sql.DB
	sessionSecret string
	events        chan map[string]any
	docker        *dockerreader.Reader
	loginMu       sync.Mutex
	loginAttempts map[string]attempt
}
type attempt struct {
	count int
	since time.Time
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
	dr, _ := dockerreader.NewReader()
	a := &app{db: db, sessionSecret: getenv("STACKHOST_SESSION_SECRET", "development-only-change-me"), events: make(chan map[string]any, 32), docker: dr, loginAttempts: make(map[string]attempt)}
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
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	var applied int
	_ = db.QueryRow("SELECT count(*) FROM schema_migrations WHERE version=1").Scan(&applied)
	if applied > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, role TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, last_login_at TEXT); CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, token_hash TEXT NOT NULL, expires_at TEXT NOT NULL, created_at TEXT NOT NULL, last_seen_at TEXT NOT NULL, revoked_at TEXT, user_agent TEXT, ip_address TEXT); CREATE TABLE IF NOT EXISTS projects(id INTEGER PRIMARY KEY, name TEXT NOT NULL, slug TEXT UNIQUE NOT NULL, description TEXT, status TEXT NOT NULL DEFAULT 'active', created_at TEXT NOT NULL, updated_at TEXT NOT NULL); CREATE TABLE IF NOT EXISTS applications(id INTEGER PRIMARY KEY, project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE, name TEXT NOT NULL, slug TEXT NOT NULL, source_type TEXT NOT NULL, docker_stack_name TEXT, status TEXT NOT NULL DEFAULT 'unknown', created_at TEXT NOT NULL, updated_at TEXT NOT NULL); CREATE TABLE IF NOT EXISTS audit_logs(id INTEGER PRIMARY KEY, user_id INTEGER REFERENCES users(id), action TEXT NOT NULL, resource_type TEXT, resource_id TEXT, metadata TEXT, ip_address TEXT, created_at TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token_hash); CREATE INDEX IF NOT EXISTS idx_apps_project ON applications(project_id);`); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO schema_migrations(version,applied_at) VALUES(1,?)", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
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
	mux.HandleFunc("/api/v1/projects/", a.auth(a.projectRoute))
	mux.HandleFunc("/api/v1/applications/", a.auth(a.applicationRoute))
	mux.HandleFunc("/api/v1/infrastructure", a.auth(a.infrastructure))
	mux.HandleFunc("/api/v1/infrastructure/nodes", a.auth(a.infrastructureNodes))
	mux.HandleFunc("/api/v1/infrastructure/services", a.auth(a.infrastructureServices))
	mux.HandleFunc("/api/v1/activity", a.auth(a.activity))
	mux.HandleFunc("/api/v1/events", a.auth(a.eventsStream))
	mux.HandleFunc("/", a.spa)
	return security(mux)
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if origin := os.Getenv("STACKHOST_ALLOWED_ORIGIN"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
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
	result := map[string]any{"needs_setup": n == 0}
	if a.docker != nil {
		result["infrastructure"] = a.docker.Snapshot(r.Context())
	}
	json.NewEncoder(w).Encode(result)
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
	if !a.loginAllowed(r) {
		jsonError(w, 429, "too_many_attempts", "Tente novamente em alguns instantes.")
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
	a.audit(id, "login", "user", id)
	a.createSession(w, r, id)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
func (a *app) loginAllowed(r *http.Request) bool {
	ip := strings.Split(r.RemoteAddr, ":")[0]
	now := time.Now()
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	v := a.loginAttempts[ip]
	if now.Sub(v.since) > time.Minute {
		v = attempt{since: now}
	}
	v.count++
	a.loginAttempts[ip] = v
	return v.count <= 10
}
func (a *app) createSession(w http.ResponseWriter, r *http.Request, id int64) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return
	}
	token := hex.EncodeToString(seed)
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	now := time.Now().UTC()
	a.db.Exec("INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at,last_seen_at,user_agent,ip_address) VALUES(?,?,?,?,?,?,?,?)", token, id, tokenHash, now.Add(24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), r.UserAgent(), r.RemoteAddr)
	http.SetCookie(w, &http.Cookie{Name: "stackhost_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: os.Getenv("STACKHOST_COOKIE_SECURE") == "true", MaxAge: 86400})
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("stackhost_session"); err == nil {
		hash := sha256.Sum256([]byte(c.Value))
		tokenHash := hex.EncodeToString(hash[:])
		var userID int64
		_ = a.db.QueryRow("SELECT user_id FROM sessions WHERE token_hash=?", tokenHash).Scan(&userID)
		a.db.Exec("UPDATE sessions SET revoked_at=? WHERE token_hash=?", time.Now().UTC().Format(time.RFC3339), tokenHash)
		if userID > 0 {
			a.audit(userID, "logout", "user", userID)
		}
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
		hash := sha256.Sum256([]byte(c.Value))
		var id int64
		var exp string
		err = a.db.QueryRow("SELECT user_id,expires_at FROM sessions WHERE token_hash=? AND revoked_at IS NULL", hex.EncodeToString(hash[:])).Scan(&id, &exp)
		if err != nil {
			jsonError(w, 401, "unauthorized", "Faça login para continuar.")
			return
		}
		t, _ := time.Parse(time.RFC3339, exp)
		if time.Now().After(t) {
			jsonError(w, 401, "unauthorized", "Sessão expirada.")
			return
		}
		_, _ = a.db.Exec("UPDATE sessions SET last_seen_at=? WHERE token_hash=?", time.Now().UTC().Format(time.RFC3339), hex.EncodeToString(hash[:]))
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
	infra := any(map[string]any{"docker": "unknown", "swarm": "unknown"})
	if a.docker != nil {
		infra = a.docker.Snapshot(r.Context())
	}
	json.NewEncoder(w).Encode(map[string]any{"projects": projects, "applications": apps, "infrastructure": infra})
}
func (a *app) audit(userID int64, action, resource string, resourceID any) {
	now := time.Now().UTC().Format(time.RFC3339)
	a.db.Exec("INSERT INTO audit_logs(user_id,action,resource_type,resource_id,created_at) VALUES(?,?,?,?,?)", userID, action, resource, fmt.Sprint(resourceID), now)
}
func (a *app) activity(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query("SELECT id,action,coalesce(resource_type,''),coalesce(resource_id,''),created_at FROM audit_logs ORDER BY id DESC LIMIT 50")
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível carregar a atividade.")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id int
		var action, resource, resourceID, created string
		rows.Scan(&id, &action, &resource, &resourceID, &created)
		out = append(out, map[string]any{"id": id, "action": action, "resource_type": resource, "resource_id": resourceID, "created_at": created})
	}
	json.NewEncoder(w).Encode(out)
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
	if userID, ok := r.Context().Value(userKey{}).(int64); ok {
		a.audit(userID, "project.created", "project", id)
	}
	a.publish("project.created", map[string]any{"id": id, "name": in.Name})
	json.NewEncoder(w).Encode(map[string]any{"id": id, "name": in.Name, "slug": in.Slug, "status": "active"})
}

func (a *app) projectRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		jsonError(w, 404, "not_found", "Projeto não encontrado.")
		return
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		jsonError(w, 404, "not_found", "Projeto não encontrado.")
		return
	}
	if len(parts) == 5 && parts[4] == "applications" {
		a.applications(w, r, id)
		return
	}
	if len(parts) != 4 {
		jsonError(w, 404, "not_found", "Recurso não encontrado.")
		return
	}
	if r.Method == "GET" {
		var name, slug, desc, status, created, updated string
		if a.db.QueryRow("SELECT name,slug,coalesce(description,''),status,created_at,updated_at FROM projects WHERE id=?", id).Scan(&name, &slug, &desc, &status, &created, &updated) != nil {
			jsonError(w, 404, "not_found", "Projeto não encontrado.")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": id, "name": name, "slug": slug, "description": desc, "status": status, "created_at": created, "updated_at": updated})
		return
	}
	if r.Method != "PATCH" {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var in struct{ Name, Description, Status string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, 422, "validation_failed", "Revise os campos informados.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := a.db.Exec("UPDATE projects SET name=COALESCE(NULLIF(?,''),name), description=COALESCE(?,description), status=COALESCE(NULLIF(?,''),status), updated_at=? WHERE id=?", in.Name, in.Description, in.Status, now, id); err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível atualizar o projeto.")
		return
	}
	a.publish("project.updated", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) applications(w http.ResponseWriter, r *http.Request, projectID int64) {
	if r.Method == "GET" {
		rows, err := a.db.Query("SELECT id,name,slug,source_type,coalesce(docker_stack_name,''),status,created_at,updated_at FROM applications WHERE project_id=? ORDER BY id DESC", projectID)
		if err != nil {
			jsonError(w, 500, "internal_error", "Não foi possível listar aplicações.")
			return
		}
		defer rows.Close()
		out := []any{}
		for rows.Next() {
			var id int
			var name, slug, source, stack, status, created, updated string
			rows.Scan(&id, &name, &slug, &source, &stack, &status, &created, &updated)
			out = append(out, map[string]any{"id": id, "project_id": projectID, "name": name, "slug": slug, "source_type": source, "docker_stack_name": stack, "status": status, "created_at": created, "updated_at": updated})
		}
		json.NewEncoder(w).Encode(out)
		return
	}
	if r.Method != "POST" {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var in struct{ Name, Slug, SourceType, DockerStackName string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
		jsonError(w, 422, "validation_failed", "O nome é obrigatório.")
		return
	}
	if in.SourceType == "" {
		in.SourceType = "compose"
	}
	valid := map[string]bool{"catalog": true, "compose": true, "image": true, "git": true}
	if !valid[in.SourceType] {
		jsonError(w, 422, "validation_failed", "Tipo de origem inválido.")
		return
	}
	if in.Slug == "" {
		in.Slug = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(in.Name), " ", "-"))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := a.db.Exec("INSERT INTO applications(project_id,name,slug,source_type,docker_stack_name,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", projectID, in.Name, in.Slug, in.SourceType, in.DockerStackName, "unknown", now, now)
	if err != nil {
		jsonError(w, 409, "already_exists", "Não foi possível criar a aplicação.")
		return
	}
	id, _ := res.LastInsertId()
	if userID, ok := r.Context().Value(userKey{}).(int64); ok {
		a.audit(userID, "application.created", "application", id)
	}
	a.publish("application.created", map[string]any{"id": id, "project_id": projectID})
	json.NewEncoder(w).Encode(map[string]any{"id": id, "project_id": projectID, "name": in.Name, "slug": in.Slug, "source_type": in.SourceType, "status": "unknown"})
}

func (a *app) applicationRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 {
		jsonError(w, 404, "not_found", "Aplicação não encontrada.")
		return
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		jsonError(w, 404, "not_found", "Aplicação não encontrada.")
		return
	}
	if r.Method == "GET" {
		var project int
		var name, slug, source, stack, status, created, updated string
		if a.db.QueryRow("SELECT project_id,name,slug,source_type,coalesce(docker_stack_name,''),status,created_at,updated_at FROM applications WHERE id=?", id).Scan(&project, &name, &slug, &source, &stack, &status, &created, &updated) != nil {
			jsonError(w, 404, "not_found", "Aplicação não encontrada.")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": id, "project_id": project, "name": name, "slug": slug, "source_type": source, "docker_stack_name": stack, "status": status, "created_at": created, "updated_at": updated})
		return
	}
	if r.Method != "PATCH" {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var in struct{ Name, Status, DockerStackName string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, 422, "validation_failed", "Revise os campos informados.")
		return
	}
	_, err = a.db.Exec("UPDATE applications SET name=COALESCE(NULLIF(?,''),name), status=COALESCE(NULLIF(?,''),status), docker_stack_name=COALESCE(?,docker_stack_name), updated_at=? WHERE id=?", in.Name, in.Status, in.DockerStackName, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível atualizar a aplicação.")
		return
	}
	a.publish("application.updated", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) publish(event string, data map[string]any) {
	payload := map[string]any{"event": event, "data": data, "at": time.Now().UTC().Format(time.RFC3339)}
	select {
	case a.events <- payload:
	default:
	}
}
func (a *app) eventsStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	b, _ := json.Marshal(map[string]any{"event": "connected"})
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-a.events:
			b, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		case <-ticker.C:
			infra := any(map[string]any{"available": false, "message": "Docker não está conectado."})
			if a.docker != nil {
				infra = a.docker.Snapshot(r.Context())
			}
			b, _ := json.Marshal(map[string]any{"event": "infrastructure.updated", "data": infra, "at": time.Now().UTC().Format(time.RFC3339)})
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}
func (a *app) infrastructure(w http.ResponseWriter, r *http.Request) {
	if a.docker != nil {
		json.NewEncoder(w).Encode(a.docker.Snapshot(r.Context()))
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"docker": map[string]any{"available": false, "message": "Docker não está conectado."}, "swarm": map[string]any{"active": false, "message": "O host não está conectado a um Swarm."}, "nodes": []any{}, "services": []any{}})
}
func (a *app) infrastructureNodes(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		jsonError(w, 503, "docker_unavailable", "Docker não está conectado.")
		return
	}
	nodes, err := a.docker.Nodes(r.Context())
	if err != nil {
		jsonError(w, 503, "swarm_unavailable", "Nós do Swarm indisponíveis.")
		return
	}
	json.NewEncoder(w).Encode(nodes)
}
func (a *app) infrastructureServices(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		jsonError(w, 503, "docker_unavailable", "Docker não está conectado.")
		return
	}
	services, err := a.docker.Services(r.Context())
	if err != nil {
		jsonError(w, 503, "swarm_unavailable", "Services do Swarm indisponíveis.")
		return
	}
	json.NewEncoder(w).Encode(services)
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
