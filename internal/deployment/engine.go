package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultDeploymentTimeout = 5 * time.Minute
	storeTimeout             = 5 * time.Second
)

type EngineOptions struct {
	Timeout     time.Duration
	OutputLimit int
	Emitter     Emitter
	Locks       *ApplicationLocks
}

type Engine struct {
	store       Store
	runner      Runner
	emitter     Emitter
	locks       *ApplicationLocks
	timeout     time.Duration
	outputLimit int
	ctx         context.Context
	stop        context.CancelFunc

	mu      sync.Mutex
	cancels map[int64]context.CancelFunc
	closing bool
	wg      sync.WaitGroup
}

func NewEngine(store Store, runner Runner, options EngineOptions) *Engine {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultDeploymentTimeout
	}
	outputLimit := options.OutputLimit
	if outputLimit <= 0 {
		outputLimit = DefaultOutputLimit
	}
	locks := options.Locks
	if locks == nil {
		locks = NewApplicationLocks()
	}
	ctx, stop := context.WithCancel(context.Background())
	return &Engine{
		store:       store,
		runner:      runner,
		emitter:     options.Emitter,
		locks:       locks,
		timeout:     timeout,
		outputLimit: outputLimit,
		ctx:         ctx,
		stop:        stop,
		cancels:     make(map[int64]context.CancelFunc),
	}
}

// Recover marks work left in a transient state by an earlier process as interrupted.
// It must be called once during application startup before accepting deployment requests.
func (e *Engine) Recover(ctx context.Context) (int64, error) {
	return e.store.InterruptTransient(ctx, time.Now().UTC())
}

// Queue persists a queued deployment and starts it on an engine-owned context. The
// returned deployment is suitable for an immediate HTTP 202 response.
func (e *Engine) Queue(ctx context.Context, request QueueRequest) (Deployment, error) {
	if err := validateQueueRequest(request); err != nil {
		return Deployment{}, err
	}
	release, acquired := e.locks.TryAcquire(request.ApplicationID)
	if !acquired {
		return Deployment{}, ErrApplicationBusy
	}
	deployment, err := e.store.Create(ctx, NewDeployment{
		ApplicationID:  request.ApplicationID,
		RuntimeMode:    request.RuntimeMode,
		SourceRevision: request.SourceRevision,
		StackName:      request.StackName,
		TriggerType:    request.TriggerType,
	})
	if err != nil {
		release()
		return Deployment{}, err
	}

	runContext, cancel := context.WithTimeout(e.ctx, e.timeout)
	e.mu.Lock()
	e.cancels[deployment.ID] = cancel
	e.mu.Unlock()
	e.emit(Event{Name: "deployment.queued", DeploymentID: deployment.ID, ApplicationID: deployment.ApplicationID, Status: deployment.Status, Deployment: deployment})

	e.wg.Add(1)
	go e.run(runContext, cancel, release, deployment, request)
	return deployment, nil
}

func (e *Engine) Get(ctx context.Context, id int64) (Deployment, error) {
	return e.store.Get(ctx, id)
}

func (e *Engine) ListByApplication(ctx context.Context, applicationID int64, limit int) ([]Deployment, error) {
	return e.store.ListByApplication(ctx, applicationID, limit)
}

func (e *Engine) Operate(ctx context.Context, operation Operation, request RuntimeRequest) error {
	if !validRuntimeMode(request.RuntimeMode) || !ValidStackName(request.StackName) || strings.TrimSpace(request.Compose) == "" {
		return ErrInvalidRequest
	}
	operationContext, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	var err error
	switch operation {
	case OperationStart:
		err = e.runner.Start(operationContext, request)
	case OperationStop:
		err = e.runner.Stop(operationContext, request)
	case OperationRestart:
		err = e.runner.Restart(operationContext, request)
	case OperationRemove:
		err = e.runner.Remove(operationContext, request)
	default:
		return ErrInvalidRequest
	}
	if err != nil {
		return err
	}
	status := map[Operation]string{OperationStart: "running", OperationStop: "stopped", OperationRestart: "running", OperationRemove: "not_deployed"}[operation]
	if err := e.store.UpdateApplicationStatus(ctx, request.ApplicationID, status, time.Now().UTC()); err != nil {
		return err
	}
	e.emit(Event{Name: "application.runtime_updated", ApplicationID: request.ApplicationID, Status: StatusSucceeded})
	return nil
}

