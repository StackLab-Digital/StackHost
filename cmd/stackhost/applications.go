package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/StackLab-Digital/StackHost/internal/composevalidator"
)

var variableKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type environmentVariable struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Secret   bool   `json:"secret"`
	HasValue bool   `json:"has_value,omitempty"`
}

type sourceInput struct {
	ComposeYAML     string                `json:"compose_yaml"`
	Environment     []environmentVariable `json:"environment"`
	Image           string                `json:"image"`
	Command         string                `json:"command"`
	Entrypoint      string                `json:"entrypoint"`
	ContainerPort   int                   `json:"container_port"`
	Replicas        int                   `json:"replicas"`
	RepositoryURL   string                `json:"repository_url"`
	Branch          string                `json:"branch"`
	DockerfilePath  string                `json:"dockerfile_path"`
	BuildContext    string                `json:"build_context"`
	CatalogSource   string                `json:"catalog_source"`
	TemplateSlug    string                `json:"template_slug"`
	TemplateVersion string                `json:"template_version"`
	Values          map[string]string     `json:"values"`
}

type sourceResult struct {
	Valid    bool                     `json:"valid"`
	Errors   []string                 `json:"errors"`
	Warnings []string                 `json:"warnings"`
	Summary  composevalidator.Summary `json:"summary"`
}

