package docker

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	client "github.com/docker/docker/client"
)

type Reader struct{ client *client.Client }
type Snapshot struct {
	Available       bool   `json:"available"`
	Message         string `json:"message,omitempty"`
	EngineVersion   string `json:"engine_version,omitempty"`
	Containers      int    `json:"containers"`
	Running         int    `json:"running"`
	Images          int    `json:"images"`
	OperatingSystem string `json:"operating_system,omitempty"`
	CPUs            int    `json:"cpus,omitempty"`
	MemoryBytes     int64  `json:"memory_bytes,omitempty"`
	Swarm           Swarm  `json:"swarm"`
}
type Swarm struct {
	Active       bool   `json:"active"`
	Message      string `json:"message,omitempty"`
	NodeID       string `json:"node_id,omitempty"`
	Nodes        int    `json:"nodes"`
	Managers     int    `json:"managers"`
	Workers      int    `json:"workers"`
	Services     int    `json:"services"`
	TasksRunning int    `json:"tasks_running"`
	TasksFailed  int    `json:"tasks_failed"`
	TasksPending int    `json:"tasks_pending"`
}
type Node struct {
	ID           string `json:"id"`
	Hostname     string `json:"hostname"`
	Role         string `json:"role"`
	Availability string `json:"availability"`
	State        string `json:"state"`
}
type Service struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Image    string `json:"image"`
	Replicas uint64 `json:"replicas"`
	Running  uint64 `json:"running"`
	Desired  uint64 `json:"desired"`
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
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
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
		out.OperatingSystem = info.OperatingSystem
		out.CPUs = info.NCPU
		out.MemoryBytes = info.MemTotal
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
	if tasks, err := r.client.TaskList(ctx, types.TaskListOptions{}); err == nil {
		for _, task := range tasks {
			switch string(task.Status.State) {
			case "running":
				out.Swarm.TasksRunning++
			case "failed", "rejected", "orphaned", "shutdown":
				out.Swarm.TasksFailed++
			case "new", "pending", "assigned", "accepted", "preparing", "ready", "starting":
				out.Swarm.TasksPending++
			}
		}
	}
	return out
}
func (r *Reader) Nodes(ctx context.Context) ([]Node, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	items, err := r.client.NodeList(ctx, types.NodeListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(items))
	for _, n := range items {
		out = append(out, Node{ID: n.ID, Hostname: n.Description.Hostname, Role: string(n.Spec.Role), Availability: string(n.Spec.Availability), State: string(n.Status.State)})
	}
	return out, nil
}
func (r *Reader) Services(ctx context.Context) ([]Service, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	items, err := r.client.ServiceList(ctx, types.ServiceListOptions{Status: true})
	if err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(items))
	for _, s := range items {
		desired, running := uint64(0), uint64(0)
		if s.ServiceStatus != nil {
			desired = uint64(s.ServiceStatus.DesiredTasks)
			running = uint64(s.ServiceStatus.RunningTasks)
		}
		image := ""
		if s.Spec.TaskTemplate.ContainerSpec != nil {
			image = s.Spec.TaskTemplate.ContainerSpec.Image
		}
		out = append(out, Service{ID: s.ID, Name: s.Spec.Name, Image: image, Desired: desired, Running: running, Replicas: desired})
	}
	return out, nil
}