func (e *Engine) Cancel(ctx context.Context, id int64) error {
	deployment, err := e.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !deployment.Status.Transient() || deployment.Status == StatusWaiting {
		return ErrNotCancellable
	}
	e.mu.Lock()
	cancel := e.cancels[id]
	e.mu.Unlock()
	if cancel == nil {
		return ErrNotCancellable
	}
	cancel()
	return nil
}

func (e *Engine) Close() {
	e.mu.Lock()
	e.closing = true
	e.mu.Unlock()
	e.stop()
	e.wg.Wait()
}

func (e *Engine) Wait() {
	e.wg.Wait()
}

func (e *Engine) run(ctx context.Context, cancel context.CancelFunc, release func(), deployment Deployment, request QueueRequest) {
	defer e.wg.Done()
	defer cancel()
	defer release()
	defer func() {
		e.mu.Lock()
		delete(e.cancels, deployment.ID)
		e.mu.Unlock()
	}()

	if e.finishForContext(ctx, &deployment, request, "") {
		return
	}
	started := time.Now().UTC()
	if err := e.transition(&deployment, DeploymentUpdate{Status: StatusPreparing, StartedAt: &started}); err != nil {
		e.finish(&deployment, request, StatusFailed, "persistence_failed", "Não foi possível registrar o início da publicação.", "")
		return
	}
	if e.finishForContext(ctx, &deployment, request, "") {
		return
	}
	if err := e.transition(&deployment, DeploymentUpdate{Status: StatusDeploying, StartedAt: &started}); err != nil {
		e.finish(&deployment, request, StatusFailed, "persistence_failed", "Não foi possível registrar a publicação.", "")
		return
	}
	if e.finishForContext(ctx, &deployment, request, "") {
		return
	}

	result := e.runner.Deploy(ctx, DeployRequest{
		DeploymentID:   deployment.ID,
		ApplicationID:  request.ApplicationID,
		RuntimeMode:    request.RuntimeMode,
		SourceRevision: request.SourceRevision,
		StackName:      request.StackName,
		Compose:        request.Compose,
		Environment:    copyEnvironment(request.Environment),
	})
	sensitive := append([]string{request.Compose}, request.SecretValues...)
	output := SanitizeOutput(result.Output, sensitive, e.outputLimit)
	if e.finishForContext(ctx, &deployment, request, output) {
		return
	}
	if result.Err != nil {
		code := result.ErrorCode
		if code == "" {
			code = "deployment_failed"
		}
		message := result.ErrorMessage
		if message == "" {
			message = "O Docker não conseguiu publicar a aplicação."
		}
		e.finish(&deployment, request, StatusFailed, code, SanitizeOutput(message, sensitive, 1024), output)
		return
	}
	if err := e.transition(&deployment, DeploymentUpdate{Status: StatusWaiting, StartedAt: &started, Output: output}); err != nil {
		e.finish(&deployment, request, StatusFailed, "persistence_failed", "Não foi possível registrar a publicação.", output)
		return
	}
	waiter, ok := e.runner.(RuntimeWaiter)
	if !ok {
		e.finish(&deployment, request, StatusFailed, "runtime_verification_unavailable", "Não foi possível verificar o runtime publicado.", output)
		return
	}
	if err := waiter.WaitReady(ctx, RuntimeRequest{ApplicationID: request.ApplicationID, RuntimeMode: request.RuntimeMode, StackName: request.StackName, Compose: request.Compose, Environment: copyEnvironment(request.Environment)}); err != nil {
		if e.finishForContext(ctx, &deployment, request, output) {
			return
		}
		e.finish(&deployment, request, StatusFailed, "runtime_not_ready", "O runtime não ficou pronto após a publicação.", output)
		return
	}
	if e.finishForContext(ctx, &deployment, request, output) {
		return
	}
	e.finish(&deployment, request, StatusSucceeded, "", "", output)
}

