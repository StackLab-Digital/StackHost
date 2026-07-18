package deployment

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	deploy func(context.Context, DeployRequest) DeployResult
	wait   func(context.Context, RuntimeRequest) error
}

func (f *fakeRunner) Deploy(ctx context.Context, request DeployRequest) DeployResult {
	if f.deploy != nil {
		return f.deploy(ctx, request)
	}
	return DeployResult{}
}

func (f *fakeRunner) WaitReady(ctx context.Context, request RuntimeRequest) error {
	if f.wait != nil {
		return f.wait(ctx, request)
	}
	return nil
}

func (f *fakeRunner) Stop(context.Context, RuntimeRequest) error    { return nil }
func (f *fakeRunner) Start(context.Context, RuntimeRequest) error   { return nil }
func (f *fakeRunner) Restart(context.Context, RuntimeRequest) error { return nil }
func (f *fakeRunner) Remove(context.Context, RuntimeRequest) error  { return nil }

func queueRequest(applicationID int64) QueueRequest {
	return QueueRequest{
		ApplicationID:  applicationID,
		RuntimeMode:    RuntimeStandalone,
		SourceRevision: 1,
		StackName:      "app" + string(rune('0'+applicationID)),
		TriggerType:    "manual",
		Compose:        "services:\n  web:\n    image: nginx:alpine",
		Environment:    map[string]string{"APP_ENV": "test"},
	}
}

func TestEngineAllowsDifferentApplicationsAndCleansSameApplicationLock(t *testing.T) {
	db := deploymentTestDB(t)
	store := NewSQLStore(db)
	started := make(chan int64, 2)
	release := make(chan struct{})
	runner := &fakeRunner{deploy: func(ctx context.Context, request DeployRequest) DeployResult {
		started <- request.ApplicationID
		select {
		case <-release:
			return DeployResult{Output: "deployed"}
		case <-ctx.Done():
			return DeployResult{Err: ctx.Err()}
		}
	}}
	engine := NewEngine(store, runner, EngineOptions{Timeout: time.Second})
	defer engine.Close()
	first, err := engine.Queue(context.Background(), queueRequest(1))
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Queue(context.Background(), queueRequest(2))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("deployments for different applications did not run concurrently")
		}
	}
	if _, err := engine.Queue(context.Background(), queueRequest(1)); !errors.Is(err, ErrApplicationBusy) {
		t.Fatalf("same application queue error = %v", err)
	}
	close(release)
	engine.Wait()
	for _, id := range []int64{first.ID, second.ID} {
		item, err := store.Get(context.Background(), id)
		if err != nil || item.Status != StatusSucceeded {
			t.Fatalf("deployment %d = %#v, err=%v", id, item, err)
		}
	}
	retry, err := engine.Queue(context.Background(), queueRequest(1))
	if err != nil {
		t.Fatalf("lock was not cleaned: %v", err)
	}
	engine.Wait()
	if item, _ := store.Get(context.Background(), retry.ID); item.Status != StatusSucceeded {
		t.Fatalf("retry status = %s", item.Status)
	}
}

func TestEngineUsesOwnContextInsteadOfRequestContext(t *testing.T) {
	store := NewSQLStore(deploymentTestDB(t))
	started := make(chan struct{})
	release := make(chan struct{})
	runner := &fakeRunner{deploy: func(ctx context.Context, _ DeployRequest) DeployResult {
		close(started)
		select {
		case <-release:
			return DeployResult{}
		case <-ctx.Done():
			return DeployResult{Err: ctx.Err()}
		}
	}}
	engine := NewEngine(store, runner, EngineOptions{Timeout: time.Second})
	defer engine.Close()
	requestContext, cancelRequest := context.WithCancel(context.Background())
	item, err := engine.Queue(requestContext, queueRequest(1))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancelRequest()
	close(release)
	engine.Wait()
	got, _ := store.Get(context.Background(), item.ID)
	if got.Status != StatusSucceeded {
		t.Fatalf("request cancellation changed deployment status to %s", got.Status)
	}
}

