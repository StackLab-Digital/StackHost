package main

import (
	"net/http"
	"time"
)

func (h *deploymentAPI) metricsRoute(w http.ResponseWriter, r *http.Request, applicationID int64) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método não permitido.")
		return
	}
	if h.app.docker == nil {
		jsonError(w, http.StatusServiceUnavailable, "docker_unavailable", "Docker não está conectado.")
		return
	}
	request, err := h.runtimeRequest(r.Context(), applicationID)
	if err != nil {
		h.writeEngineError(w, err)
		return
	}
	service := r.URL.Query().Get("service")
	current, err := h.app.docker.RuntimeMetrics(r.Context(), string(request.RuntimeMode), request.StackName, service)
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, "metrics_unavailable", "Métricas locais não estão disponíveis neste runtime.")
		return
	}
	now := time.Now().UTC()
	for _, metric := range current {
		_, _ = h.app.db.ExecContext(r.Context(), `INSERT INTO runtime_metrics(application_id,service_name,cpu_percent,memory_bytes,memory_limit_bytes,network_rx_bytes,network_tx_bytes,recorded_at) VALUES(?,?,?,?,?,?,?,?)`, applicationID, metric.Service, metric.CPUPercent, metric.MemoryBytes, metric.MemoryLimitBytes, metric.NetworkRxBytes, metric.NetworkTxBytes, now.Format(time.RFC3339Nano))
	}
	_, _ = h.app.db.ExecContext(r.Context(), `DELETE FROM runtime_metrics WHERE recorded_at < ?`, now.Add(-24*time.Hour).Format(time.RFC3339Nano))
	rows, err := h.app.db.QueryContext(r.Context(), `SELECT service_name,cpu_percent,memory_bytes,memory_limit_bytes,network_rx_bytes,network_tx_bytes,recorded_at FROM runtime_metrics WHERE application_id=? AND (?='' OR service_name=?) ORDER BY recorded_at DESC LIMIT 240`, applicationID, service, service)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal_error", "Não foi possível ler o histórico de métricas.")
		return
	}
	defer rows.Close()
	history := make([]map[string]any, 0)
	for rows.Next() {
		var name, recorded string
		var cpu float64
		var memory, limit, rx, tx int64
		if rows.Scan(&name, &cpu, &memory, &limit, &rx, &tx, &recorded) == nil {
			history = append(history, map[string]any{"service": name, "cpu_percent": cpu, "memory_bytes": memory, "memory_limit_bytes": limit, "network_rx_bytes": rx, "network_tx_bytes": tx, "recorded_at": recorded})
		}
	}
	writeJSON(w, map[string]any{"available": true, "runtime_mode": request.RuntimeMode, "current": current, "history": history, "retention_hours": 24})
}
