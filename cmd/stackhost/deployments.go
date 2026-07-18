package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/StackLab-Digital/StackHost/internal/deployment"
)

type deploymentService interface {
	Queue(context.Context, deployment.QueueRequest) (deployment.Deployment, error)
	Get(context.Context, int64) (deployment.Deployment, error)
	ListByApplication(context.Context, int64, int) ([]deployment.Deployment, error)
	Cancel(context.Context, int64) error
}

type deploymentOperator interface {
	Operate(context.Context, deployment.Operation, deployment.RuntimeRequest) error
}

type deploymentAPI struct {
	app       *app
	engine    deploymentService
	logs      deployment.LogReader
	preflight func(context.Context, deployment.RuntimeMode) error
}

func newDeploymentAPI(app *app, engine deploymentService) *deploymentAPI {
	api := &deploymentAPI{app: app, engine: engine}
	api.preflight = api.dockerPreflight
	return api
}

var (
	errDockerUnavailable = errors.New("docker unavailable")
	errSwarmUnavailable  = errors.New("swarm unavailable")
)

// Minimal wiring intentionally lives outside this file: construct SQLStore,
// DockerRunner and Engine in main; call Engine.Recover at startup and Close at
// shutdown; then register these handlers before the existing application prefix:
//
//	GET|POST /api/v1/applications/{id}/deployments -> applicationDeployments
//	GET|POST /api/v1/deployments/{id}[/{cancel}]   -> deploymentRoute
//
// Pass deployment.EmitterFunc(a.publishDeploymentEvent) to the engine options.
func (h *deploymentAPI) applicationDeployments(w http.ResponseWriter, r *http.Request) {
	applicationID, ok := applicationDeploymentID(r.URL.Path)
	if !ok {
		jsonError(w, http.StatusNotFound, "not_found", "Aplicação não encontrada.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.list(w, r, applicationID)
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		h.create(w, r, applicationID)
	default:
		jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
	}
}

func (h *deploymentAPI) deploymentRoute(w http.ResponseWriter, r *http.Request) {
	id, cancelRoute, ok := deploymentPath(r.URL.Path)
	if !ok {
		jsonError(w, http.StatusNotFound, "not_found", "Deploy não encontrado.")
		return
	}
	if cancelRoute {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
			return
		}
		if !h.requireAdmin(w, r) {
			return
		}
		h.cancel(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
		return
	}
	h.get(w, r, id)
}

func (h *deploymentAPI) create(w http.ResponseWriter, r *http.Request, applicationID int64) {
	var input struct {
		SourceRevision *int64 `json:"source_revision"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		jsonError(w, http.StatusUnprocessableEntity, "validation_failed", "Envie somente a revisão atual da origem.")
		return
	} else if err == nil {
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			jsonError(w, http.StatusUnprocessableEntity, "validation_failed", "Envie somente um objeto JSON.")
			return
		}
	}

	var sourceType, slug, stackName, configurationStatus string
	var sourceRevision int64
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT source_type,slug,coalesce(docker_stack_name,''),coalesce(configuration_status,'draft'),source_revision FROM applications WHERE id=?`, applicationID).Scan(&sourceType, &slug, &stackName, &configurationStatus, &sourceRevision); errors.Is(err, sql.ErrNoRows) {
		jsonError(w, http.StatusNotFound, "not_found", "Aplicação não encontrada.")
		return
	} else if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível preparar a publicação.")
		return
	}
	if sourceType != "compose" || configurationStatus != "configured" || sourceRevision <= 0 {
		jsonError(w, http.StatusUnprocessableEntity, "compose_required", "Configure uma origem Docker Compose válida antes de publicar.")
		return
	}
	if input.SourceRevision != nil && *input.SourceRevision != sourceRevision {
		jsonError(w, http.StatusConflict, "source_revision_changed", "A origem mudou. Atualize a página antes de publicar.")
		return
	}

	var storedSourceType, validationStatus string
	var encrypted, nonce []byte
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT source_type,validation_status,encrypted_payload,encryption_nonce FROM application_sources WHERE application_id=?`, applicationID).Scan(&storedSourceType, &validationStatus, &encrypted, &nonce); errors.Is(err, sql.ErrNoRows) {
		jsonError(w, http.StatusUnprocessableEntity, "compose_required", "Configure uma origem Docker Compose válida antes de publicar.")
		return
	} else if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível preparar a publicação.")
		return
	}
	if storedSourceType != "compose" || validationStatus != "configured" {
		jsonError(w, http.StatusUnprocessableEntity, "compose_invalid", "O Compose precisa ser válido antes da publicação.")
		return
	}
	plain, err := h.app.cipher.Decrypt(encrypted, nonce)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "source_unreadable", "Não foi possível ler o Compose.")
		return
	}
	var source sourceInput
	if err := json.Unmarshal(plain, &source); err != nil {
		jsonError(w, http.StatusInternalServerError, "source_unreadable", "Não foi possível ler o Compose.")
		return
	}
	if result := validateSource("compose", source); !result.Valid {
		jsonError(w, http.StatusUnprocessableEntity, "compose_invalid", "O Compose precisa ser válido e ter todas as variáveis obrigatórias.")
		return
	}
	var currentRevision int64
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT source_revision FROM applications WHERE id=?`, applicationID).Scan(&currentRevision); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível confirmar a revisão da origem.")
		return
	}
	if currentRevision != sourceRevision {
		jsonError(w, http.StatusConflict, "source_revision_changed", "A origem mudou. Atualize a página antes de publicar.")
		return
	}
	if stackName == "" {
		stackName = slug
	}
	if !deployment.ValidStackName(stackName) {
		jsonError(w, http.StatusUnprocessableEntity, "invalid_project_name", "O nome do projeto Docker é inválido.")
		return
	}
	var stackInUse int
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM applications WHERE id<>? AND COALESCE(NULLIF(docker_stack_name,''),slug)=?)`, applicationID, stackName).Scan(&stackInUse); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível validar o nome do projeto Docker.")
		return
	}
	if stackInUse != 0 {
		jsonError(w, http.StatusConflict, "stack_name_in_use", "Outra aplicação já usa este nome de projeto Docker.")
		return
	}

	mode := string(deployment.RuntimeStandalone)
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT runtime_mode FROM environment_settings WHERE id=1`).Scan(&mode); err != nil {
		jsonError(w, http.StatusServiceUnavailable, "runtime_unavailable", "O modo de execução não está disponível.")
		return
	}
	if mode != string(deployment.RuntimeStandalone) && mode != string(deployment.RuntimeSwarm) {
		jsonError(w, http.StatusUnprocessableEntity, "invalid_runtime_mode", "O modo de execução configurado é inválido.")
		return
	}
	if err := h.preflight(r.Context(), deployment.RuntimeMode(mode)); errors.Is(err, errDockerUnavailable) {
		jsonError(w, http.StatusServiceUnavailable, "docker_unavailable", "O Docker não está disponível.")
		return
	} else if errors.Is(err, errSwarmUnavailable) {
		jsonError(w, http.StatusServiceUnavailable, "swarm_unavailable", "O Swarm precisa estar ativo em um manager.")
		return
	} else if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "runtime_unavailable", "Não foi possível validar o runtime.")
		return
	}

	environment := make(map[string]string, len(source.Environment))
	secretValues := make([]string, 0)
	for _, variable := range source.Environment {
		if variable.Value == "" {
			continue
		}
		environment[variable.Key] = variable.Value
		if variable.Secret {
			secretValues = append(secretValues, variable.Value)
		}
	}
	queued, err := h.engine.Queue(r.Context(), deployment.QueueRequest{
		ApplicationID:  applicationID,
		RuntimeMode:    deployment.RuntimeMode(mode),
		SourceRevision: sourceRevision,
		StackName:      stackName,
		TriggerType:    "manual",
		Compose:        source.ComposeYAML,
		Environment:    environment,
		SecretValues:   secretValues,
	})
	if err != nil {
		h.writeEngineError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(queued)
}