func TestEngineImmediateCancellationFinishesCancelled(t *testing.T) {
	store := NewSQLStore(deploymentTestDB(t))
	runner := &fakeRunner{deploy: func(ctx context.Context, _ DeployRequest) DeployResult {
		<-ctx.Done()
		return DeployResult{Err: ctx.Err()}
	}}
	engine := NewEngine(store, runner, EngineOptions{Timeout: time.Second})
	defer engine.Close()
	item, err := engine.Queue(context.Background(), queueRequest(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Cancel(context.Background(), item.ID); err != nil {
		t.Fatal(err)
	}
	engine.Wait()
	got, _ := store.Get(context.Background(), item.ID)
	if got.Status != StatusCancelled || got.ErrorCode != "deployment_cancelled" {
		t.Fatalf("cancelled deployment = %#v", got)
	}
}

func TestEngineTimesOutWhileWaitingForRuntime(t *testing.T) {
	store := NewSQLStore(deploymentTestDB(t))
	var eventsMu sync.Mutex
	var events []string
	runner := &fakeRunner{wait: func(ctx context.Context, _ RuntimeRequest) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	engine := NewEngine(store, runner, EngineOptions{
		Timeout: 30 * time.Millisecond,
		Emitter: EmitterFunc(func(event Event) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, event.Name)
		}),
	})
	defer engine.Close()
	item, err := engine.Queue(context.Background(), queueRequest(1))
	if err != nil {
		t.Fatal(err)
	}
	engine.Wait()
	got, _ := store.Get(context.Background(), item.ID)
	if got.Status != StatusFailed || got.ErrorCode != "deployment_timeout" {
		t.Fatalf("timed out deployment = %#v", got)
	}
	eventsMu.Lock()
	joined := strings.Join(events, ",")
	eventsMu.Unlock()
	if !strings.Contains(joined, "deployment.waiting") || !strings.Contains(joined, "deployment.failed") {
		t.Fatalf("events = %s", joined)
	}
}

func TestEngineRequiresRuntimeReadinessAndPreservesApplicationStatus(t *testing.T) {
	db := deploymentTestDB(t)
	store := NewSQLStore(db)
	if _, err := db.Exec(`UPDATE applications SET status='running' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{wait: func(context.Context, RuntimeRequest) error { return errors.New("degraded") }}
	engine := NewEngine(store, runner, EngineOptions{Timeout: time.Second})
	defer engine.Close()
	item, err := engine.Queue(context.Background(), queueRequest(1))
	if err != nil {
		t.Fatal(err)
	}
	engine.Wait()
	got, _ := store.Get(context.Background(), item.ID)
	if got.Status != StatusFailed || got.ErrorCode != "runtime_not_ready" {
		t.Fatalf("degraded deployment = %#v", got)
	}
	var applicationStatus string
	if err := db.QueryRow(`SELECT status FROM applications WHERE id=1`).Scan(&applicationStatus); err != nil || applicationStatus != "running" {
		t.Fatalf("application status = %q, err=%v", applicationStatus, err)
	}
}

func TestEngineRedactsAndLimitsPersistedOutput(t *testing.T) {
	store := NewSQLStore(deploymentTestDB(t))
	runner := &fakeRunner{deploy: func(context.Context, DeployRequest) DeployResult {
		return DeployResult{Output: "\x1b[31mtopsecret-x-" + strings.Repeat("z", 256)}
	}}
	engine := NewEngine(store, runner, EngineOptions{Timeout: time.Second, OutputLimit: 64})
	defer engine.Close()
	request := queueRequest(1)
	request.SecretValues = []string{"topsecret", "x"}
	item, err := engine.Queue(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	engine.Wait()
	got, _ := store.Get(context.Background(), item.ID)
	if got.Status != StatusSucceeded || len(got.Output) > 64 || strings.Contains(got.Output, "topsecret") || strings.Contains(got.Output, "\x1b") || strings.Contains(got.Output, "-x-") {
		t.Fatalf("unsafe output (%d bytes): %q", len(got.Output), got.Output)
	}
}

func TestEngineRecoveryAllowsAReexecution(t *testing.T) {
	store := NewSQLStore(deploymentTestDB(t))
	stale, err := store.Create(context.Background(), NewDeployment{ApplicationID: 1, RuntimeMode: RuntimeStandalone, SourceRevision: 1, StackName: "app1"})
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(store, &fakeRunner{}, EngineOptions{Timeout: time.Second})
	defer engine.Close()
	if recovered, err := engine.Recover(context.Background()); err != nil || recovered != 1 {
		t.Fatalf("recovered = %d, err=%v", recovered, err)
	}
	if got, _ := store.Get(context.Background(), stale.ID); got.Status != StatusInterrupted {
		t.Fatalf("stale deployment status = %s", got.Status)
	}
	retry, err := engine.Queue(context.Background(), queueRequest(1))
	if err != nil {
		t.Fatal(err)
	}
	engine.Wait()
	if got, _ := store.Get(context.Background(), retry.ID); got.Status != StatusSucceeded {
		t.Fatalf("retry status = %s", got.Status)
	}
}
