package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/StackLab-Digital/StackHost/internal/deployment"
)

func (h *deploymentAPI) logsRoute(w http.ResponseWriter, r *http.Request, applicationID int64, service string) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
		return
	}
	reader, ok := h.logs.(deployment.LogReader)
	if !ok {
		jsonError(w, 503, "runtime_unavailable", "O leitor de logs não está disponível.")
		return
	}
	request, err := h.runtimeRequest(r.Context(), applicationID)
	if err != nil {
		h.writeEngineError(w, err)
		return
	}
	services, err := deployment.ServiceNames(request.Compose)
	if err != nil {
		jsonError(w, 422, "compose_invalid", "O Compose é inválido.")
		return
	}
	if service != "" {
		if _, exists := services[service]; !exists {
			jsonError(w, 404, "service_not_found", "Serviço não encontrado nesta aplicação.")
			return
		}
	}
	if service == "" {
		writeJSON(w, services)
		return
	}
	tail := 100
	if raw := r.URL.Query().Get("tail"); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || (value != 100 && value != 500 && value != 1000) {
			jsonError(w, http.StatusBadRequest, "invalid_tail", "Escolha 100, 500 ou 1000 linhas.")
			return
		}
		tail = value
	}
	logs, err := reader.Logs(r.Context(), request, service, tail)
	if err != nil {
		jsonError(w, 502, "logs_unavailable", "Não foi possível carregar os logs do serviço.")
		return
	}
	if len(logs) > deployment.DefaultOutputLimit {
		logs = logs[:deployment.DefaultOutputLimit]
	}
	writeJSON(w, map[string]string{"logs": logs})
}

func (h *deploymentAPI) logsStream(w http.ResponseWriter, r *http.Request, applicationID int64, service string) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	request, err := h.runtimeRequest(r.Context(), applicationID)
	if err != nil {
		h.writeEngineError(w, err)
		return
	}
	services, err := deployment.ServiceNames(request.Compose)
	if err != nil {
		jsonError(w, 422, "compose_invalid", "O Compose é inválido.")
		return
	}
	if _, ok := services[service]; !ok {
		jsonError(w, 404, "service_not_found", "Serviço não encontrado nesta aplicação.")
		return
	}
	reader, ok := h.logs.(deployment.LogReader)
	if !ok {
		jsonError(w, 503, "runtime_unavailable", "O leitor de logs não está disponível.")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, 500, "stream_unavailable", "Streaming não está disponível.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	logs, err := reader.Logs(r.Context(), request, service, 100)
	if err != nil {
		jsonError(w, 502, "logs_unavailable", "Não foi possível carregar os logs do serviço.")
		return
	}
	for _, line := range strings.Split(logs, "\n") {
		payload, _ := json.Marshal(map[string]string{"line": line})
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}
}
