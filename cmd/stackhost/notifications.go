package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var notificationEvents = map[string]bool{
	"deployment.succeeded": true, "deployment.failed": true, "application.degraded": true,
	"domain.active": true, "certificate.error": true, "backup.succeeded": true,
	"backup.failed": true, "docker.unavailable": true,
}

func (a *app) notificationsRoute(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(userKey{}).(int64)
	if !a.isAdmin(userID) {
		jsonError(w, http.StatusForbidden, "forbidden", "Apenas administradores podem configurar notificações.")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 4 {
		switch r.Method {
		case http.MethodGet:
			a.listNotifications(w, r)
		case http.MethodPost:
			a.createNotification(w, r)
		default:
			jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
		}
		return
	}
	if len(parts) != 5 && len(parts) != 6 {
		jsonError(w, http.StatusNotFound, "not_found", "Notificação não encontrada.")
		return
	}
	id, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, http.StatusNotFound, "not_found", "Notificação não encontrada.")
		return
	}
	if len(parts) == 6 && parts[5] == "test" && r.Method == http.MethodPost {
		a.testNotification(w, r, id)
		return
	}
	if r.Method == http.MethodDelete {
		if _, err := a.db.ExecContext(r.Context(), `DELETE FROM notifications WHERE id=?`, id); err != nil {
			jsonError(w, 500, "internal_error", "Não foi possível remover a notificação.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
}

func (a *app) listNotifications(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,name,kind,events,enabled,last_status,last_error,created_at,updated_at FROM notifications ORDER BY id DESC`)
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível listar as notificações.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, kind, events, status, lastError, created, updated string
		var enabled int
		if rows.Scan(&id, &name, &kind, &events, &enabled, &status, &lastError, &created, &updated) != nil {
			continue
		}
		items = append(items, map[string]any{"id": id, "name": name, "kind": kind, "events": json.RawMessage(events), "enabled": enabled != 0, "last_status": status, "last_error": lastError, "created_at": created, "updated_at": updated})
	}
	writeJSON(w, items)
}

func (a *app) createNotification(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name    string   `json:"name"`
		Kind    string   `json:"kind"`
		URL     string   `json:"url"`
		Events  []string `json:"events"`
		Enabled *bool    `json:"enabled"`
	}
	if !decodeJSON(w, r, &input) || strings.TrimSpace(input.Name) == "" {
		return
	}
	if input.Kind != "webhook" && input.Kind != "discord" && input.Kind != "slack" {
		jsonError(w, 422, "validation_failed", "Escolha webhook, Discord ou Slack.")
		return
	}
	u, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil || u.Scheme != "https" || u.Host == "" || isPrivateNotificationHost(u.Hostname()) {
		jsonError(w, 422, "validation_failed", "Use uma URL HTTPS válida.")
		return
	}
	for _, event := range input.Events {
		if !notificationEvents[event] {
			jsonError(w, 422, "validation_failed", "Evento de notificação inválido.")
			return
		}
	}
	ciphertext, nonce, err := a.cipher.Encrypt([]byte(input.URL))
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível proteger a URL.")
		return
	}
	events, _ := json.Marshal(input.Events)
	enabled := 1
	if input.Enabled != nil && !*input.Enabled {
		enabled = 0
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := a.db.ExecContext(r.Context(), `INSERT INTO notifications(name,kind,encrypted_url,encryption_nonce,events,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, input.Name, input.Kind, ciphertext, nonce, events, enabled, now, now)
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível salvar a notificação.")
		return
	}
	id, _ := result.LastInsertId()
	writeJSONStatus(w, http.StatusCreated, map[string]any{"id": id, "name": input.Name, "kind": input.Kind, "events": input.Events, "enabled": enabled != 0, "last_status": "never"})
}

func isPrivateNotificationHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}

func (a *app) testNotification(w http.ResponseWriter, r *http.Request, id int64) {
	var kind string
	var encrypted, nonce []byte
	if err := a.db.QueryRowContext(r.Context(), `SELECT kind,encrypted_url,encryption_nonce FROM notifications WHERE id=?`, id).Scan(&kind, &encrypted, &nonce); err != nil {
		jsonError(w, 404, "not_found", "Notificação não encontrada.")
		return
	}
	plain, err := a.cipher.Decrypt(encrypted, nonce)
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível ler a configuração.")
		return
	}
	payload := map[string]any{"event": "stackhost.test", "kind": kind, "sent_at": time.Now().UTC().Format(time.RFC3339Nano)}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, string(plain), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	status, message := "ok", ""
	if err != nil {
		status, message = "error", "Não foi possível alcançar o webhook."
	} else {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			status, message = "error", "O webhook recusou o teste."
		}
	}
	_, _ = a.db.ExecContext(r.Context(), `UPDATE notifications SET last_status=?,last_error=?,updated_at=? WHERE id=?`, status, message, time.Now().UTC().Format(time.RFC3339Nano), id)
	if status != "ok" {
		jsonError(w, 502, "notification_failed", message)
		return
	}
	writeJSON(w, map[string]string{"status": status})
}

func (a *app) deliverNotifications(event string, data map[string]any) {
	if a == nil || a.db == nil || !notificationEvents[event] {
		return
	}
	rows, err := a.db.Query(`SELECT id,encrypted_url,encryption_nonce,events FROM notifications WHERE enabled=1`)
	if err != nil {
		return
	}
	defer rows.Close()
	payload, _ := json.Marshal(map[string]any{"event": event, "data": data, "sent_at": time.Now().UTC().Format(time.RFC3339Nano)})
	for rows.Next() {
		var id int64
		var encrypted, nonce []byte
		var events string
		if rows.Scan(&id, &encrypted, &nonce, &events) != nil || !notificationSubscribes(events, event) {
			continue
		}
		plain, err := a.cipher.Decrypt(encrypted, nonce)
		if err != nil || isPrivateNotificationHost(mustURLHost(string(plain))) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, string(plain), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, requestErr := http.DefaultClient.Do(req)
		status, message := "ok", ""
		if requestErr != nil {
			status, message = "error", "delivery failed"
		} else {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				status, message = "error", "webhook returned a non-success status"
			}
		}
		cancel()
		_, _ = a.db.Exec(`UPDATE notifications SET last_status=?,last_error=?,updated_at=? WHERE id=?`, status, message, time.Now().UTC().Format(time.RFC3339Nano), id)
	}
}

func notificationSubscribes(raw, event string) bool {
	var events []string
	return json.Unmarshal([]byte(raw), &events) == nil && func() bool {
		for _, item := range events {
			if item == event {
				return true
			}
		}
		return false
	}()
}

func mustURLHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
