package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/volume"
)

func TestStorageIntegrationWithDocker(t *testing.T) {
	if os.Getenv("STACKHOST_STORAGE_INTEGRATION") != "1" {
		t.Skip("set STACKHOST_STORAGE_INTEGRATION=1 to run against a Docker daemon")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	reader, err := NewReader()
	if err != nil {
		t.Fatal(err)
	}
	name := "stackhost-storage-test-" + strings.ToLower(strings.ReplaceAll(time.Now().Format("150405.000"), ".", ""))
	createdVolume, err := reader.client.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.client.VolumeRemove(ctx, createdVolume.Name, true)
	project := "stackhost-storage-integration"
	created, err := reader.client.ContainerCreate(ctx, &container.Config{Image: "alpine:3.20", Cmd: []string{"sleep", "300"}, Labels: map[string]string{"com.docker.compose.project": project}}, &container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: createdVolume.Name, Target: "/data"}}}, nil, nil, "")
	if err != nil {
		t.Skipf("alpine:3.20 is unavailable: %v", err)
	}
	defer reader.client.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
	if err = reader.client.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	items, err := reader.ApplicationStorage(ctx, project, "standalone")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != name || items[0].Type != "named" {
		t.Fatalf("unexpected mounts: %#v", items)
	}
	if err = reader.StorageFolder(ctx, project, "standalone", name, "uploads"); err != nil {
		t.Fatal(err)
	}
	if err = reader.StorageUpload(ctx, project, "standalone", name, "uploads", []byte("hello"), "greeting.txt"); err != nil {
		t.Fatal(err)
	}
	files, err := reader.StorageArchive(ctx, project, "standalone", name, "uploads/greeting.txt", 64<<10)
	if err != nil || len(files) != 1 || string(files[0].Content) != "hello" {
		t.Fatalf("archive read = %#v, %v", files, err)
	}
	if err = reader.StorageRename(ctx, project, "standalone", name, "uploads/greeting.txt", "uploads/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if err = reader.StorageDelete(ctx, project, "standalone", name, "uploads/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	archive := testStorageArchive(t, "restored.txt", "restored")
	if err = reader.StorageRestore(ctx, project, "standalone", name, archive); err != nil {
		t.Fatal(err)
	}
	files, err = reader.StorageArchive(ctx, project, "standalone", name, "restored.txt", 64<<10)
	if err != nil || len(files) != 1 || string(files[0].Content) != "restored" {
		t.Fatalf("restore read = %#v, %v", files, err)
	}
	bindSource, err := os.MkdirTemp("", "stackhost-storage-bind-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(bindSource)
	bindContainer, err := reader.client.ContainerCreate(ctx, &container.Config{Image: "alpine:3.20", Cmd: []string{"sleep", "300"}, Labels: map[string]string{"com.docker.compose.project": project}}, &container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeBind, Source: bindSource, Target: "/bind"}}}, nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.client.ContainerRemove(context.Background(), bindContainer.ID, container.RemoveOptions{Force: true})
	if err = reader.client.ContainerStart(ctx, bindContainer.ID, container.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	items, err = reader.ApplicationStorage(ctx, project, "standalone")
	if err != nil {
		t.Fatal(err)
	}
	bindFound := false
	for _, item := range items {
		if item.Type == "bind" && item.Mountpoint == "/bind" {
			bindFound = true
			break
		}
	}
	if !bindFound {
		t.Fatalf("expected bind mount in %#v", items)
	}
}

func TestStorageIntegrationRepresentativeImages(t *testing.T) {
	if os.Getenv("STACKHOST_STORAGE_INTEGRATION") != "1" {
		t.Skip("set STACKHOST_STORAGE_INTEGRATION=1 to run against a Docker daemon")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	reader, err := NewReader()
	if err != nil {
		t.Fatal(err)
	}
	images := []string{"postgres:16-alpine", "redis:7-alpine", "wordpress:6-php8.3-apache", "nginx:1.27-alpine"}
	for _, image := range images {
		name := "stackhost-storage-" + strings.ReplaceAll(strings.ReplaceAll(strings.Split(image, ":")[0], "/", "-"), "_", "-")
		project := name + "-project"
		createdVolume, volumeErr := reader.client.VolumeCreate(ctx, volume.CreateOptions{Name: name})
		if volumeErr != nil {
			t.Fatal(volumeErr)
		}
		created, createErr := reader.client.ContainerCreate(ctx, &container.Config{Image: image, Entrypoint: []string{"sleep"}, Cmd: []string{"300"}, Labels: map[string]string{"com.docker.compose.project": project}}, &container.HostConfig{Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: createdVolume.Name, Target: "/data"}}}, nil, nil, "")
		if createErr != nil {
			_ = reader.client.VolumeRemove(ctx, createdVolume.Name, true)
			t.Fatal(createErr)
		}
		if startErr := reader.client.ContainerStart(ctx, created.ID, container.StartOptions{}); startErr != nil {
			_ = reader.client.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
			_ = reader.client.VolumeRemove(ctx, createdVolume.Name, true)
			t.Fatalf("%s: %v", image, startErr)
		}
		items, listErr := reader.ApplicationStorage(ctx, project, "standalone")
		_ = reader.client.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
		_ = reader.client.VolumeRemove(ctx, createdVolume.Name, true)
		if listErr != nil {
			t.Fatal(listErr)
		}
		found := false
		for _, item := range items {
			if item.Name == name && item.Type == "named" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s: expected managed named volume in %#v", image, items)
		}
	}
}

func TestStorageIntegrationSwarm(t *testing.T) {
	if os.Getenv("STACKHOST_STORAGE_INTEGRATION") != "1" {
		t.Skip("set STACKHOST_STORAGE_INTEGRATION=1 to run against a Docker daemon")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader, err := NewReader()
	if err != nil {
		t.Fatal(err)
	}
	info, err := reader.client.Info(ctx)
	if err != nil || info.Swarm.LocalNodeState != swarm.LocalNodeStateActive {
		t.Skip("create a local Swarm service before running this opt-in discovery check")
	}
	items, err := reader.ApplicationStorage(ctx, "stackhost-storage-swarm-cli", "swarm")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.Name == "stackhost-storage-swarm-volume-cli" && item.Type == "named" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Swarm volume in %#v", items)
	}
}

func testStorageArchive(t *testing.T, name, content string) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(tw, content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
