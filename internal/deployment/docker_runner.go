package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	stackNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	environmentKey   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

var allowedHostEnvironment = []string{
	"DOCKER_CERT_PATH",
	"DOCKER_CONFIG",
	"DOCKER_CONTEXT",
	"DOCKER_HOST",
	"DOCKER_TLS_VERIFY",
	"HOME",
	"PATH",
	"TMPDIR",
	"XDG_RUNTIME_DIR",
}

type CommandExecutor interface {
	Run(context.Context, string, []string, []string) ([]byte, error)
}

type commandExecutor struct {
	limit int
}

func (e commandExecutor) Run(ctx context.Context, name string, args, environment []string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = environment
	output := &limitedBuffer{limit: e.limit}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	return output.Bytes(), err
}

type limitedBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - len(b.data)
	if remaining > len(value) {
		remaining = len(value)
	}
	if remaining > 0 {
		b.data = append(b.data, value[:remaining]...)
	}
	return len(value), nil
}

func (b *limitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

type DockerRunnerOption func(*DockerRunner)

type DockerRunner struct {
	executor        CommandExecutor
	temporaryDir    string
	hostEnvironment map[string]string
	pollInterval    time.Duration
}

func NewDockerRunner(options ...DockerRunnerOption) *DockerRunner {
	runner := &DockerRunner{
		executor:        commandExecutor{limit: DefaultOutputLimit},
		hostEnvironment: make(map[string]string),
		pollInterval:    500 * time.Millisecond,
	}
	for _, key := range allowedHostEnvironment {
		if value, exists := os.LookupEnv(key); exists {
			runner.hostEnvironment[key] = value
		}
	}
	if runner.hostEnvironment["DOCKER_HOST"] == "" {
		if value := os.Getenv("STACKHOST_DOCKER_HOST"); value != "" {
			runner.hostEnvironment["DOCKER_HOST"] = value
		}
	}
	for _, option := range options {
		option(runner)
	}
	return runner
}

func WithCommandExecutor(executor CommandExecutor) DockerRunnerOption {
	return func(runner *DockerRunner) {
		if executor != nil {
			runner.executor = executor
		}
	}
}

func WithTemporaryDir(directory string) DockerRunnerOption {
	return func(runner *DockerRunner) { runner.temporaryDir = directory }
}

func WithHostEnvironment(environment map[string]string) DockerRunnerOption {
	return func(runner *DockerRunner) {
		runner.hostEnvironment = make(map[string]string)
		for _, key := range allowedHostEnvironment {
			if value, exists := environment[key]; exists {
				runner.hostEnvironment[key] = value
			}
		}
	}
}

func WithPollInterval(interval time.Duration) DockerRunnerOption {
	return func(runner *DockerRunner) {
		if interval > 0 {
			runner.pollInterval = interval
		}
	}
}

func (r *DockerRunner) Deploy(ctx context.Context, request DeployRequest) DeployResult {
	if err := validateRuntime(request.RuntimeMode, request.StackName, request.Compose, request.Environment); err != nil {
		return DeployResult{ErrorCode: "invalid_deployment", ErrorMessage: "A configuração da publicação é inválida.", Err: err}
	}
	output, err := r.withComposeFile(ctx, request.Compose, request.Environment, func(path string, environment []string) ([]byte, error) {
		arguments := []string{"compose", "--project-name", request.StackName, "--file", path, "up", "--detach", "--remove-orphans"}
		if request.RuntimeMode == RuntimeSwarm {
			arguments = []string{"stack", "deploy", "--compose-file", path, "--prune", request.StackName}
		}
		return r.executor.Run(ctx, "docker", arguments, environment)
	})
	if err != nil {
		return DeployResult{Output: string(output), ErrorCode: "docker_command_failed", ErrorMessage: "O Docker não conseguiu publicar a aplicação.", Err: err}
	}
	return DeployResult{Output: string(output)}
}

func (r *DockerRunner) Start(ctx context.Context, request RuntimeRequest) error {
	if request.RuntimeMode == RuntimeSwarm {
		result := r.Deploy(ctx, deployFromRuntime(request))
		return result.Err
	}
	return r.composeAction(ctx, request, "start")
}

func (r *DockerRunner) Stop(ctx context.Context, request RuntimeRequest) error {
	if request.RuntimeMode == RuntimeSwarm {
		return r.swarmScale(ctx, request.StackName, 0)
	}
	return r.composeAction(ctx, request, "stop")
}

func (r *DockerRunner) Restart(ctx context.Context, request RuntimeRequest) error {
	if request.RuntimeMode == RuntimeSwarm {
		services, err := r.swarmServices(ctx, request.StackName)
		if err != nil {
			return err
		}
		for _, service := range services {
			if _, err := r.executor.Run(ctx, "docker", []string{"service", "update", "--force", service}, r.environment(nil)); err != nil {
				return err
			}
		}
		return nil
	}
	return r.composeAction(ctx, request, "restart")
}

func (r *DockerRunner) swarmServices(ctx context.Context, stackName string) ([]string, error) {
	output, err := r.executor.Run(ctx, "docker", []string{"stack", "services", "--format", "{{.Name}}", stackName}, r.environment(nil))
	if err != nil {
		return nil, err
	}
	services := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) != "" {
			services = append(services, strings.TrimSpace(line))
		}
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("no swarm services")
	}
	return services, nil
}

