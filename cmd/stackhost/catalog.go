package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type catalogTemplate struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Category     string   `json:"category"`
	Source       string   `json:"source"`
	Requirements []string `json:"requirements"`
}

func (a *app) catalog(w http.ResponseWriter, r *http.Request) {
	root := getenv("STACKHOST_CATALOG_DIR", "./catalog")
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method_not_allowed", "Método não permitido.")
		return
	}
	if len(parts) >= 5 {
		if parts[3] != "official" && parts[3] != "community" && parts[3] != "private" {
			jsonError(w, 404, "not_found", "Catálogo não encontrado.")
			return
		}
		path := filepath.Join(root, parts[3], filepath.Clean(parts[4]), "template.json")
		if !withinDirectory(path, filepath.Join(root, parts[3])) {
			jsonError(w, 404, "not_found", "Template não encontrado.")
			return
		}
		body, err := os.ReadFile(path)
		if err != nil {
			jsonError(w, 404, "not_found", "Template não encontrado.")
			return
		}
		var item catalogTemplate
		if json.Unmarshal(body, &item) != nil {
			jsonError(w, 500, "catalog_invalid", "O template do catálogo está inválido.")
			return
		}
		json.NewEncoder(w).Encode(item)
		return
	}
	items := []catalogTemplate{}
	for _, source := range []string{"official", "community", "private"} {
		entries, _ := os.ReadDir(filepath.Join(root, source))
		for _, entry := range entries {
			if !entry.IsDir() || strings.Contains(entry.Name(), "..") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(root, source, entry.Name(), "template.json"))
			if err != nil {
				continue
			}
			var item catalogTemplate
			if json.Unmarshal(body, &item) == nil {
				items = append(items, item)
			}
		}
	}
	json.NewEncoder(w).Encode(items)
}

func withinDirectory(path, root string) bool {
	path, _ = filepath.Abs(path)
	root, _ = filepath.Abs(root)
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
