package docker

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/api/types"
	client "github.com/docker/docker/client"
)

type Reader struct{ client *client.Client }
type Snapshot struct {
	Available     bool   `json:"available"`
	Message       string `json:"message,omitempty"`
	EngineVersion string `json:"engine_version,omitempty"`
	Containers    int    `json:"containers"`
	Running       int    `json:"running"`
	Images        int    `json:"images"`
	Swarm         Swarm  `json:"swarm"`
}
type Swarm struct {
	Active   bool   `json:"active"`
	Message  string `json:"message,omitempty"`
	NodeID   string `json:"node_id,omitempty"`
	Nodes    int    `json:"nodes"`
	Managers int    `json:"managers"`
	Workers  int    `json:"workers"`
	Services int    `json:"services"`
}

func NewReader() (*Reader, error) {
	host := os.Getenv("STACKHOST_DOCKER_HOST")
	opts := []client.Opt{client.FromEnv}
	if host != "" {
		opts = []client.Opt{client.WithHost(host)}
	}
	c, err := client.NewClientWithOpts(append(opts, client.WithAPIVersionNegotiation())...)
	if err != nil {
		return nil, err
	}
	return &Reader{client: c}, nil
}
func (r *Reader) Snapshot(ctx context.Context) Snapshot {
	out := Snapshot{}
	if _, err := r.client.Ping(ctx); err != nil {
		out.Message = fmt.Sprintf("Docker indisponível: %v", err)
		return out
	}
	out.Available = true
	v, err := r.client.ServerVersion(ctx)
	if err == nil {
		out.EngineVersion = v.Version
	}
	info, err := r.client.Info(ctx)
	if err == nil {
		out.Containers = info.Containers
		out.Running = info.ContainersRunning
		out.Images = info.Images
	}
	swarm, err := r.client.SwarmInspect(ctx)
	if err != nil {
		out.Swarm.Message = "O host não está em um Swarm."
		return out
	}
	out.Swarm.Active = true
	out.Swarm.NodeID = swarm.ID
	nodes, err := r.client.NodeList(ctx, types.NodeListOptions{})
	if err == nil {
		out.Swarm.Nodes = len(nodes)
		for _, n := range nodes {
			if n.Spec.Role == "manager" {
				out.Swarm.Managers++
			} else {
				out.Swarm.Workers++
			}
		}
	}
	services, err := r.client.ServiceList(ctx, types.ServiceListOptions{})
	if err == nil {
		out.Swarm.Services = len(services)
	}
	return out
}