func (a *app) validateSourcePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var input struct {
		SourceType string      `json:"source_type"`
		Source     sourceInput `json:"source"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024+64*1024)
	if json.NewDecoder(r.Body).Decode(&input) != nil || !sourceTypeValid(input.SourceType) {
		jsonValidation(w, map[string]string{"source_type": "Selecione uma origem válida."})
		return
	}
	result := validateSource(input.SourceType, input.Source)
	json.NewEncoder(w).Encode(result)
}

func sourceTypeValid(source string) bool {
	return map[string]bool{"catalog": true, "compose": true, "image": true, "git": true}[source]
}

func validateSource(sourceType string, input sourceInput) sourceResult {
	result := sourceResult{Errors: []string{}, Warnings: []string{}, Summary: composevalidator.Summary{Services: []string{}, Images: []string{}, Ports: []string{}, Volumes: []string{}, Networks: []string{}}}
	for _, variable := range input.Environment {
		if !variableKey.MatchString(variable.Key) {
			result.Errors = append(result.Errors, fmt.Sprintf("A chave %q não é válida.", variable.Key))
		}
		if len(variable.Key) > 128 || len(variable.Value) > 64*1024 {
			result.Errors = append(result.Errors, "Variável acima do limite permitido.")
		}
	}
	if sourceType == "compose" {
		compose := composevalidator.Validate(input.ComposeYAML)
		result.Errors = append(result.Errors, compose.Errors...)
		result.Warnings = append(result.Warnings, compose.Warnings...)
		result.Summary = compose.Summary
	} else if sourceType == "image" {
		if strings.TrimSpace(input.Image) == "" {
			result.Errors = append(result.Errors, "Informe uma imagem Docker.")
		}
		if input.ContainerPort < 0 || input.ContainerPort > 65535 {
			result.Errors = append(result.Errors, "A porta deve estar entre 1 e 65535.")
		}
		if input.ContainerPort == 0 {
			input.ContainerPort = 0
		}
		if input.Replicas < 0 {
			result.Errors = append(result.Errors, "A quantidade de réplicas não pode ser negativa.")
		}
		if input.Replicas == 0 {
			input.Replicas = 1
		}
		result.Summary.Images = []string{input.Image}
	} else if sourceType == "git" {
		u, err := url.Parse(strings.TrimSpace(input.RepositoryURL))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh") || u.User != nil {
			result.Errors = append(result.Errors, "Informe uma URL Git HTTP, HTTPS ou SSH sem credenciais embutidas.")
		}
		if len(input.RepositoryURL) > 2048 {
			result.Errors = append(result.Errors, "A URL do repositório é muito longa.")
		}
		if strings.Contains(filepath.Clean(input.DockerfilePath), "..") || strings.Contains(filepath.Clean(input.BuildContext), "..") {
			result.Errors = append(result.Errors, "Os caminhos não podem sair do contexto do repositório.")
		}
	} else if sourceType == "catalog" {
		if input.CatalogSource == "" {
			input.CatalogSource = "official"
		}
		if input.TemplateSlug == "" {
			result.Errors = append(result.Errors, "Selecione um template do catálogo.")
		}
	} else {
		result.Errors = append(result.Errors, "Tipo de origem inválido.")
	}
	result.Valid = len(result.Errors) == 0
	return result
}

func sourceSummary(input sourceInput, result sourceResult) map[string]any {
	summary := map[string]any{"services": result.Summary.Services, "images": result.Summary.Images, "ports": result.Summary.Ports, "volumes": result.Summary.Volumes, "networks": result.Summary.Networks, "environment_variables": result.Summary.EnvironmentVariables}
	if input.Image != "" {
		summary["image"] = input.Image
	}
	if input.RepositoryURL != "" {
		summary["repository_url"] = input.RepositoryURL
	}
	if input.TemplateSlug != "" {
		summary["template_slug"] = input.TemplateSlug
	}
	summary["variables"] = len(input.Environment)
	return summary
}

func (a *app) applicationRouteV2(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "applications" {
		jsonError(w, 404, "not_found", "Aplicação não encontrada.")
		return
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		jsonError(w, 404, "not_found", "Aplicação não encontrada.")
		return
	}
	if len(parts) == 4 {
		if r.Method == http.MethodDelete {
			if _, err := a.db.Exec("DELETE FROM applications WHERE id=?", id); err != nil {
				jsonError(w, 500, "internal_error", "Não foi possível excluir a aplicação.")
				return
			}
			userID, _ := r.Context().Value(userKey{}).(int64)
			a.audit(userID, "application.deleted", "application", id)
			a.publish("application.deleted", map[string]any{"id": id})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		a.applicationMetadata(w, r, id)
		return
	}
	if len(parts) == 5 && parts[4] == "source" {
		a.applicationSource(w, r, id)
		return
	}
	if len(parts) == 6 && parts[4] == "source" {
		switch parts[5] {
		case "validate":
			a.validateApplicationSource(w, r, id)
		case "change":
			a.changeApplicationSource(w, r, id)
		default:
			jsonError(w, 404, "not_found", "Rota não encontrada.")
		}
		return
	}
	jsonError(w, 404, "not_found", "Rota não encontrada.")
}

func (a *app) applicationMetadata(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method == http.MethodGet {
		var projectID int64
		var name, description, slug, source, stack, created, updated, configStatus string
		var revision int
		if err := a.db.QueryRow(`SELECT project_id,name,coalesce(description,''),slug,source_type,coalesce(docker_stack_name,''),coalesce(configuration_status,'draft'),created_at,updated_at,source_revision FROM applications WHERE id=?`, id).Scan(&projectID, &name, &description, &slug, &source, &stack, &configStatus, &created, &updated, &revision); err != nil {
			jsonError(w, 404, "not_found", "Aplicação não encontrada.")
			return
		}
		var projectName string
		_ = a.db.QueryRow("SELECT name FROM projects WHERE id=?", projectID).Scan(&projectName)
		var sourceStatus, validationErrors, validationWarnings, summary string
		_ = a.db.QueryRow("SELECT coalesce(validation_status,''),coalesce(validation_errors,'[]'),coalesce(validation_warnings,'[]'),coalesce(summary_json,'{}') FROM application_sources WHERE application_id=?", id).Scan(&sourceStatus, &validationErrors, &validationWarnings, &summary)
		json.NewEncoder(w).Encode(map[string]any{"id": id, "project": map[string]any{"id": projectID, "name": projectName}, "name": name, "description": description, "slug": slug, "source_type": source, "docker_stack_name": stack, "configuration_status": configStatus, "validation_status": sourceStatus, "validation_errors": json.RawMessage(validationErrors), "validation_warnings": json.RawMessage(validationWarnings), "source_summary": json.RawMessage(summary), "source_revision": revision, "created_at": created, "updated_at": updated, "activity": a.applicationActivity(id)})
		return
	}
	if r.Method != http.MethodPatch {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	var in struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		DockerStackName string `json:"docker_stack_name"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Name) > 100 || len(in.Description) > 2000 {
		jsonValidation(w, map[string]string{"form": "Revise os dados da aplicação."})
		return
	}
	if _, err := a.db.Exec("UPDATE applications SET name=COALESCE(NULLIF(?,''),name), description=COALESCE(?,description), docker_stack_name=COALESCE(NULLIF(?,''),docker_stack_name), updated_at=? WHERE id=?", in.Name, in.Description, in.DockerStackName, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível atualizar a aplicação.")
		return
	}
	userID, _ := r.Context().Value(userKey{}).(int64)
	a.audit(userID, "application.updated", "application", id)
	a.publish("application.updated", map[string]any{"id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) applicationSource(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method == http.MethodGet {
		var sourceType string
		var encrypted, nonce []byte
		if err := a.db.QueryRow("SELECT source_type,encrypted_payload,encryption_nonce FROM application_sources WHERE application_id=?", id).Scan(&sourceType, &encrypted, &nonce); err != nil {
			_ = a.db.QueryRow("SELECT source_type FROM applications WHERE id=?", id).Scan(&sourceType)
			json.NewEncoder(w).Encode(map[string]any{"source_type": sourceType, "configured": false, "environment": []environmentVariable{}})
			return
		}
		plain, err := a.cipher.Decrypt(encrypted, nonce)
		if err != nil {
			jsonError(w, 500, "source_unreadable", "Não foi possível ler a configuração da origem.")
			return
		}
		var input sourceInput
		if json.Unmarshal(plain, &input) != nil {
			jsonError(w, 500, "source_unreadable", "A configuração da origem está inválida.")
			return
		}
		for i := range input.Environment {
			if input.Environment[i].Secret {
				input.Environment[i].Value = ""
				input.Environment[i].HasValue = true
			}
		}
		inputBytes, _ := json.Marshal(input)
		json.NewEncoder(w).Encode(map[string]any{"source_type": sourceType, "configured": true, "payload": json.RawMessage(inputBytes)})
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	if r.Method == http.MethodPost {
		a.validateApplicationSource(w, r, id)
		return
	}
	var input struct {
		SourceType string      `json:"source_type"`
		Source     sourceInput `json:"source"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024+64*1024)
	if json.NewDecoder(r.Body).Decode(&input) != nil || !sourceTypeValid(input.SourceType) {
		jsonValidation(w, map[string]string{"source_type": "Selecione uma origem válida."})
		return
	}
	var existingEncrypted, existingNonce []byte
	if err := a.db.QueryRow("SELECT encrypted_payload,encryption_nonce FROM application_sources WHERE application_id=?", id).Scan(&existingEncrypted, &existingNonce); err == nil {
		if plain, decryptErr := a.cipher.Decrypt(existingEncrypted, existingNonce); decryptErr == nil {
			var previous sourceInput
			if json.Unmarshal(plain, &previous) == nil {
				for i := range input.Source.Environment {
					for _, old := range previous.Environment {
						if input.Source.Environment[i].Key == old.Key && input.Source.Environment[i].Secret && input.Source.Environment[i].HasValue && input.Source.Environment[i].Value == "" {
							input.Source.Environment[i].Value = old.Value
						}
					}
				}
			}
		}
	}
	result := validateSource(input.SourceType, input.Source)
	a.saveApplicationSource(w, r, id, input.SourceType, input.Source, result)
}

func (a *app) saveApplicationSource(w http.ResponseWriter, r *http.Request, id int64, sourceType string, input sourceInput, result sourceResult) {
	payload, _ := json.Marshal(input)
	ciphertext, nonce, err := a.cipher.Encrypt(payload)
	if err != nil {
		jsonError(w, 500, "encryption_failed", "Não foi possível proteger a configuração.")
		return
	}
	checksumBytes := sha256.Sum256(payload)
	checksum := hex.EncodeToString(checksumBytes[:])
	summary, _ := json.Marshal(sourceSummary(input, result))
	errors, _ := json.Marshal(result.Errors)
	warnings, _ := json.Marshal(result.Warnings)
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := a.db.Begin()
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível salvar a origem.")
		return
	}
	defer tx.Rollback()
	status := "invalid"
	if result.Valid {
		status = "configured"
	}
	_, err = tx.Exec(`INSERT INTO application_sources(application_id,source_type,encrypted_payload,encryption_nonce,checksum,validation_status,validation_errors,validation_warnings,summary_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(application_id) DO UPDATE SET source_type=excluded.source_type,encrypted_payload=excluded.encrypted_payload,encryption_nonce=excluded.encryption_nonce,checksum=excluded.checksum,validation_status=excluded.validation_status,validation_errors=excluded.validation_errors,validation_warnings=excluded.validation_warnings,summary_json=excluded.summary_json,updated_at=excluded.updated_at`, id, sourceType, ciphertext, nonce, checksum, status, errors, warnings, summary, now, now)
	if err == nil {
		configuredAt, validatedAt := "", ""
		if result.Valid {
			configuredAt, validatedAt = now, now
		}
		_, err = tx.Exec("UPDATE applications SET source_type=?,configuration_status=?,configured_at=NULLIF(?,''),last_validated_at=NULLIF(?,''),source_revision=source_revision+1,updated_at=? WHERE id=?", sourceType, status, configuredAt, validatedAt, now, id)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível salvar a origem.")
		return
	}
	userID, _ := r.Context().Value(userKey{}).(int64)
	a.audit(userID, "application.source_configured", "application", id)
	a.publish("application.source_configured", map[string]any{"id": id})
	json.NewEncoder(w).Encode(map[string]any{"status": status, "checksum": checksum, "summary": sourceSummary(input, result), "errors": result.Errors, "warnings": result.Warnings})
}

func (a *app) validateApplicationSource(w http.ResponseWriter, r *http.Request, id int64) {
	var sourceType string
	var encrypted, nonce []byte
	if err := a.db.QueryRow("SELECT source_type,encrypted_payload,encryption_nonce FROM application_sources WHERE application_id=?", id).Scan(&sourceType, &encrypted, &nonce); err != nil {
		jsonError(w, 404, "source_not_configured", "Configure uma origem antes de validar.")
		return
	}
	plain, err := a.cipher.Decrypt(encrypted, nonce)
	if err != nil {
		jsonError(w, 500, "source_unreadable", "Não foi possível ler a origem.")
		return
	}
	var input sourceInput
	if json.Unmarshal(plain, &input) != nil {
		jsonError(w, 500, "source_unreadable", "A configuração da origem está inválida.")
		return
	}
	result := validateSource(sourceType, input)
	userID, _ := r.Context().Value(userKey{}).(int64)
	a.audit(userID, "application.source_validated", "application", id)
	if result.Valid {
		a.publish("application.source_validated", map[string]any{"id": id})
	} else {
		a.publish("application.source_invalid", map[string]any{"id": id})
	}
	json.NewEncoder(w).Encode(result)
}

func (a *app) changeApplicationSource(w http.ResponseWriter, r *http.Request, id int64) {
	var input struct {
		SourceType   string `json:"source_type"`
		ConfirmReset bool   `json:"confirm_reset"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || !input.ConfirmReset || !sourceTypeValid(input.SourceType) {
		jsonValidation(w, map[string]string{"confirm_reset": "Confirme a troca da origem para continuar."})
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := a.db.Exec("DELETE FROM application_sources WHERE application_id=?", id); err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível trocar a origem.")
		return
	}
	if _, err := a.db.Exec("UPDATE applications SET source_type=?,configuration_status='draft',configured_at=NULL,last_validated_at=NULL,source_revision=source_revision+1,updated_at=? WHERE id=?", input.SourceType, now, id); err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível trocar a origem.")
		return
	}
	userID, _ := r.Context().Value(userKey{}).(int64)
	a.audit(userID, "application.source_changed", "application", id)
	a.publish("application.source_changed", map[string]any{"id": id, "source_type": input.SourceType})
	json.NewEncoder(w).Encode(map[string]any{"status": "draft", "source_type": input.SourceType})
}

func (a *app) applicationActivity(id int64) []map[string]any {
	rows, err := a.db.Query("SELECT action,created_at FROM audit_logs WHERE resource_type='application' AND resource_id=? ORDER BY id DESC LIMIT 20", strconv.FormatInt(id, 10))
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var action, created string
		_ = rows.Scan(&action, &created)
		items = append(items, map[string]any{"action": action, "description": activityDescription(action), "created_at": created})
	}
	return items
}
