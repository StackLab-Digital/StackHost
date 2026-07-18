package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type backupAPI struct {
	app     *app
	dataDir string
}

func (a *app) backupRoute(w http.ResponseWriter, r *http.Request) {
	if a.backups == nil {
		jsonError(w, http.StatusServiceUnavailable, "runtime_unavailable", "Backups não estão disponíveis.")
		return
	}
	userID, ok := r.Context().Value(userKey{}).(int64)
	if !ok || !a.isAdmin(userID) {
		jsonError(w, http.StatusForbidden, "forbidden", "Apenas administradores podem acessar backups.")
		return
	}
	a.backups.route(w, r)
}

func (a *app) isAdmin(userID int64) bool {
	var role string
	return a.db.QueryRow(`SELECT role FROM users WHERE id=?`, userID).Scan(&role) == nil && role == "admin"
}

func (h *backupAPI) route(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 4 {
		if r.Method == http.MethodPost {
			h.create(w, r)
			return
		}
		if r.Method == http.MethodGet {
			h.list(w, r)
			return
		}
	}
	if len(parts) != 5 && len(parts) != 6 {
		jsonError(w, 404, "not_found", "Backup não encontrado.")
		return
	}
	id, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil || id <= 0 {
		jsonError(w, 404, "not_found", "Backup não encontrado.")
		return
	}
	switch {
	case len(parts) == 6 && parts[5] == "download" && r.Method == http.MethodGet:
		h.download(w, r, id)
	case len(parts) == 5 && r.Method == http.MethodDelete:
		h.remove(w, r, id)
	default:
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
	}
}

func (h *backupAPI) create(w http.ResponseWriter, r *http.Request) {
	if err := os.MkdirAll(filepath.Join(h.dataDir, "backups"), 0750); err != nil {
		jsonError(w, 500, "backup_failed", "Não foi possível preparar o backup.")
		return
	}
	now := time.Now().UTC()
	path := filepath.Join(h.dataDir, "backups", "stackhost-"+now.Format("20060102-150405")+".tar.gz")
	if err := h.writeArchive(r.Context(), path); err != nil {
		jsonError(w, 500, "backup_failed", "Não foi possível criar o backup.")
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		jsonError(w, 500, "backup_failed", "Não foi possível validar o backup.")
		return
	}
	result, err := h.app.db.ExecContext(r.Context(), `INSERT INTO backups(kind,status,path,size_bytes,created_at) VALUES('system','ready',?,?,?)`, path, info.Size(), now.Format(time.RFC3339Nano))
	if err != nil {
		_ = os.Remove(path)
		jsonError(w, 500, "backup_failed", "Não foi possível registrar o backup.")
		return
	}
	id, _ := result.LastInsertId()
	h.app.publish("backup.succeeded", map[string]any{"backup_id": id, "size_bytes": info.Size()})
	h.getAndWrite(w, r.Context(), id)
}

func (h *backupAPI) writeArchive(ctx context.Context, destination string) error {
	tempDB := filepath.Join(h.dataDir, ".backup-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".db")
	defer os.Remove(tempDB)
	if _, err := h.app.db.ExecContext(ctx, `VACUUM INTO ?`, tempDB); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipWriter := gzip.NewWriter(file)
	archive := tar.NewWriter(gzipWriter)
	if err := addTarFile(archive, tempDB, "stackhost.db"); err != nil {
		_ = archive.Close()
		_ = gzipWriter.Close()
		return err
	}
	certDir := filepath.Join(h.dataDir, "certificates")
	var walkErr error
	if _, err := os.Stat(certDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = archive.Close()
		_ = gzipWriter.Close()
		return err
	} else if err == nil {
		walkErr = filepath.Walk(certDir, func(path string, info os.FileInfo, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil || info.IsDir() {
				return err
			}
			relative, err := filepath.Rel(certDir, path)
			if err != nil {
				return err
			}
			return addTarFile(archive, path, filepath.ToSlash(filepath.Join("certificates", relative)))
		})
	}
	archiveErr := archive.Close()
	gzipErr := gzipWriter.Close()
	if walkErr != nil {
		return walkErr
	}
	if archiveErr != nil {
		return archiveErr
	}
	return gzipErr
}

func addTarFile(archive *tar.Writer, path, name string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name, header.Mode = name, 0600
	if err := archive.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(archive, file)
	return err
}

func (h *backupAPI) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.app.db.QueryContext(r.Context(), `SELECT id,kind,status,size_bytes,error_message,created_at FROM backups ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível listar os backups.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, size int64
		var kind, status, message, created string
		if err := rows.Scan(&id, &kind, &status, &size, &message, &created); err != nil {
			continue
		}
		items = append(items, map[string]any{"id": id, "kind": kind, "status": status, "size_bytes": size, "error_message": message, "created_at": created})
	}
	writeJSON(w, items)
}

func (h *backupAPI) getAndWrite(w http.ResponseWriter, ctx context.Context, id int64) {
	var kind, status, path, message, created string
	var size int64
	if err := h.app.db.QueryRowContext(ctx, `SELECT kind,status,path,size_bytes,error_message,created_at FROM backups WHERE id=?`, id).Scan(&kind, &status, &path, &size, &message, &created); err != nil {
		jsonError(w, 404, "not_found", "Backup não encontrado.")
		return
	}
	writeJSON(w, map[string]any{"id": id, "kind": kind, "status": status, "size_bytes": size, "error_message": message, "created_at": created})
}

func (h *backupAPI) download(w http.ResponseWriter, r *http.Request, id int64) {
	var path string
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT path FROM backups WHERE id=? AND status='ready'`, id).Scan(&path); err != nil {
		jsonError(w, 404, "not_found", "Backup não encontrado.")
		return
	}
	if filepath.Dir(path) != filepath.Join(h.dataDir, "backups") {
		jsonError(w, 404, "not_found", "Backup não encontrado.")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	http.ServeFile(w, r, path)
}

func (h *backupAPI) remove(w http.ResponseWriter, r *http.Request, id int64) {
	var path string
	if err := h.app.db.QueryRowContext(r.Context(), `SELECT path FROM backups WHERE id=?`, id).Scan(&path); err != nil {
		jsonError(w, 404, "not_found", "Backup não encontrado.")
		return
	}
	if filepath.Dir(path) == filepath.Join(h.dataDir, "backups") {
		_ = os.Remove(path)
	}
	if _, err := h.app.db.ExecContext(r.Context(), `DELETE FROM backups WHERE id=?`, id); err != nil {
		jsonError(w, 500, "internal_error", "Não foi possível remover o backup.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
