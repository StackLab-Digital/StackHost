package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	dockerreader "github.com/StackLab-Digital/StackHost/internal/docker"
	"github.com/StackLab-Digital/StackHost/internal/ingress"
)

type domainAPI struct {
	app      *app
	store    ingress.Store
	proxy    *ingress.Proxy
	resolver ingress.TargetResolver
}

type dockerDomainResolver struct {
	app    *app
	reader *dockerreader.Reader
}

func (r dockerDomainResolver) Resolve(ctx context.Context, domain ingress.Domain) (string, error) {
	var stackName, mode string
	if err := r.app.db.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(docker_stack_name,''),slug) FROM applications WHERE id=?`, domain.ApplicationID).Scan(&stackName); err != nil {
		return "", err
	}
	_ = r.app.db.QueryRowContext(ctx, `SELECT runtime_mode FROM environment_settings WHERE id=1`).Scan(&mode)
	return r.reader.ResolveIngressTarget(ctx, mode, stackName, domain.ServiceName, domain.TargetPort)
}

func newDomainAPI(a *app, store ingress.Store, proxy *ingress.Proxy, reader *dockerreader.Reader) *domainAPI {
	api := &domainAPI{app: a, store: store, proxy: proxy}
	if reader != nil {
		api.resolver = dockerDomainResolver{app: a, reader: reader}
	}
	return api
}

func (h *domainAPI) reload(ctx context.Context) {
	if h.proxy == nil || h.resolver == nil {
		return
	}
	items, err := h.store.ListEnabled(ctx)
	if err == nil {
		h.proxy.SetRoutes(ctx, items, h.resolver)
	}
}

func (h *domainAPI) route(w http.ResponseWriter, r *http.Request, applicationID int64, parts []string) {
	if len(parts) == 5 {
		switch r.Method {
		case http.MethodGet:
			h.list(w, r, applicationID)
		case http.MethodPost:
			if h.requireAdmin(w, r) {
				h.create(w, r, applicationID)
			}
		default:
			jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
		}
		return
	}
	domainID, err := strconv.ParseInt(parts[5], 10, 64)
	if err != nil || domainID <= 0 {
		jsonError(w, http.StatusNotFound, "not_found", "Domínio não encontrado.")
		return
	}
	if len(parts) == 6 {
		if r.Method != http.MethodPatch && r.Method != http.MethodDelete {
			jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
			return
		}
		if !h.requireAdmin(w, r) {
			return
		}
		if r.Method == http.MethodDelete {
			h.delete(w, r, applicationID, domainID)
		} else {
			h.update(w, r, applicationID, domainID)
		}
		return
	}
	if len(parts) == 7 && parts[6] == "check" && r.Method == http.MethodPost {
		if !h.requireAdmin(w, r) {
			return
		}
		h.check(w, r, applicationID, domainID)
		return
	}
	jsonError(w, http.StatusNotFound, "not_found", "Rota não encontrada.")
}

func (h *domainAPI) list(w http.ResponseWriter, r *http.Request, applicationID int64) {
	items, err := h.store.List(r.Context(), applicationID)
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível listar os domínios.")
		return
	}
	writeJSON(w, items)
}

func (h *domainAPI) create(w http.ResponseWriter, r *http.Request, applicationID int64) {
	var input ingress.CreateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.applicationExists(r.Context(), applicationID); err != nil {
		h.writeDomainError(w, err)
		return
	}
	item, err := h.store.Create(r.Context(), applicationID, input)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	h.reload(r.Context())
	writeJSONStatus(w, http.StatusCreated, item)
}

func (h *domainAPI) update(w http.ResponseWriter, r *http.Request, applicationID, domainID int64) {
	var input ingress.UpdateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	item, err := h.store.Update(r.Context(), applicationID, domainID, input)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	h.reload(r.Context())
	writeJSON(w, item)
}

func (h *domainAPI) delete(w http.ResponseWriter, r *http.Request, applicationID, domainID int64) {
	if err := h.store.Delete(r.Context(), applicationID, domainID); err != nil {
		h.writeDomainError(w, err)
		return
	}
	h.reload(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (h *domainAPI) check(w http.ResponseWriter, r *http.Request, applicationID, domainID int64) {
	item, err := h.store.Get(r.Context(), applicationID, domainID)
	if err != nil {
		h.writeDomainError(w, err)
		return
	}
	if _, err := net.LookupHost(item.Hostname); err != nil {
		_ = h.store.MarkCheck(r.Context(), item.ID, ingress.StatusDNSError, "pending", "O domínio ainda não resolve no DNS.")
		item.Status, item.CertificateStatus, item.LastError = ingress.StatusDNSError, "pending", "O domínio ainda não resolve no DNS."
		writeJSON(w, item)
		return
	}
	if h.resolver == nil {
		_ = h.store.MarkCheck(r.Context(), item.ID, ingress.StatusTargetUnavailable, "pending", "O runtime Docker não está disponível.")
		item.Status, item.CertificateStatus, item.LastError = ingress.StatusTargetUnavailable, "pending", "O runtime Docker não está disponível."
		writeJSON(w, item)
		return
	}
	if _, err := h.resolver.Resolve(r.Context(), item); err != nil {
		_ = h.store.MarkCheck(r.Context(), item.ID, ingress.StatusTargetUnavailable, "pending", "O serviço configurado ainda não está disponível.")
		item.Status, item.CertificateStatus, item.LastError = ingress.StatusTargetUnavailable, "pending", "O serviço configurado ainda não está disponível."
		writeJSON(w, item)
		return
	}
	_ = h.store.MarkCheck(r.Context(), item.ID, ingress.StatusActive, "pending", "")
	item.Status, item.CertificateStatus, item.LastError = ingress.StatusActive, "pending", ""
	h.app.publish("domain.active", map[string]any{"domain_id": item.ID, "application_id": applicationID, "hostname": item.Hostname})
	h.reload(r.Context())
	writeJSON(w, item)
}

func (h *domainAPI) status(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListEnabled(r.Context())
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível carregar o ingress.")
		return
	}
	writeJSON(w, map[string]any{"enabled": len(items) > 0, "domains": len(items)})
}

func (h *domainAPI) applicationExists(ctx context.Context, id int64) error {
	var exists int
	if err := h.app.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM applications WHERE id=?)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ingress.ErrNotFound
	}
	return nil
}

func (h *domainAPI) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	userID, ok := r.Context().Value(userKey{}).(int64)
	if !ok {
		jsonError(w, 403, "forbidden", "Apenas administradores podem alterar domínios.")
		return false
	}
	var role string
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, userID).Scan(&role); err != nil || role != "admin" {
		jsonError(w, 403, "forbidden", "Apenas administradores podem alterar domínios.")
		return false
	}
	return true
}

func (h *domainAPI) writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ingress.ErrNotFound):
		jsonError(w, 404, "not_found", "Domínio não encontrado.")
	case errors.Is(err, ingress.ErrInvalidDomain):
		jsonError(w, 422, "invalid_domain", "Informe um domínio público, serviço e porta válidos.")
	default:
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			jsonError(w, 409, "domain_in_use", "Este domínio já está cadastrado.")
		} else {
			jsonError(w, 500, "internal_error", "Não foi possível salvar o domínio.")
		}
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		jsonError(w, 422, "validation_failed", "JSON inválido.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		jsonError(w, 422, "validation_failed", "Envie somente um objeto JSON.")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, value any) { writeJSONStatus(w, http.StatusOK, value) }
func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

var _ ingress.TargetResolver = dockerDomainResolver{}
