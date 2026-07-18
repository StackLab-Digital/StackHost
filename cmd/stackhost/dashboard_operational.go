package main

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"syscall"
	"time"
)

type hostResources struct {
	CPUPercent       *float64 `json:"cpu_percent"`
	MemoryUsedBytes  *uint64  `json:"memory_used_bytes"`
	MemoryTotalBytes *uint64  `json:"memory_total_bytes"`
	DiskUsedBytes    *uint64  `json:"disk_used_bytes"`
	DiskTotalBytes   *uint64  `json:"disk_total_bytes"`
	UptimeSeconds    *uint64  `json:"uptime_seconds"`
}

func (a *app) systemResources(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	writeJSON(w, hostResourceSnapshot(ctx, a.dataDir))
}

func (a *app) systemHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	result := map[string]any{"docker_available": false, "runtime_mode": "unknown", "swarm_active": false, "status": "unknown"}
	if mode, err := a.runtimeMode(ctx); err == nil {
		result["runtime_mode"] = mode
	}
	if a.docker != nil {
		snapshot := a.docker.Snapshot(ctx)
		result["docker_available"] = snapshot.Available
		result["docker_message"] = snapshot.Message
		result["engine_version"] = snapshot.EngineVersion
		result["swarm_active"] = snapshot.Swarm.Active
		if snapshot.Available {
			result["status"] = "healthy"
		} else {
			result["status"] = "critical"
		}
	}
	writeJSON(w, result)
}

func (a *app) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	mode, _ := a.runtimeMode(ctx)
	snapshot := map[string]any{"available": false}
	if a.docker != nil {
		info := a.docker.Snapshot(ctx)
		snapshot = map[string]any{"available": info.Available, "engine_version": info.EngineVersion, "runtime_mode": mode, "swarm_active": info.Swarm.Active, "message": info.Message, "swarm": info.Swarm}
	}
	applications := map[string]int{"total": 0, "running": 0, "degraded": 0, "stopped": 0, "failed": 0, "not_deployed": 0}
	rows, _ := a.db.QueryContext(ctx, `SELECT COALESCE(status,'not_deployed'), count(*) FROM applications GROUP BY COALESCE(status,'not_deployed')`)
	if rows != nil {
		for rows.Next() {
			var status string
			var count int
			_ = rows.Scan(&status, &count)
			applications[status] += count
			applications["total"] += count
		}
		rows.Close()
	}
	deployments := map[string]any{"running": scalarInt(ctx, a.db, `SELECT count(*) FROM deployments WHERE status IN ('queued','preparing','deploying','waiting')`), "failed_last_24h": scalarInt(ctx, a.db, `SELECT count(*) FROM deployments WHERE status='failed' AND created_at >= ?`, time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339)), "recent": recentDeployments(ctx, a.db)}
	domains := map[string]int{"total": 0, "active": 0, "pending": 0, "errors": 0, "certificate_errors": 0}
	if rows, err := a.db.QueryContext(ctx, `SELECT status, certificate_status, count(*) FROM application_domains GROUP BY status, certificate_status`); err == nil {
		for rows.Next() {
			var status, certificateStatus string
			var count int
			_ = rows.Scan(&status, &certificateStatus, &count)
			domains["total"] += count
			if certificateStatus == "error" || certificateStatus == "failed" {
				domains["certificate_errors"] += count
			}
			switch status {
			case "active", "ready":
				domains["active"] += count
			case "error", "failed":
				domains["errors"] += count
			default:
				domains["pending"] += count
			}
		}
		rows.Close()
	}
	backup := map[string]any{"last_status": "", "last_created_at": "", "next_scheduled_at": "", "schedule": "manual"}
	var backupStatus, backupCreated string
	if a.db.QueryRowContext(ctx, `SELECT status,created_at FROM backups ORDER BY created_at DESC LIMIT 1`).Scan(&backupStatus, &backupCreated) == nil {
		backup["last_status"] = backupStatus
		backup["last_created_at"] = backupCreated
	}
	var backupSchedule string
	if a.db.QueryRowContext(ctx, `SELECT schedule FROM backup_settings WHERE id=1`).Scan(&backupSchedule) == nil {
		backup["schedule"] = backupSchedule
		if backupSchedule != "manual" && backupCreated != "" {
			if created, err := time.Parse(time.RFC3339Nano, backupCreated); err == nil {
				interval := 24 * time.Hour
				if backupSchedule == "weekly" {
					interval = 7 * 24 * time.Hour
				}
				backup["next_scheduled_at"] = created.Add(interval).Format(time.RFC3339Nano)
			}
		}
	}
	alerts := dashboardAlerts(applications, domains, deployments, snapshot, backup)
	resources := hostResourceSnapshot(ctx, a.dataDir)
	alerts = append(alerts, resourceAlerts(resources)...)
	sortAlerts(alerts)
	writeJSON(w, map[string]any{"docker": snapshot, "host": resources, "applications": applications, "deployments": deployments, "domains": domains, "backups": backup, "alerts": alerts, "activity": recentActivity(ctx, a.db), "projects": scalarInt(ctx, a.db, `SELECT count(*) FROM projects`), "application_count": applications["total"]})
}

func (a *app) runtimeMode(ctx context.Context) (string, error) {
	var mode string
	err := a.db.QueryRowContext(ctx, `SELECT runtime_mode FROM environment_settings WHERE id=1`).Scan(&mode)
	return mode, err
}
func scalarInt(ctx context.Context, db *sql.DB, query string, args ...any) int {
	var n int
	_ = db.QueryRowContext(ctx, query, args...).Scan(&n)
	return n
}

