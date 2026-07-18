package deployment

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type commandCall struct {
	name        string
	arguments   []string
	environment []string
}

type recordingExecutor struct {
	mu    sync.Mutex
	calls []commandCall
	run   func(commandCall) ([]byte, error)
}

func (e *recordingExecutor) Run(_ context.Context, name string, arguments, environment []string) ([]byte, error) {
	call := commandCall{name: name, arguments: append([]string(nil), arguments...), environment: append([]string(nil), environment...)}
	e.mu.Lock()
	e.calls = append(e.calls, call)
	e.mu.Unlock()
	if e.run != nil {
		return e.run(call)
	}
	return nil, nil
}

func TestDockerRunnerDeployArgumentsEnvironmentAndTemporaryCleanup(t *testing.T) {
	compose := "services:\n  web:\n    image: nginx:alpine"
	for _, test := range []struct {
		name       string
		mode       RuntimeMode
		pathIndex  int
		wantPrefix []string
		wantSuffix []string
	}{
		{name: "standalone", mode: RuntimeStandalone, pathIndex: 4, wantPrefix: []string{"compose", "--project-name", "demo", "--file"}, wantSuffix: []string{"up", "--detach", "--remove-orphans"}},
		{name: "swarm", mode: RuntimeSwarm, pathIndex: 3, wantPrefix: []string{"stack", "deploy", "--compose-file"}, wantSuffix: []string{"--prune", "demo"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var temporaryPath string
			executor := &recordingExecutor{run: func(call commandCall) ([]byte, error) {
				temporaryPath = call.arguments[test.pathIndex]
				info, err := os.Stat(temporaryPath)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0600 {
					t.Fatalf("temporary mode = %o", info.Mode().Perm())
				}
				content, err := os.ReadFile(temporaryPath)
				if err != nil || string(content) != compose {
					t.Fatalf("temporary content = %q, err=%v", content, err)
				}
				return []byte("ok"), nil
			}}
			runner := NewDockerRunner(
				WithCommandExecutor(executor),
				WithTemporaryDir(t.TempDir()),
				WithHostEnvironment(map[string]string{"PATH": "/bin", "DOCKER_HOST": "unix:///safe.sock", "STACKHOST_ENCRYPTION_KEY": "must-not-leak"}),
			)
			result := runner.Deploy(context.Background(), DeployRequest{RuntimeMode: test.mode, StackName: "demo", Compose: compose, Environment: map[string]string{"APP_TOKEN": "secret"}})
			if result.Err != nil || result.Output != "ok" {
				t.Fatalf("deploy result = %#v", result)
			}
			if temporaryPath == "" {
				t.Fatal("executor did not receive a temporary file")
			}
			if _, err := os.Stat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary file was not removed: %v", err)
			}
			executor.mu.Lock()
			call := executor.calls[0]
			executor.mu.Unlock()
			if call.name != "docker" || !reflect.DeepEqual(call.arguments[:len(test.wantPrefix)], test.wantPrefix) || !reflect.DeepEqual(call.arguments[len(call.arguments)-len(test.wantSuffix):], test.wantSuffix) {
				t.Fatalf("docker invocation = %s %#v", call.name, call.arguments)
			}
			for _, want := range []string{"APP_TOKEN=secret", "DOCKER_HOST=unix:///safe.sock", "PATH=/bin"} {
				if !slices.Contains(call.environment, want) {
					t.Fatalf("environment missing %q: %#v", want, call.environment)
				}
			}
			if slices.ContainsFunc(call.environment, func(value string) bool { return strings.HasPrefix(value, "STACKHOST_") }) {
				t.Fatalf("StackHost secret leaked to Docker: %#v", call.environment)
			}
		})
	}
}

func TestDockerRunnerRemovesTemporaryFileOnCommandFailure(t *testing.T) {
	var temporaryPath string
	executor := &recordingExecutor{run: func(call commandCall) ([]byte, error) {
		temporaryPath = call.arguments[4]
		return []byte("failure"), errors.New("exit")
	}}
	runner := NewDockerRunner(WithCommandExecutor(executor), WithTemporaryDir(t.TempDir()), WithHostEnvironment(nil))
	result := runner.Deploy(context.Background(), DeployRequest{RuntimeMode: RuntimeStandalone, StackName: "demo", Compose: "services:\n  web:\n    image: nginx"})
	if result.Err == nil {
		t.Fatal("expected deploy failure")
	}
	if _, err := os.Stat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file survived command failure: %v", err)
	}
}

