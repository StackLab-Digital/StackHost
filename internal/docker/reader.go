package docker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/volume"
	client "github.com/docker/docker/client"
)

type Reader struct{ client *client.Client }
type Snapshot struct {
	Available       bool        `json:"available"`
	Message         string      `json:"message,omitempty"`
	EngineVersion   string      `json:"engine_version,omitempty"`
	Containers      int         `json:"containers"`
	Running         int         `json:"running"`
	Images          int         `json:"images"`
	Networks        int         `json:"networks"`
	Volumes         int         `json:"volumes"`
	Stacks          int         `json:"stacks"`
	OperatingSystem string      `json:"operating_system,omitempty"`
	Architecture    string      `json:"architecture,omitempty"`
	KernelVersion   string      `json:"kernel_version,omitempty"`
	DaemonName      string      `json:"daemon_name,omitempty"`
	Environment     Environment `json:"environment"`
	CPUs            int         `json:"cpus,omitempty"`
	MemoryBytes     int64       `json:"memory_bytes,omitempty"`
	Swarm           Swarm       `json:"swarm"`
}
type Swarm struct {
	Active           bool   `json:"active"`
	ControlAvailable bool   `json:"control_available"`
	NodeRole         string `json:"node_role,omitempty"`
	CanInitialize    bool   `json:"can_initialize"`
	Message          string `json:"message,omitempty"`
	NodeID           string `json:"node_id,omitempty"`
	Nodes            int    `json:"nodes"`
	Managers         int    `json:"managers"`
	Workers          int    `json:"workers"`
	Services         int    `json:"services"`
	TasksRunning     int    `json:"tasks_running"`
	TasksFailed      int    `json:"tasks_failed"`
	TasksPending     int    `json:"tasks_pending"`
}
type Environment struct {
	Kind            string `json:"kind"`
	IsDockerDesktop bool   `json:"is_docker_desktop"`
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
type Container struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	State   string   `json:"state"`
	Status  string   `json:"status"`
	Created int64    `json:"created"`
	Ports   []string `json:"ports,omitempty"`
}
type Image struct {
	ID      string   `json:"id"`
	Tags    []string `json:"tags,omitempty"`
	Size    int64    `json:"size"`
	Created int64    `json:"created"`
}
type Volume struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}
type Network struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Scope      string `json:"scope"`
	Internal   bool   `json:"internal"`
	Attachable bool   `json:"attachable"`
}

func activeSwarm(info swarm.Info) bool {
	return info.LocalNodeState == swarm.LocalNodeStateActive
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
		out.Architecture = info.Architecture
		out.KernelVersion = info.KernelVersion
		out.DaemonName = info.Name
		out.Environment = Environment{Kind: "self-hosted", IsDockerDesktop: strings.Contains(strings.ToLower(info.OperatingSystem), "docker desktop") || strings.Contains(strings.ToLower(info.Name), "docker desktop")}
		if out.Environment.IsDockerDesktop {
			out.Environment.Kind = "docker-desktop"
		}
		out.CPUs = info.NCPU
		out.MemoryBytes = info.MemTotal
	}
	if networks, err := r.client.NetworkList(ctx, types.NetworkListOptions{}); err == nil {
		out.Networks = len(networks)
	}
	if volumes, err := r.client.VolumeList(ctx, volume.ListOptions{}); err == nil {
		out.Volumes = len(volumes.Volumes)
	}
	if !activeSwarm(info.Swarm) {
		out.Swarm.Message = "O host não está em um Swarm."
		out.Swarm.CanInitialize = true
		return out
	}
	out.Swarm.Active = true
	out.Swarm.ControlAvailable = info.Swarm.ControlAvailable
	if info.Swarm.ControlAvailable {
		out.Swarm.NodeRole = "manager"
	} else {
		out.Swarm.NodeRole = "worker"
	}
	out.Swarm.NodeID = info.Swarm.NodeID
	if !info.Swarm.ControlAvailable {
		out.Swarm.Message = "Swarm ativo; detalhes de nodes e serviços disponíveis apenas em manager."
		return out
	}
	swarm, err := r.client.SwarmInspect(ctx)
	if err != nil {
		out.Swarm.Message = "Swarm ativo; detalhes do cluster indisponíveis."
		return out
	}
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
		stacks := make(map[string]struct{})
		for _, service := range services {
			if name := service.Spec.Labels["com.docker.stack.namespace"]; name != "" {
				stacks[name] = struct{}{}
			}
		}
		out.Stacks = len(stacks)
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

func (r *Reader) InitSwarm(ctx context.Context, advertiseAddress string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return r.client.SwarmInit(ctx, swarm.InitRequest{ListenAddr: "0.0.0.0:2377", AdvertiseAddr: advertiseAddress})
}

func (r *Reader) Containers(ctx context.Context, limit int, state string) ([]Container, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	containerFilters := filters.NewArgs()
	if state != "" {
		containerFilters.Add("status", state)
	}
	items, err := r.client.ContainerList(ctx, container.ListOptions{All: true, Limit: limit, Filters: containerFilters})
	if err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(items))
	for _, item := range items {
		name := ""
		if len(item.Names) > 0 {
			name = strings.TrimPrefix(item.Names[0], "/")
		}
		out = append(out, Container{ID: item.ID, Name: name, Image: item.Image, State: item.State, Status: item.Status, Created: item.Created})
	}
	return out, nil
}

func (r *Reader) Images(ctx context.Context, limit int) ([]Image, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	items, err := r.client.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]Image, 0, len(items))
	for _, item := range items {
		out = append(out, Image{ID: item.ID, Tags: item.RepoTags, Size: item.Size, Created: item.Created})
	}
	return out, nil
}

func (r *Reader) Volumes(ctx context.Context, limit int) ([]Volume, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	items, err := r.client.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, err
	}
	volumes := items.Volumes
	if limit > 0 && len(volumes) > limit {
		volumes = volumes[:limit]
	}
	out := make([]Volume, 0, len(volumes))
	for _, item := range volumes {
		out = append(out, Volume{Name: item.Name, Driver: item.Driver, Scope: item.Scope})
	}
	return out, nil
}

func (r *Reader) Networks(ctx context.Context, limit int) ([]Network, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	items, err := r.client.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]Network, 0, len(items))
	for _, item := range items {
		out = append(out, Network{ID: item.ID, Name: item.Name, Driver: item.Driver, Scope: item.Scope, Internal: item.Internal, Attachable: item.Attachable})
	}
	return out, nil
}