func (r *DockerRunner) swarmScale(ctx context.Context, stackName string, replicas int) error {
	services, err := r.swarmServices(ctx, stackName)
	if err != nil {
		return err
	}
	for _, service := range services {
		if _, err := r.executor.Run(ctx, "docker", []string{"service", "scale", fmt.Sprintf("%s=%d", service, replicas)}, r.environment(nil)); err != nil {
			return err
		}
	}
	return nil
}

func (r *DockerRunner) Remove(ctx context.Context, request RuntimeRequest) error {
	if !validRuntimeMode(request.RuntimeMode) || !ValidStackName(request.StackName) {
		return ErrInvalidRequest
	}
	if request.RuntimeMode == RuntimeSwarm {
		_, err := r.executor.Run(ctx, "docker", []string{"stack", "rm", request.StackName}, r.environment(nil))
		return err
	}
	return r.composeAction(ctx, request, "down")
}

func (r *DockerRunner) WaitReady(ctx context.Context, request RuntimeRequest) error {
	if err := validateRuntime(request.RuntimeMode, request.StackName, request.Compose, request.Environment); err != nil {
		return err
	}
	expected, err := composeServices(request.Compose)
	if err != nil || len(expected) == 0 {
		return fmt.Errorf("compose services: %w", ErrInvalidRequest)
	}
	if request.RuntimeMode == RuntimeSwarm {
		return r.pollReady(ctx, func() bool { return r.swarmReady(ctx, request.StackName, expected) })
	}
	return r.withComposeReadyFile(ctx, request, expected)
}

func (r *DockerRunner) Logs(ctx context.Context, request RuntimeRequest, service string, tail int) (string, error) {
	if tail <= 0 || tail > 1000 {
		tail = 100
	}
	if err := validateRuntime(request.RuntimeMode, request.StackName, request.Compose, request.Environment); err != nil {
		return "", err
	}
	if request.RuntimeMode == RuntimeSwarm {
		output, err := r.executor.Run(ctx, "docker", []string{"service", "logs", "--raw", "--tail", strconv.Itoa(tail), request.StackName + "_" + service}, r.environment(nil))
		return SanitizeOutput(string(output), nil, DefaultOutputLimit), err
	}
	output, err := r.withComposeFile(ctx, request.Compose, request.Environment, func(path string, environment []string) ([]byte, error) {
		return r.executor.Run(ctx, "docker", []string{"compose", "--project-name", request.StackName, "--file", path, "logs", "--no-color", "--tail", strconv.Itoa(tail), service}, environment)
	})
	return SanitizeOutput(string(output), nil, DefaultOutputLimit), err
}

func (r *DockerRunner) withComposeReadyFile(ctx context.Context, request RuntimeRequest, expected map[string]struct{}) error {
	_, err := r.withComposeFile(ctx, request.Compose, request.Environment, func(path string, environment []string) ([]byte, error) {
		err := r.pollReady(ctx, func() bool {
			output, runErr := r.executor.Run(ctx, "docker", []string{"compose", "--project-name", request.StackName, "--file", path, "ps", "--format", "json"}, environment)
			return runErr == nil && standaloneReady(output, expected)
		})
		return nil, err
	})
	return err
}

func (r *DockerRunner) swarmReady(ctx context.Context, stackName string, expected map[string]struct{}) bool {
	services, err := r.executor.Run(ctx, "docker", []string{"stack", "services", "--format", "{{json .}}", stackName}, r.environment(nil))
	if err != nil || !swarmServicesReady(services, stackName, expected) {
		return false
	}
	tasks, err := r.executor.Run(ctx, "docker", []string{"stack", "ps", "--filter", "desired-state=running", "--format", "{{json .}}", stackName}, r.environment(nil))
	return err == nil && swarmTasksReady(tasks)
}

func (r *DockerRunner) pollReady(ctx context.Context, ready func() bool) error {
	if ready() {
		return nil
	}
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if ready() {
				return nil
			}
		}
	}
}

func composeServices(compose string) (map[string]struct{}, error) {
	var document struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		return nil, err
	}
	services := make(map[string]struct{}, len(document.Services))
	for service := range document.Services {
		services[service] = struct{}{}
	}
	return services, nil
}

func ServiceNames(compose string) (map[string]struct{}, error) { return composeServices(compose) }