func TestDockerRunnerMapsStackHostDockerHostWithoutInheritingOtherVariables(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("STACKHOST_DOCKER_HOST", "unix:///stackhost.sock")
	t.Setenv("STACKHOST_SESSION_SECRET", "never-pass-this")
	runner := NewDockerRunner()
	environment := runner.environment(nil)
	if !slices.Contains(environment, "DOCKER_HOST=unix:///stackhost.sock") {
		t.Fatalf("mapped Docker host missing: %#v", environment)
	}
	if slices.Contains(environment, "STACKHOST_SESSION_SECRET=never-pass-this") {
		t.Fatalf("StackHost environment leaked: %#v", environment)
	}
}

func TestDockerRunnerRejectsApplicationDockerControlVariables(t *testing.T) {
	runner := NewDockerRunner(WithCommandExecutor(&recordingExecutor{}), WithHostEnvironment(nil))
	result := runner.Deploy(context.Background(), DeployRequest{RuntimeMode: RuntimeStandalone, StackName: "demo", Compose: "services:\n  web:\n    image: nginx", Environment: map[string]string{"DOCKER_HOST": "tcp://attacker:2375"}})
	if !errors.Is(result.Err, ErrInvalidRequest) {
		t.Fatalf("reserved environment error = %v", result.Err)
	}
}

func TestDockerRunnerWaitReadyPollsStandaloneAndSwarmRuntime(t *testing.T) {
	compose := "services:\n  web:\n    image: nginx"
	standaloneChecks := 0
	standalone := &recordingExecutor{run: func(commandCall) ([]byte, error) {
		standaloneChecks++
		if standaloneChecks == 1 {
			return []byte(`[{"Service":"web","State":"running","Health":"starting"}]`), nil
		}
		return []byte(`[{"Service":"web","State":"running","Health":"healthy"}]`), nil
	}}
	runner := NewDockerRunner(WithCommandExecutor(standalone), WithTemporaryDir(t.TempDir()), WithHostEnvironment(nil), WithPollInterval(time.Millisecond))
	if err := runner.WaitReady(context.Background(), RuntimeRequest{RuntimeMode: RuntimeStandalone, StackName: "demo", Compose: compose}); err != nil {
		t.Fatal(err)
	}
	if standaloneChecks < 2 {
		t.Fatalf("standalone checks = %d", standaloneChecks)
	}

	swarm := &recordingExecutor{run: func(call commandCall) ([]byte, error) {
		if call.arguments[1] == "services" {
			return []byte(`{"Name":"demo_web","Replicas":"1/1"}`), nil
		}
		return []byte(`{"CurrentState":"Running 1 second ago","Error":""}`), nil
	}}
	runner = NewDockerRunner(WithCommandExecutor(swarm), WithHostEnvironment(nil), WithPollInterval(time.Millisecond))
	if err := runner.WaitReady(context.Background(), RuntimeRequest{RuntimeMode: RuntimeSwarm, StackName: "demo", Compose: compose}); err != nil {
		t.Fatal(err)
	}
}

func TestDockerRunnerWaitReadyStopsAtContextDeadline(t *testing.T) {
	executor := &recordingExecutor{run: func(commandCall) ([]byte, error) {
		return []byte(`[{"Service":"web","State":"exited"}]`), nil
	}}
	runner := NewDockerRunner(WithCommandExecutor(executor), WithTemporaryDir(t.TempDir()), WithHostEnvironment(nil), WithPollInterval(time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := runner.WaitReady(ctx, RuntimeRequest{RuntimeMode: RuntimeStandalone, StackName: "demo", Compose: "services:\n  web:\n    image: nginx"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait error = %v", err)
	}
}

func TestCommandExecutorBoundsCaptureBeforeReturning(t *testing.T) {
	if os.Getenv("STACKHOST_HELPER_PROCESS") == "1" {
		_, _ = os.Stdout.WriteString(strings.Repeat("o", 512))
		_, _ = os.Stderr.WriteString(strings.Repeat("e", 512))
		os.Exit(0)
	}
	executor := commandExecutor{limit: 32}
	output, err := executor.Run(context.Background(), os.Args[0], []string{"-test.run=TestCommandExecutorBoundsCaptureBeforeReturning"}, []string{"STACKHOST_HELPER_PROCESS=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 32 {
		t.Fatalf("captured %d bytes, want 32", len(output))
	}
}

func TestSanitizeOutputRedactsShortSecretsAndKeepsStrictLimit(t *testing.T) {
	output := SanitizeOutput("a-b-secret-"+strings.Repeat("x", 100), []string{"a", "b", "secret"}, 24)
	if len(output) > 24 || strings.Contains(output, "secret") || strings.Contains(output, "a-b") {
		t.Fatalf("sanitized output (%d bytes) = %q", len(output), output)
	}
}