func (e *Engine) transition(deployment *Deployment, update DeploymentUpdate) error {
	ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
	defer cancel()
	if err := e.store.Update(ctx, deployment.ID, update); err != nil {
		return err
	}
	applyUpdate(deployment, update)
	e.emit(Event{Name: "deployment." + string(update.Status), DeploymentID: deployment.ID, ApplicationID: deployment.ApplicationID, Status: update.Status, Deployment: *deployment})
	return nil
}

func (e *Engine) finishForContext(ctx context.Context, deployment *Deployment, request QueueRequest, output string) bool {
	e.mu.Lock()
	closing := e.closing
	e.mu.Unlock()
	if closing && errors.Is(ctx.Err(), context.Canceled) {
		e.finish(deployment, request, StatusInterrupted, "stackhost_restarted", "A publicação foi interrompida pelo reinício do StackHost.", output)
		return true
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		e.finish(deployment, request, StatusFailed, "deployment_timeout", "A publicação excedeu o tempo limite.", output)
		return true
	case errors.Is(ctx.Err(), context.Canceled):
		e.finish(deployment, request, StatusCancelled, "deployment_cancelled", "A publicação foi cancelada.", output)
		return true
	default:
		return false
	}
}

func (e *Engine) finish(deployment *Deployment, request QueueRequest, status Status, errorCode, errorMessage, output string) {
	finished := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
	defer cancel()
	update := DeploymentUpdate{Status: status, Output: output, ErrorCode: errorCode, ErrorMessage: errorMessage, StartedAt: deployment.StartedAt, FinishedAt: &finished}
	if err := e.store.Update(ctx, deployment.ID, update); err != nil {
		return
	}
	applyUpdate(deployment, update)
	e.emit(Event{Name: "deployment." + string(status), DeploymentID: deployment.ID, ApplicationID: deployment.ApplicationID, Status: status, Deployment: *deployment})
	if status == StatusSucceeded {
		if err := e.store.UpdateApplicationStatus(ctx, request.ApplicationID, "running", finished); err == nil {
			e.emit(Event{Name: "application.runtime_updated", DeploymentID: deployment.ID, ApplicationID: deployment.ApplicationID, Status: status, Deployment: *deployment})
		}
	}
}

func (e *Engine) emit(event Event) {
	if e.emitter == nil {
		return
	}
	defer func() { _ = recover() }()
	e.emitter.Emit(event)
}

func validateQueueRequest(request QueueRequest) error {
	if request.ApplicationID <= 0 || request.SourceRevision <= 0 || !validRuntimeMode(request.RuntimeMode) || !ValidStackName(request.StackName) || request.TriggerType != "manual" || strings.TrimSpace(request.Compose) == "" {
		return fmt.Errorf("%w: deployment fields", ErrInvalidRequest)
	}
	if err := validateRuntime(request.RuntimeMode, request.StackName, request.Compose, request.Environment); err != nil {
		return err
	}
	return nil
}

func applyUpdate(deployment *Deployment, update DeploymentUpdate) {
	deployment.Status = update.Status
	deployment.Output = update.Output
	deployment.ErrorCode = update.ErrorCode
	deployment.ErrorMessage = update.ErrorMessage
	if deployment.StartedAt == nil && update.StartedAt != nil {
		value := *update.StartedAt
		deployment.StartedAt = &value
	}
	if update.FinishedAt != nil {
		value := *update.FinishedAt
		deployment.FinishedAt = &value
	}
}

func copyEnvironment(environment map[string]string) map[string]string {
	copy := make(map[string]string, len(environment))
	for key, value := range environment {
		copy[key] = value
	}
	return copy
}
