package deployment

import (
	"context"
	"errors"
	"time"
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusPreparing   Status = "preparing"
	StatusDeploying   Status = "deploying"
	StatusWaiting     Status = "waiting"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
	StatusInterrupted Status = "interrupted"
)

type RuntimeMode string

const (
	RuntimeStandalone RuntimeMode = "standalone"
	RuntimeSwarm      RuntimeMode = "swarm"
)

var (
	ErrApplicationBusy = errors.New("application already has a deployment in progress")
	ErrNotFound        = errors.New("deployment not found")
	ErrNotCancellable  = errors.New("deployment cannot be cancelled")
	ErrInvalidRequest  = errors.New("invalid deployment request")
	ErrUnsupported     = errors.New("runtime operation is not supported")
)

type Deployment struct {
	ID             int64       `json:"id"`
	ApplicationID  int64       `json:"application_id"`
	RuntimeMode    RuntimeMode `json:"runtime_mode"`
	Status         Status      `json:"status"`
	SourceRevision int64       `json:"source_revision"`
	StackName      string      `json:"stack_name"`
	TriggerType    string      `json:"trigger_type"`
	Output         string      `json:"output"`
	ErrorCode      string      `json:"error_code"`
	ErrorMessage   string      `json:"error_message"`
	CreatedAt      time.Time   `json:"created_at"`
	StartedAt      *time.Time  `json:"started_at,omitempty"`
	FinishedAt     *time.Time  `json:"finished_at,omitempty"`
}

type NewDeployment struct {
	ApplicationID  int64
	RuntimeMode    RuntimeMode
	SourceRevision int64
	StackName      string
	TriggerType    string
}

type DeploymentUpdate struct {
	Status       Status
	Output       string
	ErrorCode    string
	ErrorMessage string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

type Store interface {
	Create(context.Context, NewDeployment) (Deployment, error)
	Get(context.Context, int64) (Deployment, error)
	ListByApplication(context.Context, int64, int) ([]Deployment, error)
	Update(context.Context, int64, DeploymentUpdate) error
	InterruptTransient(context.Context, time.Time) (int64, error)
	UpdateApplicationStatus(context.Context, int64, string, time.Time) error
}

type QueueRequest struct {
	ApplicationID  int64
	RuntimeMode    RuntimeMode
	SourceRevision int64
	StackName      string
	TriggerType    string
	Compose        string
	Environment    map[string]string
	SecretValues   []string
}

type DeployRequest struct {
	DeploymentID   int64
	ApplicationID  int64
	RuntimeMode    RuntimeMode
	SourceRevision int64
	StackName      string
	Compose        string
	Environment    map[string]string
}

type DeployResult struct {
	Output       string
	ErrorCode    string
	ErrorMessage string
	Err          error
}

type RuntimeRequest struct {
	ApplicationID int64
	RuntimeMode   RuntimeMode
	StackName     string
	Compose       string
	Environment   map[string]string
}

type Operation string

const (
	OperationStart   Operation = "start"
	OperationStop    Operation = "stop"
	OperationRestart Operation = "restart"
	OperationRemove  Operation = "remove"
)

type Runner interface {
	Deploy(context.Context, DeployRequest) DeployResult
	Stop(context.Context, RuntimeRequest) error
	Start(context.Context, RuntimeRequest) error
	Restart(context.Context, RuntimeRequest) error
	Remove(context.Context, RuntimeRequest) error
}

type LogReader interface {
	Logs(context.Context, RuntimeRequest, string, int) (string, error)
}

// RuntimeWaiter verifies the real runtime after the deploy command exits. An
// engine never considers a deployment successful from the CLI exit code alone.
type RuntimeWaiter interface {
	WaitReady(context.Context, RuntimeRequest) error
}

type Event struct {
	Name          string     `json:"event"`
	DeploymentID  int64      `json:"deployment_id"`
	ApplicationID int64      `json:"application_id"`
	Status        Status     `json:"status,omitempty"`
	Deployment    Deployment `json:"deployment"`
}

type Emitter interface {
	Emit(Event)
}

type EmitterFunc func(Event)

func (f EmitterFunc) Emit(event Event) {
	if f != nil {
		f(event)
	}
}

func (s Status) Transient() bool {
	switch s {
	case StatusQueued, StatusPreparing, StatusDeploying, StatusWaiting:
		return true
	default:
		return false
	}
}

func validRuntimeMode(mode RuntimeMode) bool {
	return mode == RuntimeStandalone || mode == RuntimeSwarm
}