func (h *deploymentAPI) dockerPreflight(ctx context.Context, mode deployment.RuntimeMode) error {
	if h.app.docker == nil {
		return errDockerUnavailable
	}
	snapshot := h.app.docker.Snapshot(ctx)
	if !snapshot.Available {
		return errDockerUnavailable
	}
	if mode == deployment.RuntimeSwarm && (!snapshot.Swarm.Active || !snapshot.Swarm.ControlAvailable) {
		return errSwarmUnavailable
	}
	return nil
}

func (h *deploymentAPI) list(w http.ResponseWriter, r *http.Request, applicationID int64) {
	var exists int
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM applications WHERE id=?)`, applicationID).Scan(&exists); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível listar os deploys.")
		return
	}
	if exists == 0 {
		jsonError(w, http.StatusNotFound, "not_found", "Aplicação não encontrada.")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.engine.ListByApplication(r.Context(), applicationID, limit)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível listar os deploys.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

func (h *deploymentAPI) get(w http.ResponseWriter, r *http.Request, id int64) {
	item, err := h.engine.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, deployment.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found", "Deploy não encontrado.")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível carregar o deploy.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(item)
}

func (h *deploymentAPI) cancel(w http.ResponseWriter, r *http.Request, id int64) {
	if err := h.engine.Cancel(r.Context(), id); err != nil {
		if errors.Is(err, deployment.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found", "Deploy não encontrado.")
			return
		}
		if errors.Is(err, deployment.ErrNotCancellable) {
			jsonError(w, http.StatusConflict, "deployment_not_cancellable", "Este deploy não pode mais ser cancelado.")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível cancelar o deploy.")
		return
	}
	item, err := h.engine.Get(r.Context(), id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível carregar o deploy.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(item)
}

func (h *deploymentAPI) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	userID, ok := r.Context().Value(userKey{}).(int64)
	if !ok {
		jsonError(w, http.StatusForbidden, "forbidden", "Apenas administradores podem executar esta ação.")
		return false
	}
	var role string
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT role FROM users WHERE id=?`, userID).Scan(&role); err != nil || role != "admin" {
		jsonError(w, http.StatusForbidden, "forbidden", "Apenas administradores podem executar esta ação.")
		return false
	}
	return true
}

