package storage

import (
	"context"
	"fmt"
	"path"
	"strings"
)

// Volume describes storage attached to a managed application.
type Volume struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Source     string `json:"-"`
	Mountpoint string `json:"mountpoint,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	UsedBytes  int64  `json:"used_bytes,omitempty"`
	InUse      bool   `json:"in_use"`
}

type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Modified  string `json:"modified,omitempty"`
}

type File struct {
	Entry
	Content []byte `json:"-"`
}

// StorageManager is the provider boundary for standalone and Swarm backends.
// Keeping the application and path in every operation prevents accidental use
// of a container or volume outside the managed application scope.
type StorageManager interface {
	ListVolumes(context.Context, string, string) ([]Volume, error)
	Browse(context.Context, string, string, string, string) ([]Entry, error)
	Read(context.Context, string, string, string, string) (File, error)
	Upload(context.Context, string, string, string, string, []byte, string) error
	Delete(context.Context, string, string, string, string) error
	Backup(context.Context, string, string, string) (string, error)
	Restore(context.Context, string, string, string, string) error
}

// SafePath normalizes a client supplied relative path and rejects traversal,
// absolute paths, and Docker socket targets.
func SafePath(relative string) (string, error) {
	relative = strings.TrimSpace(strings.ReplaceAll(relative, "\\", "/"))
	if relative == "" || relative == "." {
		return ".", nil
	}
	if strings.HasPrefix(relative, "/") || strings.Contains(relative, ":") {
		return "", fmt.Errorf("path must be relative")
	}
	clean := path.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("path traversal is not allowed")
	}
	if clean == "etc" || strings.HasPrefix(clean, "etc/") || clean == "root" || strings.HasPrefix(clean, "root/") || clean == "var/run/docker.sock" || strings.HasPrefix(clean, "var/run/docker.sock/") {
		return "", fmt.Errorf("protected path")
	}
	return clean, nil
}

func Join(base, child string) (string, error) {
	if base == "." {
		return SafePath(child)
	}
	cleanBase, err := SafePath(base)
	if err != nil {
		return "", err
	}
	cleanChild, err := SafePath(child)
	if err != nil {
		return "", err
	}
	return SafePath(path.Join(cleanBase, cleanChild))
}