func standaloneReady(output []byte, expected map[string]struct{}) bool {
	type container struct {
		Service string `json:"Service"`
		State   string `json:"State"`
		Health  string `json:"Health"`
	}
	var containers []container
	if err := jsonRows(output, &containers); err != nil {
		return false
	}
	ready := make(map[string]bool, len(containers))
	for _, container := range containers {
		health := strings.ToLower(container.Health)
		ready[container.Service] = strings.EqualFold(container.State, "running") && (health == "" || health == "healthy")
	}
	for service := range expected {
		if !ready[service] {
			return false
		}
	}
	return true
}

func swarmServicesReady(output []byte, stackName string, expected map[string]struct{}) bool {
	type service struct {
		Name     string `json:"Name"`
		Replicas string `json:"Replicas"`
	}
	var services []service
	if err := jsonRows(output, &services); err != nil {
		return false
	}
	ready := make(map[string]bool, len(services))
	for _, service := range services {
		parts := strings.SplitN(service.Replicas, "/", 2)
		if len(parts) != 2 {
			continue
		}
		running, runningErr := strconv.Atoi(strings.TrimSpace(parts[0]))
		desired, desiredErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		name := strings.TrimPrefix(service.Name, stackName+"_")
		ready[name] = runningErr == nil && desiredErr == nil && desired > 0 && running == desired
	}
	for service := range expected {
		if !ready[service] {
			return false
		}
	}
	return true
}

func swarmTasksReady(output []byte) bool {
	type task struct {
		CurrentState string `json:"CurrentState"`
		Error        string `json:"Error"`
	}
	var tasks []task
	if err := jsonRows(output, &tasks); err != nil || len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		if task.Error != "" || !strings.HasPrefix(strings.ToLower(task.CurrentState), "running") {
			return false
		}
	}
	return true
}

func jsonRows[T any](output []byte, destination *[]T) error {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return fmt.Errorf("empty docker output")
	}
	if strings.HasPrefix(trimmed, "[") {
		return json.Unmarshal([]byte(trimmed), destination)
	}
	for _, line := range strings.Split(trimmed, "\n") {
		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return err
		}
		*destination = append(*destination, item)
	}
	return nil
}

func (r *DockerRunner) composeAction(ctx context.Context, request RuntimeRequest, action string) error {
	if err := validateRuntime(request.RuntimeMode, request.StackName, request.Compose, request.Environment); err != nil || request.RuntimeMode != RuntimeStandalone {
		return ErrInvalidRequest
	}
	_, err := r.withComposeFile(ctx, request.Compose, request.Environment, func(path string, environment []string) ([]byte, error) {
		return r.executor.Run(ctx, "docker", []string{"compose", "--project-name", request.StackName, "--file", path, action}, environment)
	})
	return err
}

func (r *DockerRunner) withComposeFile(ctx context.Context, compose string, variables map[string]string, run func(string, []string) ([]byte, error)) ([]byte, error) {
	temporary, err := os.CreateTemp(r.temporaryDir, "stackhost-compose-*.yml")
	if err != nil {
		return nil, fmt.Errorf("create temporary compose: %w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return nil, fmt.Errorf("secure temporary compose: %w", err)
	}
	if _, err := temporary.WriteString(compose); err != nil {
		temporary.Close()
		return nil, fmt.Errorf("write temporary compose: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("close temporary compose: %w", err)
	}
	return run(path, r.environment(variables))
}

func (r *DockerRunner) environment(variables map[string]string) []string {
	values := make(map[string]string, len(r.hostEnvironment)+len(variables))
	for key, value := range r.hostEnvironment {
		values[key] = value
	}
	for key, value := range variables {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}

func validateRuntime(mode RuntimeMode, stackName, compose string, variables map[string]string) error {
	if !validRuntimeMode(mode) || !ValidStackName(stackName) || strings.TrimSpace(compose) == "" {
		return ErrInvalidRequest
	}
	for key := range variables {
		if !environmentKey.MatchString(key) || reservedEnvironmentKey(key) {
			return fmt.Errorf("environment key %q: %w", key, ErrInvalidRequest)
		}
	}
	return nil
}

func reservedEnvironmentKey(key string) bool {
	upper := strings.ToUpper(key)
	if strings.HasPrefix(upper, "DOCKER_") || strings.HasPrefix(upper, "COMPOSE_") {
		return true
	}
	for _, allowed := range allowedHostEnvironment {
		if upper == allowed {
			return true
		}
	}
	return false
}

func ValidStackName(name string) bool {
	return stackNamePattern.MatchString(name)
}

func deployFromRuntime(request RuntimeRequest) DeployRequest {
	return DeployRequest{ApplicationID: request.ApplicationID, RuntimeMode: request.RuntimeMode, StackName: request.StackName, Compose: request.Compose, Environment: request.Environment}
}

var _ Runner = (*DockerRunner)(nil)
var _ RuntimeWaiter = (*DockerRunner)(nil)