func (h *deploymentAPI) writeEngineError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, deployment.ErrNotFound):
		jsonError(w, http.StatusNotFound, "not_found", "Aplicação não encontrada.")
	case errors.Is(err, deployment.ErrApplicationBusy):
		jsonError(w, http.StatusConflict, "deployment_in_progress", "Já existe um deploy em andamento para esta aplicação.")
	case errors.Is(err, deployment.ErrInvalidRequest):
		jsonError(w, http.StatusUnprocessableEntity, "invalid_deployment", "A configuração da publicação é inválida.")
	default:
		jsonError(w, http.StatusBadGateway, "runtime_error", "O Docker não conseguiu executar a operação.")
	}
}

func (h *deploymentAPI) runtimeRequest(ctx context.Context, applicationID int64) (deployment.RuntimeRequest, error) {
	var sourceType, slug, stackName, configurationStatus string
	var sourceRevision int64
	if err := h.app.db.QueryRowContext(ctx, `SELECT source_type,slug,coalesce(docker_stack_name,''),coalesce(configuration_status,'draft'),source_revision FROM applications WHERE id=?`, applicationID).Scan(&sourceType, &slug, &stackName, &configurationStatus, &sourceRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return deployment.RuntimeRequest{}, deployment.ErrNotFound
		}
		return deployment.RuntimeRequest{}, err
	}
	if sourceType != "compose" || configurationStatus != "configured" || sourceRevision <= 0 {
		return deployment.RuntimeRequest{}, deployment.ErrInvalidRequest
	}
	if stackName == "" {
		stackName = slug
	}
	var encrypted, nonce []byte
	if err := h.app.db.QueryRowContext(ctx, `SELECT encrypted_payload,encryption_nonce FROM application_sources WHERE application_id=?`, applicationID).Scan(&encrypted, &nonce); err != nil {
		return deployment.RuntimeRequest{}, err
	}
	plain, err := h.app.cipher.Decrypt(encrypted, nonce)
	if err != nil {
		return deployment.RuntimeRequest{}, err
	}
	var source sourceInput
	if err := json.Unmarshal(plain, &source); err != nil {
		return deployment.RuntimeRequest{}, err
	}
	if result := validateSource("compose", source); !result.Valid {
		return deployment.RuntimeRequest{}, deployment.ErrInvalidRequest
	}
	mode := string(deployment.RuntimeStandalone)
	_ = h.app.db.QueryRowContext(ctx, `SELECT runtime_mode FROM environment_settings WHERE id=1`).Scan(&mode)
	environment := make(map[string]string, len(source.Environment))
	for _, variable := range source.Environment {
		if variable.Value != "" {
			environment[variable.Key] = variable.Value
		}
	}
	return deployment.RuntimeRequest{ApplicationID: applicationID, RuntimeMode: deployment.RuntimeMode(mode), StackName: stackName, Compose: source.ComposeYAML, Environment: environment}, nil
}

func (h *deploymentAPI) action(w http.ResponseWriter, r *http.Request, applicationID int64, operation deployment.Operation) {
	operator, ok := h.engine.(deploymentOperator)
	if !ok {
		jsonError(w, 503, "runtime_unavailable", "O motor de operações não está disponível.")
		return
	}
	request, err := h.runtimeRequest(r.Context(), applicationID)
	if err != nil {
		h.writeEngineError(w, err)
		return
	}
	if err := operator.Operate(r.Context(), operation, request); err != nil {
		h.writeEngineError(w, err)
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]any{"status": "ok", "operation": operation})
}

func (a *app) publishDeploymentEvent(event deployment.Event) {
	if event.DeploymentID == 0 {
		a.publish(event.Name, map[string]any{"application_id": event.ApplicationID, "status": event.Status})
		return
	}
	a.publish(event.Name, map[string]any{
		"deployment_id":  event.DeploymentID,
		"application_id": event.ApplicationID,
		"status":         event.Status,
		"deployment":     event.Deployment,
	})
}

func applicationDeploymentID(path string) (int64, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "applications" || parts[4] != "deployments" {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	return id, err == nil && id > 0
}

func deploymentPath(path string) (id int64, cancel bool, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || len(parts) > 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "deployments" {
		return 0, false, false
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || id <= 0 {
		return 0, false, false
	}
	if len(parts) == 5 && parts[4] != "cancel" {
		return 0, false, false
	}
	return id, len(parts) == 5, true
}
