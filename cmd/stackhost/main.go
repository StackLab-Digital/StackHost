package main

import (
	"context"
	"crypto/hmac"
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

	"github.com/StackLab-Digital/StackHost/internal/deployment"
	dockerreader "github.com/StackLab-Digital/StackHost/internal/docker"
	eventhub "github.com/StackLab-Digital/StackHost/internal/events"
	"github.com/StackLab-Digital/StackHost/internal/ingress"
	"github.com/StackLab-Digital/StackHost/internal/migrations"
	"github.com/StackLab-Digital/StackHost/internal/secure"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type app struct {
	db            *sql.DB
	sessionSecret string
	events        *eventhub.Hub
	docker        *dockerreader.Reader
	cipher        *secure.Cipher
	loginMu       sync.Mutex
	loginAttempts map[string]attempt
	deployments   *deploymentAPI
	domains       *domainAPI
	ingressProxy  *ingress.Proxy
	backups       *backupAPI
}
type attempt struct {
	count int
	since time.Time
}

func main() {
	addr := getenv("STACKHOST_HTTP_ADDR", ":8080")
	dataDir := getenv("STACKHOST_DATA_DIR", "./data")
	appEnv := getenv("STACKHOST_APP_ENV", "development")
	sessionSecret := getenv("STACKHOST_SESSION_SECRET", "development-only-change-me")
	encryptionKey, encryptionKeySet := os.LookupEnv("STACKHOST_ENCRYPTION_KEY")
	if !encryptionKeySet {
		if appEnv == "production" {
			panic("STACKHOST_ENCRYPTION_KEY is required in production")
		}
		encryptionKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	}
	if appEnv == "production" && (sessionSecret == "development-only-change-me" || len(sessionSecret) < 32) {
		panic("STACKHOST_SESSION_SECRET must be a strong value in production")
	}
	cipher, err := secure.New(encryptionKey)
	if err != nil {
		panic("STACKHOST_ENCRYPTION_KEY must be base64 encoded 32 bytes")
	}
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		panic(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dataDir, "stackhost.db?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL"))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err = migrations.Run(context.Background(), db); err != nil {
		panic(err)
	}
	dr, _ := dockerreader.NewReader()
	events := eventhub.New(64)
	defer events.Close()
	a := &app{db: db, sessionSecret: sessionSecret, cipher: cipher, events: events, docker: dr, loginAttempts: make(map[string]attempt)}
	a.backups = &backupAPI{app: a, dataDir: dataDir}
	a.ingressProxy = ingress.NewProxy()
	a.domains = newDomainAPI(a, ingress.NewSQLStore(db), a.ingressProxy, dr)
	a.domains.reload(context.Background())
	store := deployment.NewSQLStore(db)
	runner := deployment.NewDockerRunner()
	engine := deployment.NewEngine(store, runner, deployment.EngineOptions{Emitter: deployment.EmitterFunc(a.publishDeploymentEvent)})
	a.deployments = newDeploymentAPI(a, engine)
	a.deployments.logs = runner
	if _, err := engine.Recover(context.Background()); err != nil {
		panic(err)
	}
	defer engine.Close()
	ingressServers := startIngressServers(a)
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ingressServers.Close(shutdown)
	}()
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
	mux.HandleFunc("/api/v1/applications/", a.auth(a.applicationRouteV2))
	mux.HandleFunc("/api/v1/deployments/", a.auth(func(w http.ResponseWriter, r *http.Request) {
		if a.deployments == nil {
			jsonError(w, http.StatusServiceUnavailable, "runtime_unavailable", "O motor de deploy não está disponível.")
			return
		}
		a.deployments.deploymentRoute(w, r)
	}))
	mux.HandleFunc("/api/v1/ingress/status", a.auth(func(w http.ResponseWriter, r *http.Request) {
		if a.domains == nil {
			jsonError(w, http.StatusServiceUnavailable, "runtime_unavailable", "O ingress não está disponível.")
			return
		}
		a.domains.status(w, r)
	}))
	mux.HandleFunc("/api/v1/backups/system", a.auth(a.backupRoute))
	mux.HandleFunc("/api/v1/backups/system/", a.auth(a.backupRoute))
	mux.HandleFunc("/api/v1/notifications", a.auth(a.notificationsRoute))
	mux.HandleFunc("/api/v1/notifications/", a.auth(a.notificationsRoute))
	mux.HandleFunc("/api/v1/catalog", a.auth(a.catalog))
	mux.HandleFunc("/api/v1/source/validate", a.auth(a.validateSourcePreview))
	mux.HandleFunc("/api/v1/infrastructure", a.auth(a.infrastructure))
	mux.HandleFunc("/api/v1/infrastructure/nodes", a.auth(a.infrastructureNodes))
	mux.HandleFunc("/api/v1/infrastructure/services", a.auth(a.infrastructureServices))
	mux.HandleFunc("/api/v1/infrastructure/containers", a.auth(a.infrastructureContainers))
	mux.HandleFunc("/api/v1/infrastructure/images", a.auth(a.infrastructureImages))
	mux.HandleFunc("/api/v1/infrastructure/volumes", a.auth(a.infrastructureVolumes))
	mux.HandleFunc("/api/v1/infrastructure/networks", a.auth(a.infrastructureNetworks))
	mux.HandleFunc("/api/v1/infrastructure/swarm/init", a.auth(a.infrastructureSwarmInit))
	mux.HandleFunc("/api/v1/settings/environment", a.auth(a.environmentSettings))
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
	w.Header().Set("Content-Type", "application/json")
	if err := a.db.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_ready"})
		return
	}
	current, err := migrations.IsCurrent(r.Context(), a.db)
	if err != nil || !current {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_ready", "reason": "migrations"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
func jsonError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg}})
}
func jsonValidation(w http.ResponseWriter, fields map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "validation_failed", "message": "Revise os campos informados.", "fields": fields}})
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
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonValidation(w, map[string]string{"form": "JSON inválido."})
		return
	}
	fields := map[string]string{}
	if strings.TrimSpace(in.Name) == "" {
		fields["name"] = "O nome é obrigatório."
	}
	if !strings.Contains(in.Email, "@") {
		fields["email"] = "Informe um e-mail válido."
	}
	if len(in.Password) < 8 {
		fields["password"] = "Use ao menos 8 caracteres."
	}
	if len(fields) > 0 {
		jsonValidation(w, fields)
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
	tokenHash := a.sessionHash(token)
	now := time.Now().UTC()
	a.db.Exec("INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at,last_seen_at,user_agent,ip_address) VALUES(?,?,?,?,?,?,?,?)", token, id, tokenHash, now.Add(24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), r.UserAgent(), r.RemoteAddr)
	http.SetCookie(w, &http.Cookie{Name: "stackhost_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: os.Getenv("STACKHOST_COOKIE_SECURE") == "true", MaxAge: 86400})
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("stackhost_session"); err == nil {
		tokenHash := a.sessionHash(c.Value)
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
		tokenHash := a.sessionHash(c.Value)
		var id int64
		var exp string
		err = a.db.QueryRow("SELECT user_id,expires_at FROM sessions WHERE token_hash=? AND revoked_at IS NULL", tokenHash).Scan(&id, &exp)
		if err != nil {
			jsonError(w, 401, "unauthorized", "Faça login para continuar.")
			return
		}
		t, _ := time.Parse(time.RFC3339, exp)
		if time.Now().After(t) {
			jsonError(w, 401, "unauthorized", "Sessão expirada.")
			return
		}
		_, _ = a.db.Exec("UPDATE sessions SET last_seen_at=? WHERE token_hash=?", time.Now().UTC().Format(time.RFC3339), tokenHash)
		r = r.WithContext(context.WithValue(r.Context(), userKey{}, id))
		next(w, r)
	}
}

func (a *app) sessionHash(token string) string {
	mac := hmac.New(sha256.New, []byte(a.sessionSecret))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

type userKey struct{}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	var u struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		Role  string `json:"role"`
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
	recent := []any{}
	if rows, err := a.db.Query("SELECT action,coalesce(resource_type,''),created_at FROM audit_logs ORDER BY id DESC LIMIT 5"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var action, resource, created string
			rows.Scan(&action, &resource, &created)
			recent = append(recent, map[string]string{"action": action, "resource_type": resource, "created_at": created, "description": activityDescription(action)})
		}
	}
	recentProjects := []any{}
	if rows, err := a.db.Query("SELECT id,name,(SELECT count(*) FROM applications a WHERE a.project_id=projects.id),updated_at FROM projects ORDER BY updated_at DESC LIMIT 4"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, count int
			var name, updated string
			_ = rows.Scan(&id, &name, &count, &updated)
			recentProjects = append(recentProjects, map[string]any{"id": id, "name": name, "applications_count": count, "updated_at": updated})
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"projects": projects, "applications": apps, "infrastructure": infra, "activity": recent, "recent_projects": recentProjects})
}
func activityDescription(action string) string {
	descriptions := map[string]string{"login": "Entrou no painel", "logout": "Saiu do painel", "onboarding": "Criou o administrador inicial", "project.created": "Criou um projeto", "project.updated": "Atualizou um projeto", "application.created": "Adicionou uma aplicação", "application.updated": "Atualizou uma aplicação", "swarm.initialized": "Preparou o ambiente Docker"}
	return descriptions[action]
}
func (a *app) audit(userID int64, action, resource string, resourceID any) {
	now := time.Now().UTC().Format(time.RFC3339)
	a.db.Exec("INSERT INTO audit_logs(user_id,action,resource_type,resource_id,created_at) VALUES(?,?,?,?,?)", userID, action, resource, fmt.Sprint(resourceID), now)
}
func (a *app) activity(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query("SELECT audit_logs.id,audit_logs.action,coalesce(audit_logs.resource_type,''),coalesce(audit_logs.resource_id,''),audit_logs.created_at,coalesce(users.name,'Administrador') FROM audit_logs LEFT JOIN users ON users.id=audit_logs.user_id ORDER BY audit_logs.id DESC LIMIT 50")
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível carregar a atividade.")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id int
		var action, resource, resourceID, created, actor string
		rows.Scan(&id, &action, &resource, &resourceID, &created, &actor)
		out = append(out, map[string]any{"id": id, "action": action, "description": activityDescription(action), "resource_type": resource, "resource_id": resourceID, "actor_name": actor, "created_at": created})
	}
	json.NewEncoder(w).Encode(out)
}
func (a *app) projects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "GET" {
		rows, err := a.db.Query("SELECT id,name,slug,coalesce(description,''),status,created_at,updated_at,(SELECT count(*) FROM applications a WHERE a.project_id=projects.id) FROM projects ORDER BY id DESC")
		if err != nil {
			jsonError(w, 500, "internal_error", "Não foi possível listar os projetos.")
			return
		}
		defer rows.Close()
		out := []any{}
		for rows.Next() {
			var id int
			var name, slug, desc, status, created, updated string
			var applicationsCount int
			rows.Scan(&id, &name, &slug, &desc, &status, &created, &updated, &applicationsCount)
			out = append(out, map[string]any{"id": id, "name": name, "slug": slug, "description": desc, "status": status, "applications_count": applicationsCount, "created_at": created, "updated_at": updated})
		}
		json.NewEncoder(w).Encode(out)
		return
	}
	if r.Method != "POST" {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var in struct{ Name, Slug, Description string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonValidation(w, map[string]string{"form": "JSON inválido."})
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		jsonValidation(w, map[string]string{"name": "O nome é obrigatório."})
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
	if userID, ok := r.Context().Value(userKey{}).(int64); ok {
		a.audit(userID, "project.updated", "project", id)
	}
	a.publish("project.updated", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) applications(w http.ResponseWriter, r *http.Request, projectID int64) {
	if r.Method == "GET" {
		rows, err := a.db.Query("SELECT id,name,slug,source_type,coalesce(docker_stack_name,''),coalesce(status,'unknown'),coalesce(configuration_status,'draft'),created_at,updated_at FROM applications WHERE project_id=? ORDER BY id DESC", projectID)
		if err != nil {
			jsonError(w, 500, "internal_error", "Não foi possível listar aplicações.")
			return
		}
		defer rows.Close()
		out := []any{}
		for rows.Next() {
			var id int
			var name, slug, source, stack, status, configurationStatus, created, updated string
			if err := rows.Scan(&id, &name, &slug, &source, &stack, &status, &configurationStatus, &created, &updated); err != nil {
				jsonError(w, 500, "internal_error", "Não foi possível listar aplicações.")
				return
			}
			out = append(out, map[string]any{"id": id, "project_id": projectID, "name": name, "slug": slug, "source_type": source, "docker_stack_name": stack, "status": status, "configuration_status": configurationStatus, "created_at": created, "updated_at": updated})
		}
		if err := rows.Err(); err != nil {
			jsonError(w, 500, "internal_error", "Não foi possível listar aplicações.")
			return
		}
		json.NewEncoder(w).Encode(out)
		return
	}
	if r.Method != "POST" {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var in struct {
		Name            string      `json:"name"`
		Description     string      `json:"description"`
		Slug            string      `json:"slug"`
		SourceType      string      `json:"source_type"`
		DockerStackName string      `json:"docker_stack_name"`
		Source          sourceInput `json:"source"`
		SaveAsDraft     bool        `json:"save_as_draft"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024+64*1024)
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonValidation(w, map[string]string{"form": "JSON inválido."})
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		jsonValidation(w, map[string]string{"name": "O nome é obrigatório."})
		return
	}
	if len(in.Name) > 100 || len(in.Description) > 2000 {
		jsonValidation(w, map[string]string{"name": "Revise o tamanho dos campos."})
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
	result := validateSource(in.SourceType, in.Source)
	legacyDraft := in.SourceType == "compose" && strings.TrimSpace(in.Source.ComposeYAML) == "" && len(in.Source.Environment) == 0
	if !in.SaveAsDraft && !legacyDraft && !result.Valid {
		json.NewEncoder(w).Encode(result)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	configurationStatus := "draft"
	configuredAt, validatedAt := "", ""
	sourceRevision := 0
	if result.Valid {
		configurationStatus = "configured"
		configuredAt, validatedAt = now, now
		sourceRevision = 1
	}
	tx, err := a.db.Begin()
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível criar a aplicação.")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec("INSERT INTO applications(project_id,name,description,slug,source_type,docker_stack_name,status,configuration_status,configured_at,last_validated_at,source_revision,created_at,updated_at) VALUES(?,?,?,?,?,?, 'not_deployed', ?,NULLIF(?,''),NULLIF(?,''),?,?,?)", projectID, in.Name, in.Description, in.Slug, in.SourceType, in.DockerStackName, configurationStatus, configuredAt, validatedAt, sourceRevision, now, now)
	if err != nil {
		jsonError(w, 409, "already_exists", "Não foi possível criar a aplicação.")
		return
	}
	id, _ := res.LastInsertId()
	if result.Valid {
		payload, _ := json.Marshal(in.Source)
		ciphertext, nonce, cipherErr := a.cipher.Encrypt(payload)
		checksumBytes := sha256.Sum256(payload)
		summary, _ := json.Marshal(sourceSummary(in.Source, result))
		errorsJSON, _ := json.Marshal(result.Errors)
		warningsJSON, _ := json.Marshal(result.Warnings)
		if cipherErr != nil {
			jsonError(w, 500, "encryption_failed", "Não foi possível proteger a origem.")
			return
		}
		_, err = tx.Exec(`INSERT INTO application_sources(application_id,source_type,encrypted_payload,encryption_nonce,checksum,validation_status,validation_errors,validation_warnings,summary_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, in.SourceType, ciphertext, nonce, hex.EncodeToString(checksumBytes[:]), "configured", errorsJSON, warningsJSON, summary, now, now)
		if err != nil {
			jsonError(w, 500, "internal_error", "Não foi possível salvar a origem.")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível criar a aplicação.")
		return
	}
	if userID, ok := r.Context().Value(userKey{}).(int64); ok {
		a.audit(userID, "application.created", "application", id)
	}
	a.publish("application.created", map[string]any{"id": id, "project_id": projectID})
	json.NewEncoder(w).Encode(map[string]any{"id": id, "project_id": projectID, "name": in.Name, "description": in.Description, "slug": in.Slug, "source_type": in.SourceType, "status": "not_deployed", "configuration_status": configurationStatus})
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
	var in struct {
		Name            string `json:"name"`
		DockerStackName string `json:"docker_stack_name"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, 422, "validation_failed", "Revise os campos informados.")
		return
	}
	_, err = a.db.Exec("UPDATE applications SET name=COALESCE(NULLIF(?,''),name), docker_stack_name=COALESCE(?,docker_stack_name), updated_at=? WHERE id=?", in.Name, in.DockerStackName, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível atualizar a aplicação.")
		return
	}
	if userID, ok := r.Context().Value(userKey{}).(int64); ok {
		a.audit(userID, "application.updated", "application", id)
	}
	a.publish("application.updated", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) publish(event string, data map[string]any) {
	if a.events != nil {
		a.events.Publish(event, data)
	}
	go a.deliverNotifications(event, data)
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
	if a.events == nil {
		return
	}
	subscriber := a.events.Subscribe()
	defer a.events.Unsubscribe(subscriber)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-subscriber:
			if !ok {
				return
			}
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
	json.NewEncoder(w).Encode(map[string]any{"available": false, "message": "Docker não está conectado.", "swarm": map[string]any{"active": false, "message": "O host não está conectado a um Swarm."}, "nodes": []any{}, "services": []any{}})
}

func (a *app) environmentSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPatch {
		userID, _ := r.Context().Value(userKey{}).(int64)
		var role string
		if a.db.QueryRow("SELECT role FROM users WHERE id=?", userID).Scan(&role) != nil || role != "admin" {
			jsonError(w, http.StatusForbidden, "forbidden", "Apenas administradores podem alterar o modo de execução.")
			return
		}
		var input struct {
			RuntimeMode string `json:"runtime_mode"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || (input.RuntimeMode != "standalone" && input.RuntimeMode != "swarm") {
			jsonValidation(w, map[string]string{"runtime_mode": "Escolha standalone ou swarm."})
			return
		}
		_, err := a.db.Exec("UPDATE environment_settings SET runtime_mode=?,updated_at=? WHERE id=1", input.RuntimeMode, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			jsonError(w, 500, "internal_error", "Não foi possível salvar o modo de execução.")
			return
		}
		a.audit(userID, "environment.runtime_mode_updated", "environment", 1)
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPatch {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var mode string
	if a.db.QueryRow("SELECT runtime_mode FROM environment_settings WHERE id=1").Scan(&mode) != nil {
		mode = "standalone"
	}
	result := map[string]any{"runtime_mode": mode, "docker_available": false, "swarm_active": false, "swarm_manager": false, "standalone_available": false}
	if a.docker != nil {
		snapshot := a.docker.Snapshot(r.Context())
		result["docker_available"] = snapshot.Available
		result["standalone_available"] = snapshot.Available
		result["swarm_active"] = snapshot.Swarm.Active
		result["swarm_manager"] = snapshot.Swarm.ControlAvailable
	}
	json.NewEncoder(w).Encode(result)
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
func inventoryLimit(r *http.Request) int {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}
func (a *app) infrastructureContainers(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		jsonError(w, 503, "docker_unavailable", "Docker não está conectado.")
		return
	}
	items, err := a.docker.Containers(r.Context(), inventoryLimit(r), r.URL.Query().Get("state"))
	if err != nil {
		jsonError(w, 503, "docker_unavailable", "Não foi possível consultar os containers.")
		return
	}
	json.NewEncoder(w).Encode(items)
}
func (a *app) infrastructureImages(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		jsonError(w, 503, "docker_unavailable", "Docker não está conectado.")
		return
	}
	items, err := a.docker.Images(r.Context(), inventoryLimit(r))
	if err != nil {
		jsonError(w, 503, "docker_unavailable", "Não foi possível consultar as imagens.")
		return
	}
	json.NewEncoder(w).Encode(items)
}
func (a *app) infrastructureVolumes(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		jsonError(w, 503, "docker_unavailable", "Docker não está conectado.")
		return
	}
	items, err := a.docker.Volumes(r.Context(), inventoryLimit(r))
	if err != nil {
		jsonError(w, 503, "docker_unavailable", "Não foi possível consultar os volumes.")
		return
	}
	json.NewEncoder(w).Encode(items)
}
func (a *app) infrastructureNetworks(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		jsonError(w, 503, "docker_unavailable", "Docker não está conectado.")
		return
	}
	items, err := a.docker.Networks(r.Context(), inventoryLimit(r))
	if err != nil {
		jsonError(w, 503, "docker_unavailable", "Não foi possível consultar as redes.")
		return
	}
	json.NewEncoder(w).Encode(items)
}
func (a *app) infrastructureSwarmInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	userID, _ := r.Context().Value(userKey{}).(int64)
	var role string
	if a.db.QueryRow("SELECT role FROM users WHERE id=?", userID).Scan(&role) != nil || role != "admin" {
		jsonError(w, 403, "permission_denied", "Somente administradores podem preparar o ambiente.")
		return
	}
	if a.docker == nil {
		jsonError(w, 503, "docker_unavailable", "Docker não está conectado.")
		return
	}
	current := a.docker.Snapshot(r.Context())
	if current.Swarm.Active {
		json.NewEncoder(w).Encode(map[string]any{"status": "active", "node_role": "manager", "message": "O ambiente já está preparado."})
		return
	}
	var input struct {
		AdvertiseAddress string `json:"advertise_address"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&input)
	}
	clusterID, err := a.docker.InitSwarm(r.Context(), strings.TrimSpace(input.AdvertiseAddress))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "advertise") || strings.Contains(strings.ToLower(err.Error()), "address") {
			jsonError(w, http.StatusUnprocessableEntity, "advertise_address_required", "O Docker precisa de um endereço de anúncio. Abra as opções avançadas e informe o endereço desta máquina.")
			return
		}
		jsonError(w, 422, "swarm_init_failed", "Não foi possível preparar o ambiente Docker. Verifique a rede do servidor e tente novamente.")
		return
	}
	a.audit(userID, "swarm.initialized", "infrastructure", clusterID)
	a.publish("swarm.initialized", map[string]any{"status": "active"})
	json.NewEncoder(w).Encode(map[string]any{"status": "active", "cluster_id": clusterID, "node_role": "manager", "message": "Ambiente preparado com sucesso."})
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
