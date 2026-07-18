package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
type RuntimeMetric struct {
	Service          string  `json:"service"`
	CPUPercent       float64 `json:"cpu_percent"`
	MemoryBytes      uint64  `json:"memory_bytes"`
	MemoryLimitBytes uint64  `json:"memory_limit_bytes"`
	NetworkRxBytes   uint64  `json:"network_rx_bytes"`
	NetworkTxBytes   uint64  `json:"network_tx_bytes"`
	RestartCount     int     `json:"restart_count"`
	Health           string  `json:"health,omitempty"`
}
type RuntimeService struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Desired int    `json:"desired"`
	Running int    `json:"running"`
	Failed  int    `json:"failed"`
	Health  string `json:"health"`
}

type RuntimeSnapshot struct {
	Mode     string           `json:"mode"`
	Status   string           `json:"status"`
	Services []RuntimeService `json:"services"`
}
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
	if host == "" {
		host = os.Getenv("DOCKER_HOST")
	}
	if host == "" {
		if home, err := os.UserHomeDir(); err == nil {
			desktopSocket := filepath.Join(home, ".docker", "run", "docker.sock")
			if _, err := os.Stat(desktopSocket); err == nil {
				host = "unix://" + desktopSocket
			}
		}
	}
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

// ResolveIngressTarget derives an internal target from Docker labels and the
// managed ingress network. It never accepts a user-supplied URL.
func (r *Reader) ResolveIngressTarget(ctx context.Context, runtimeMode, stackName, serviceName string, port int) (string, error) {
	if r == nil || r.client == nil || stackName == "" || serviceName == "" || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid ingress target")
	}
	if runtimeMode == "swarm" {
		return fmt.Sprintf("http://%s_%s:%d", stackName, serviceName, port), nil
	}
	containers, err := r.client.ContainerList(ctx, container.ListOptions{Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+stackName), filters.Arg("label", "com.docker.compose.service="+serviceName), filters.Arg("status", "running"))})
	if err != nil || len(containers) == 0 {
		return "", fmt.Errorf("ingress target unavailable")
	}
	for _, item := range containers {
		inspected, inspectErr := r.client.ContainerInspect(ctx, item.ID)
		if inspectErr != nil || inspected.NetworkSettings == nil {
			continue
		}
		if networkSettings, ok := inspected.NetworkSettings.Networks["stackhost-ingress"]; ok && networkSettings.IPAddress != "" {
			return fmt.Sprintf("http://%s:%d", networkSettings.IPAddress, port), nil
		}
		for _, networkSettings := range inspected.NetworkSettings.Networks {
			if networkSettings.IPAddress != "" {
				return fmt.Sprintf("http://%s:%d", networkSettings.IPAddress, port), nil
			}
		}
	}
	return "", fmt.Errorf("ingress target unavailable")
}

func (r *Reader) EnsureIngressNetwork(ctx context.Context, swarmMode bool) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("docker unavailable")
	}
	if _, err := r.client.NetworkInspect(ctx, "stackhost-ingress", types.NetworkInspectOptions{}); err == nil {
		return nil
	}
	driver := "bridge"
	options := network.CreateOptions{Driver: driver, Labels: map[string]string{"com.stackhost.managed": "true"}}
	if swarmMode {
		options.Driver = "overlay"
		options.Attachable = true
	}
	if _, err := r.client.NetworkCreate(ctx, "stackhost-ingress", options); err != nil {
		return err
	}
	if containerID := os.Getenv("HOSTNAME"); containerID != "" {
		_ = r.client.NetworkConnect(ctx, "stackhost-ingress", containerID, nil)
	}
	return nil
}

