package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/docker/api/types"
	client "github.com/docker/docker/client"
)

func TestStorageMountForUsesOnlyApplicationLabelWithFakeDocker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1.41/containers/json":
			_ = json.NewEncoder(w).Encode([]types.Container{{ID: "managed", Labels: map[string]string{"com.docker.compose.project": "demo"}}})
		case r.URL.Path == "/v1.41/containers/managed/json":
			_ = json.NewEncoder(w).Encode(types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: "managed"}, Mounts: []types.MountPoint{{Type: "volume", Name: "demo-data", Source: "/var/lib/docker/volumes/demo-data/_data", Destination: "/data"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	dockerClient, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.41"))
	if err != nil {
		t.Fatal(err)
	}
	reader := &Reader{client: dockerClient}
	mount, err := reader.storageMountFor(context.Background(), "demo", "standalone", "demo-data")
	if err != nil {
		t.Fatal(err)
	}
	if mount.containerID != "managed" || mount.destination != "/data" {
		t.Fatalf("unexpected mount: %#v", mount)
	}
}