func recentDeployments(ctx context.Context, db *sql.DB) []any {
	items := []any{}
	rows, err := db.QueryContext(ctx, `SELECT d.id,d.application_id,d.status,d.source_revision,d.created_at,COALESCE(a.name,'') FROM deployments d LEFT JOIN applications a ON a.id=d.application_id ORDER BY d.created_at DESC LIMIT 8`)
	if err != nil {
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var id, applicationID, revision int64
		var status, created, name string
		if rows.Scan(&id, &applicationID, &status, &revision, &created, &name) == nil {
			items = append(items, map[string]any{"id": id, "application_id": applicationID, "application": name, "status": status, "revision": revision, "created_at": created})
		}
	}
	return items
}

func recentActivity(ctx context.Context, db *sql.DB) []any {
	items := []any{}
	rows, err := db.QueryContext(ctx, `SELECT action,COALESCE(resource_type,''),created_at FROM audit_logs ORDER BY id DESC LIMIT 5`)
	if err != nil {
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var action, resource, created string
		if rows.Scan(&action, &resource, &created) == nil {
			items = append(items, map[string]string{"action": action, "resource_type": resource, "created_at": created, "description": activityDescription(action)})
		}
	}
	return items
}

func dashboardAlerts(applications, domains map[string]int, deployments, docker, backup map[string]any) []map[string]any {
	alerts := []map[string]any{}
	if available, _ := docker["available"].(bool); !available {
		alerts = append(alerts, map[string]any{"severity": "critical", "code": "docker_unavailable", "title": "Docker indisponível", "description": "O daemon Docker não respondeu.", "resource_type": "system", "action_url": "/infrastructure"})
	}
	if mode, _ := docker["runtime_mode"].(string); mode == "swarm" {
		if active, _ := docker["swarm_active"].(bool); !active {
			alerts = append(alerts, map[string]any{"severity": "critical", "code": "swarm_unavailable", "title": "Swarm indisponível", "description": "O modo Swarm está selecionado, mas o cluster não está ativo.", "resource_type": "system", "action_url": "/infrastructure?tab=swarm"})
		}
	}
	if n := applications["degraded"]; n > 0 {
		alerts = append(alerts, map[string]any{"severity": "warning", "code": "application_degraded", "title": "Aplicação degradada", "description": strconv.Itoa(n) + " aplicação(ões) precisam de atenção.", "resource_type": "application", "action_url": "/applications"})
	}
	if n, _ := deployments["failed_last_24h"].(int); n > 0 {
		alerts = append(alerts, map[string]any{"severity": "warning", "code": "deploy_failed", "title": "Deploy falhou recentemente", "description": strconv.Itoa(n) + " deploy(s) falharam nas últimas 24 horas.", "resource_type": "system", "action_url": "/activity"})
	}
	if n := domains["errors"]; n > 0 {
		alerts = append(alerts, map[string]any{"severity": "warning", "code": "domain_error", "title": "Domínio com erro", "description": strconv.Itoa(n) + " domínio(s) apresentam erro.", "resource_type": "domain", "action_url": "/applications"})
	}
	if n := domains["certificate_errors"]; n > 0 {
		alerts = append(alerts, map[string]any{"severity": "warning", "code": "certificate_error", "title": "Certificado com erro", "description": strconv.Itoa(n) + " certificado(s) apresentam erro.", "resource_type": "domain", "action_url": "/applications"})
	}
	if status, _ := backup["last_status"].(string); status == "failed" {
		alerts = append(alerts, map[string]any{"severity": "warning", "code": "backup_failed", "title": "Backup falhou", "description": "O último backup não foi concluído.", "resource_type": "backup", "action_url": "/settings"})
	}
	if next, _ := backup["next_scheduled_at"].(string); next != "" {
		if scheduled, err := time.Parse(time.RFC3339Nano, next); err == nil && time.Now().After(scheduled) {
			alerts = append(alerts, map[string]any{"severity": "warning", "code": "backup_overdue", "title": "Backup atrasado", "description": "O backup agendado ainda não foi executado.", "resource_type": "backup", "action_url": "/settings"})
		}
	}
	return alerts
}

func sortAlerts(alerts []map[string]any) {
	priority := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(alerts, func(i, j int) bool {
		return priority[alerts[i]["severity"].(string)] < priority[alerts[j]["severity"].(string)]
	})
}

func resourceAlerts(resources hostResources) []map[string]any {
	alerts := []map[string]any{}
	if resources.DiskTotalBytes != nil && resources.DiskUsedBytes != nil && *resources.DiskTotalBytes > 0 && float64(*resources.DiskUsedBytes)/float64(*resources.DiskTotalBytes) >= .85 {
		alerts = append(alerts, map[string]any{"severity": "warning", "code": "disk_high", "title": "Espaço em disco baixo", "description": "O volume de dados está acima de 85% de utilização.", "resource_type": "system", "action_url": "/infrastructure"})
	}
	if resources.MemoryTotalBytes != nil && resources.MemoryUsedBytes != nil && *resources.MemoryTotalBytes > 0 && float64(*resources.MemoryUsedBytes)/float64(*resources.MemoryTotalBytes) >= .90 {
		alerts = append(alerts, map[string]any{"severity": "warning", "code": "memory_high", "title": "Memória acima de 90%", "description": "A memória disponível do host está baixa.", "resource_type": "system", "action_url": "/infrastructure"})
	}
	return alerts
}

func hostResourceSnapshot(ctx context.Context, path string) hostResources {
	result := hostResources{}
	result.CPUPercent = readCPUPercent(ctx)
	result.MemoryUsedBytes, result.MemoryTotalBytes = readHostMemory()
	result.UptimeSeconds = readHostUptime()
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err == nil {
		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bavail * uint64(stat.Bsize)
		used := total - free
		result.DiskTotalBytes = &total
		result.DiskUsedBytes = &used
	}
	return result
}
