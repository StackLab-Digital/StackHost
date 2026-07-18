package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *app) applicationStorage(w http.ResponseWriter, r *http.Request, id int64) {
	if a.docker == nil {
		jsonError(w, http.StatusServiceUnavailable, "docker_unavailable", "O Docker não está disponível.")
		return
	}
	var project, slug, mode string
	if err := a.db.QueryRowContext(r.Context(), "SELECT coalesce(docker_stack_name,''),slug FROM applications WHERE id=?", id).Scan(&project, &slug); err != nil {
		jsonError(w, http.StatusNotFound, "not_found", "Aplicação não encontrada.")
		return
	}
	if strings.TrimSpace(project) == "" {
		project = slug
	}
	_ = a.db.QueryRowContext(r.Context(), "SELECT coalesce(runtime_mode,'standalone') FROM environment_settings WHERE id=1").Scan(&mode)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 7 {
		volumeName, decodeErr := url.PathUnescape(parts[5])
		if decodeErr != nil {
			jsonError(w, 400, "invalid_volume", "Volume inválido.")
			return
		}
		relative := r.URL.Query().Get("path")
		switch parts[6] {
		case "folder":
			if r.Method != http.MethodPost {
				jsonError(w, 405, "method_not_allowed", "Método não permitido.")
				return
			}
			var input struct {
				Path string `json:"path"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&input) != nil {
				jsonValidation(w, map[string]string{"path": "Informe uma pasta válida."})
				return
			}
			if err := a.docker.StorageFolder(r.Context(), project, mode, volumeName, input.Path); err != nil {
				jsonError(w, 400, "invalid_path", "Não foi possível criar a pasta.")
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		case "file":
			if r.Method == http.MethodDelete {
				if err := a.docker.StorageDelete(r.Context(), project, mode, volumeName, relative); err != nil {
					jsonError(w, 400, "invalid_path", "Não foi possível excluir o arquivo.")
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		case "upload":
			if r.Method != http.MethodPost {
				jsonError(w, 405, "method_not_allowed", "Método não permitido.")
				return
			}
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				jsonError(w, 400, "invalid_upload", "Upload inválido.")
				return
			}
			uploaded, header, err := r.FormFile("file")
			if err != nil {
				jsonError(w, 400, "invalid_upload", "Arquivo não informado.")
				return
			}
			defer uploaded.Close()
			data, err := io.ReadAll(io.LimitReader(uploaded, 32<<20+1))
			if err != nil || len(data) > 32<<20 {
				jsonError(w, 413, "file_too_large", "O arquivo excede o limite de 32 MB.")
				return
			}
			if err = a.docker.StorageUpload(r.Context(), project, mode, volumeName, relative, data, filepath.Base(header.Filename)); err != nil {
				jsonError(w, 400, "upload_failed", "Não foi possível enviar o arquivo.")
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		case "download":
			if r.Method != http.MethodPost {
				jsonError(w, 405, "method_not_allowed", "Método não permitido.")
				return
			}
			files, err := a.docker.StorageArchive(r.Context(), project, mode, volumeName, relative, 32<<20)
			if err != nil || len(files) == 0 || files[0].Type != "file" {
				jsonError(w, 404, "file_not_found", "Arquivo não encontrado.")
				return
			}
			w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(files[0].Name)+`"`)
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(files[0].Content)
			return
		case "backup":
			root := filepath.Join(a.dataDir, "backups", "storage", strconv.FormatInt(id, 10))
			if len(parts) >= 8 && parts[7] == "download" {
				if r.Method != http.MethodGet {
					jsonError(w, 405, "method_not_allowed", "Método não permitido.")
					return
				}
				name := filepath.Base(r.URL.Query().Get("name"))
				if name == "." || name == ".." || name != r.URL.Query().Get("name") {
					jsonError(w, 400, "invalid_backup", "Backup inválido.")
					return
				}
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil {
					jsonError(w, 404, "backup_not_found", "Backup não encontrado.")
					return
				}
				w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
				w.Header().Set("Content-Type", "application/gzip")
				_, _ = w.Write(data)
				return
			}
			if r.Method == http.MethodGet {
				items := make([]map[string]any, 0)
				files, _ := os.ReadDir(root)
				for _, item := range files {
					if item.IsDir() {
						continue
					}
					info, statErr := item.Info()
					if statErr == nil {
						items = append(items, map[string]any{"name": item.Name(), "size_bytes": info.Size(), "created_at": info.ModTime().UTC().Format(time.RFC3339)})
					}
				}
				writeJSON(w, map[string]any{"backups": items})
				return
			}
			if r.Method != http.MethodPost {
				jsonError(w, 405, "method_not_allowed", "Método não permitido.")
				return
			}
			files, err := a.docker.StorageArchive(r.Context(), project, mode, volumeName, ".", 256<<20)
			if err != nil {
				jsonError(w, 502, "backup_failed", "Não foi possível ler o volume.")
				return
			}
			stamp := time.Now().UTC().Format("20060102T150405Z")
			dir := root
			if err = os.MkdirAll(dir, 0750); err != nil {
				jsonError(w, 500, "backup_failed", "Não foi possível criar o backup.")
				return
			}
			target := filepath.Join(dir, stamp+"-"+filepath.Base(volumeName)+".tar.gz")
			out, err := os.Create(target)
			if err != nil {
				jsonError(w, 500, "backup_failed", "Não foi possível salvar o backup.")
				return
			}
			gz := gzip.NewWriter(out)
			tw := tar.NewWriter(gz)
			for _, file := range files {
				header := &tar.Header{Name: file.Path, Mode: 0600, Size: int64(len(file.Content)), ModTime: time.Now().UTC()}
				if file.Type == "directory" {
					header.Typeflag = tar.TypeDir
					header.Size = 0
				}
				if err = tw.WriteHeader(header); err == nil && file.Type == "file" {
					_, err = tw.Write(file.Content)
				}
				if err != nil {
					break
				}
			}
			if closeErr := tw.Close(); err == nil {
				err = closeErr
			}
			if closeErr := gz.Close(); err == nil {
				err = closeErr
			}
			if closeErr := out.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				_ = os.Remove(target)
				jsonError(w, 500, "backup_failed", "Não foi possível concluir o backup.")
				return
			}
			info, _ := os.Stat(target)
			writeJSONStatus(w, http.StatusCreated, map[string]any{"name": filepath.Base(target), "size_bytes": info.Size(), "created_at": time.Now().UTC().Format(time.RFC3339)})
			return
		case "restore":
			if r.Method != http.MethodPost {
				jsonError(w, 405, "method_not_allowed", "Método não permitido.")
				return
			}
			var input struct {
				Backup  string `json:"backup"`
				Confirm bool   `json:"confirm"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&input) != nil || !input.Confirm {
				jsonError(w, 400, "confirmation_required", "Confirme a restauração do backup.")
				return
			}
			backupName := filepath.Base(input.Backup)
			if backupName != input.Backup || backupName == "." || backupName == ".." {
				jsonError(w, 400, "invalid_backup", "Backup inválido.")
				return
			}
			root := filepath.Join(a.dataDir, "backups", "storage", strconv.FormatInt(id, 10))
			cleanBackup := filepath.Join(root, backupName)
			archive, err := os.ReadFile(cleanBackup)
			if err != nil {
				jsonError(w, 404, "backup_not_found", "Backup não encontrado.")
				return
			}
			if err = a.docker.StorageRestore(r.Context(), project, mode, volumeName, archive); err != nil {
				jsonError(w, 400, "restore_failed", "Não foi possível restaurar o backup.")
				return
			}
			writeJSON(w, map[string]any{"restored": true})
			return
		}
		if r.Method != http.MethodGet {
			jsonError(w, 405, "method_not_allowed", "Método não permitido.")
			return
		}
	}
	if len(parts) == 6 && r.Method == http.MethodPost {
		volumeName, decodeErr := url.PathUnescape(parts[5])
		if decodeErr != nil {
			jsonError(w, 400, "invalid_volume", "Volume inválido.")
			return
		}
		var input struct {
			Path    string `json:"path"`
			NewPath string `json:"new_path"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&input) != nil || input.NewPath == "" {
			jsonError(w, 400, "invalid_path", "Informe o novo caminho.")
			return
		}
		if err := a.docker.StorageRename(r.Context(), project, mode, volumeName, input.Path, input.NewPath); err != nil {
			jsonError(w, 400, "rename_failed", "Não foi possível renomear o item.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) >= 7 {
		volumeName, decodeErr := url.PathUnescape(parts[5])
		if decodeErr != nil {
			jsonError(w, 400, "invalid_volume", "Volume inválido.")
			return
		}
		if r.Method != http.MethodGet {
			jsonError(w, 405, "method_not_allowed", "Método não permitido.")
			return
		}
		relative := r.URL.Query().Get("path")
		files, archiveErr := a.docker.StorageArchive(r.Context(), project, mode, volumeName, relative, 8<<20)
		if archiveErr != nil {
			jsonError(w, 404, "storage_path_not_found", "Caminho não encontrado.")
			return
		}
		if parts[6] == "file" {
			if len(files) == 0 || files[0].Type != "file" {
				jsonError(w, 404, "file_not_found", "Arquivo não encontrado.")
				return
			}
			file := files[0]
			writeJSON(w, map[string]any{"name": file.Name, "path": file.Path, "type": file.Type, "mime": mime.TypeByExtension(filepath.Ext(file.Name)), "size_bytes": file.SizeBytes, "modified": file.Modified, "content": string(file.Content), "content_base64": base64.StdEncoding.EncodeToString(file.Content)})
			return
		}
		if parts[6] == "tree" {
			prefix := strings.Trim(relative, "."+"/")
			entries := make([]any, 0)
			seen := map[string]bool{}
			for _, file := range files {
				entryPath := strings.Trim(file.Path, "/")
				if prefix != "" && !strings.HasPrefix(entryPath, prefix+"/") {
					continue
				}
				rest := strings.TrimPrefix(entryPath, prefix)
				rest = strings.Trim(rest, "/")
				if rest == "" {
					continue
				}
				name := strings.Split(rest, "/")[0]
				childPath := name
				if prefix != "" {
					childPath = prefix + "/" + name
				}
				if seen[childPath] {
					continue
				}
				seen[childPath] = true
				kind := file.Type
				if strings.Contains(rest, "/") {
					kind = "directory"
				}
				entries = append(entries, map[string]any{"name": name, "path": childPath, "type": kind, "size_bytes": file.SizeBytes, "modified": file.Modified})
			}
			writeJSON(w, map[string]any{"path": relative, "entries": entries})
			return
		}
	}
	items, err := a.docker.ApplicationStorage(r.Context(), project, mode)
	if err != nil {
		jsonError(w, http.StatusBadGateway, "storage_unavailable", "Não foi possível descobrir os volumes da aplicação.")
		return
	}
	if len(parts) == 5 {
		json.NewEncoder(w).Encode(map[string]any{"application_id": id, "volumes": items, "count": len(items)})
		return
	}
	if len(parts) == 6 {
		volumeName, decodeErr := url.PathUnescape(parts[5])
		if decodeErr != nil {
			jsonError(w, 400, "invalid_volume", "Volume inválido.")
			return
		}
		for _, item := range items {
			if item.Name == volumeName {
				writeJSON(w, item)
				return
			}
		}
	}
	jsonError(w, 404, "not_found", "Storage não encontrado.")
}