// RuntimeMetrics returns a point-in-time Docker stats snapshot for one
// Compose project. Swarm metrics intentionally remain local-only: this
// reader never presents another node's data as a global measurement.
func (r *Reader) RuntimeMetrics(ctx context.Context, runtimeMode, stackName, serviceName string) ([]RuntimeMetric, error) {
	if r == nil || r.client == nil || runtimeMode != "standalone" || stackName == "" {
		return nil, fmt.Errorf("local runtime metrics unavailable")
	}
	filterArgs := filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+stackName), filters.Arg("status", "running"))
	if serviceName != "" {
		filterArgs.Add("label", "com.docker.compose.service="+serviceName)
	}
	containers, err := r.client.ContainerList(ctx, container.ListOptions{Filters: filterArgs})
	if err != nil {
		return nil, err
	}
	metrics := make([]RuntimeMetric, 0, len(containers))
	for _, item := range containers {
		stats, err := r.client.ContainerStatsOneShot(ctx, item.ID)
		if err != nil {
			continue
		}
		var snapshot container.StatsResponse
		decodeErr := json.NewDecoder(stats.Body).Decode(&snapshot)
		_ = stats.Body.Close()
		if decodeErr != nil && decodeErr != io.EOF {
			continue
		}
		cpuDelta := float64(snapshot.CPUStats.CPUUsage.TotalUsage - snapshot.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(snapshot.CPUStats.SystemUsage - snapshot.PreCPUStats.SystemUsage)
		cpuPercent := 0.0
		if cpuDelta > 0 && systemDelta > 0 && snapshot.CPUStats.OnlineCPUs > 0 {
			cpuPercent = (cpuDelta / systemDelta) * float64(snapshot.CPUStats.OnlineCPUs) * 100
		}
		var rx, tx uint64
		for _, network := range snapshot.Networks {
			rx += network.RxBytes
			tx += network.TxBytes
		}
		metrics = append(metrics, RuntimeMetric{Service: item.Labels["com.docker.compose.service"], CPUPercent: cpuPercent, MemoryBytes: snapshot.MemoryStats.Usage, MemoryLimitBytes: snapshot.MemoryStats.Limit, NetworkRxBytes: rx, NetworkTxBytes: tx})
	}
	return metrics, nil
}

// RuntimeSnapshot returns service state without invoking the Docker CLI.
func (r *Reader) RuntimeSnapshot(ctx context.Context, runtimeMode, stackName string) (RuntimeSnapshot, error) {
	if r == nil || r.client == nil || stackName == "" {
		return RuntimeSnapshot{}, fmt.Errorf("runtime unavailable")
	}
	out := RuntimeSnapshot{Mode: runtimeMode, Status: "not_deployed", Services: []RuntimeService{}}
	if runtimeMode == "swarm" {
		items, err := r.client.ServiceList(ctx, types.ServiceListOptions{Filters: filters.NewArgs(filters.Arg("label", "com.docker.stack.namespace="+stackName)), Status: true})
		if err != nil {
			return out, err
		}
		for _, item := range items {
			desired, running := 0, 0
			if item.ServiceStatus != nil {
				desired, running = int(item.ServiceStatus.DesiredTasks), int(item.ServiceStatus.RunningTasks)
			}
			image := ""
			if item.Spec.TaskTemplate.ContainerSpec != nil {
				image = item.Spec.TaskTemplate.ContainerSpec.Image
			}
			out.Services = append(out.Services, RuntimeService{Name: item.Spec.Name, Image: image, Desired: desired, Running: running, Health: "unknown"})
		}
	} else {
		items, err := r.client.ContainerList(ctx, container.ListOptions{Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+stackName))})
		if err != nil {
			return out, err
		}
		for _, item := range items {
			inspected, err := r.client.ContainerInspect(ctx, item.ID)
			if err != nil || inspected.State == nil {
				continue
			}
			health := "no_healthcheck"
			if inspected.State.Health != nil && inspected.State.Health.Status != "" {
				health = strings.ToLower(inspected.State.Health.Status)
			}
			ready := inspected.State.Running && health != "unhealthy"
			name := item.Labels["com.docker.compose.service"]
			out.Services = append(out.Services, RuntimeService{Name: name, Image: item.Image, Desired: 1, Running: boolInt(ready), Failed: boolInt(!ready), Health: health})
		}
	}
	if len(out.Services) == 0 {
		return out, nil
	}
	running, failed := 0, 0
	for _, service := range out.Services {
		running += service.Running
		failed += service.Failed
	}
	switch {
	case running == len(out.Services):
		out.Status = "running"
	case running > 0:
		out.Status = "degraded"
	case failed == len(out.Services):
		out.Status = "stopped"
	default:
		out.Status = "unknown"
	}
	return out, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
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
