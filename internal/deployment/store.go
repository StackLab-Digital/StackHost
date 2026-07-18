package deployment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const deploymentColumns = `id,application_id,runtime_mode,status,source_revision,stack_name,trigger_type,output,error_code,error_message,created_at,started_at,finished_at`

type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) *SQLStore {
	return &SQLStore{db: db}
}

func (s *SQLStore) Create(ctx context.Context, input NewDeployment) (Deployment, error) {
	if input.TriggerType == "" {
		input.TriggerType = "manual"
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO deployments(application_id,runtime_mode,status,source_revision,stack_name,trigger_type,created_at) VALUES(?,?,?,?,?,?,?)`, input.ApplicationID, input.RuntimeMode, StatusQueued, input.SourceRevision, input.StackName, input.TriggerType, formatTime(now))
	if err != nil {
		return Deployment{}, fmt.Errorf("create deployment: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Deployment{}, fmt.Errorf("deployment id: %w", err)
	}
	return Deployment{ID: id, ApplicationID: input.ApplicationID, RuntimeMode: input.RuntimeMode, Status: StatusQueued, SourceRevision: input.SourceRevision, StackName: input.StackName, TriggerType: input.TriggerType, CreatedAt: now}, nil
}

func (s *SQLStore) Get(ctx context.Context, id int64) (Deployment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+deploymentColumns+` FROM deployments WHERE id=?`, id)
	deployment, err := scanDeployment(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Deployment{}, ErrNotFound
	}
	if err != nil {
		return Deployment{}, fmt.Errorf("get deployment: %w", err)
	}
	return deployment, nil
}

func (s *SQLStore) ListByApplication(ctx context.Context, applicationID int64, limit int) ([]Deployment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+deploymentColumns+` FROM deployments WHERE application_id=? ORDER BY created_at DESC,id DESC LIMIT ?`, applicationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer rows.Close()

	items := make([]Deployment, 0)
	for rows.Next() {
		deployment, err := scanDeployment(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan deployment: %w", err)
		}
		items = append(items, deployment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployments: %w", err)
	}
	return items, nil
}

func (s *SQLStore) Update(ctx context.Context, id int64, update DeploymentUpdate) error {
	result, err := s.db.ExecContext(ctx, `UPDATE deployments SET status=?,output=?,error_code=?,error_message=?,started_at=COALESCE(started_at,?),finished_at=? WHERE id=?`, update.Status, update.Output, update.ErrorCode, update.ErrorMessage, nullableTime(update.StartedAt), nullableTime(update.FinishedAt), id)
	if err != nil {
		return fmt.Errorf("update deployment: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deployment rows affected: %w", err)
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) InterruptTransient(ctx context.Context, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE deployments SET status=?,error_code='stackhost_restarted',error_message='A execução foi interrompida pela reinicialização do StackHost.',finished_at=? WHERE status IN (?,?,?,?)`, StatusInterrupted, formatTime(now.UTC()), StatusQueued, StatusPreparing, StatusDeploying, StatusWaiting)
	if err != nil {
		return 0, fmt.Errorf("recover deployments: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("recovered deployment count: %w", err)
	}
	return changed, nil
}

func (s *SQLStore) UpdateApplicationStatus(ctx context.Context, applicationID int64, status string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE applications SET status=?,updated_at=? WHERE id=?`, status, formatTime(now.UTC()), applicationID)
	if err != nil {
		return fmt.Errorf("update application runtime status: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("application rows affected: %w", err)
	}
	if changed == 0 {
		return fmt.Errorf("application %d: %w", applicationID, ErrNotFound)
	}
	return nil
}

type scanFunc func(...any) error

func scanDeployment(scan scanFunc) (Deployment, error) {
	var item Deployment
	var mode, status string
	var created string
	var started, finished sql.NullString
	if err := scan(&item.ID, &item.ApplicationID, &mode, &status, &item.SourceRevision, &item.StackName, &item.TriggerType, &item.Output, &item.ErrorCode, &item.ErrorMessage, &created, &started, &finished); err != nil {
		return Deployment{}, err
	}
	item.RuntimeMode = RuntimeMode(mode)
	item.Status = Status(status)
	var err error
	item.CreatedAt, err = parseTime(created)
	if err != nil {
		return Deployment{}, err
	}
	if started.Valid {
		value, err := parseTime(started.String)
		if err != nil {
			return Deployment{}, err
		}
		item.StartedAt = &value
	}
	if finished.Valid {
		value, err := parseTime(finished.String)
		if err != nil {
			return Deployment{}, err
		}
		item.FinishedAt = &value
	}
	return item, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid deployment timestamp %q: %w", value, err)
	}
	return parsed, nil
}
