package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
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
	domains := map[string]int{"total": 0, "active": 0, "pending": 0, "errors": 0}
	if rows, err := a.db.QueryContext(ctx, `SELECT status, count(*) FROM application_domains GROUP BY status`); err == nil {
		for rows.Next() {
			var status string
			var count int
			_ = rows.Scan(&status, &count)
			domains["total"] += count
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
	backup := map[string]any{"last_status": "", "last_created_at": "", "next_scheduled_at": ""}
	var backupStatus, backupCreated string
	if a.db.QueryRowContext(ctx, `SELECT status,created_at FROM backups ORDER BY created_at DESC LIMIT 1`).Scan(&backupStatus, &backupCreated) == nil {
		backup["last_status"] = backupStatus
		backup["last_created_at"] = backupCreated
	}
	alerts := dashboardAlerts(applications, domains, deployments, snapshot)
	resources := hostResourceSnapshot(ctx, a.dataDir)
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

func dashboardAlerts(applications, domains map[string]int, deployments, docker map[string]any) []map[string]any {
	alerts := []map[string]any{}
	if available, _ := docker["available"].(bool); !available {
		alerts = append(alerts, map[string]any{"severity": "critical", "code": "docker_unavailable", "title": "Docker indisponível", "description": "O daemon Docker não respondeu.", "resource_type": "system", "action_url": "/infrastructure"})
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
	return alerts
}

func hostResourceSnapshot(ctx context.Context, path string) hostResources {
	result := hostResources{}
	if runtime.GOOS == "linux" {
		if file, err := os.Open("/proc/meminfo"); err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			var total, available uint64
			for scanner.Scan() {
				fields := strings.Fields(scanner.Text())
				if len(fields) >= 2 {
					value, _ := strconv.ParseUint(fields[1], 10, 64)
					switch fields[0] {
					case "MemTotal:":
						total = value * 1024
					case "MemAvailable:":
						available = value * 1024
					}
				}
			}
			if total > 0 {
				used := total - available
				result.MemoryTotalBytes = &total
				result.MemoryUsedBytes = &used
			}
		}
		if file, err := os.Open("/proc/uptime"); err == nil {
			defer file.Close()
			var seconds float64
			_, _ = fmt.Fscan(file, &seconds)
			value := uint64(seconds)
			result.UptimeSeconds = &value
		}
	}
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
