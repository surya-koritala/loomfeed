package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// A2ATaskRepo persists the task state returned by the A2A gateway.
type A2ATaskRepo struct {
	pool *pgxpool.Pool
}

func NewA2ATaskRepo(pool *pgxpool.Pool) *A2ATaskRepo {
	return &A2ATaskRepo{pool: pool}
}

const a2aTaskColumns = `task_id, participant_id, skill, request, state,
	status_message, artifacts, created_at, updated_at, completed_at`

func scanA2ATask(row pgx.Row) (*models.A2ATask, error) {
	var task models.A2ATask
	if err := row.Scan(
		&task.TaskID, &task.ParticipantID, &task.Skill, &task.Request, &task.State,
		&task.StatusMessage, &task.Artifacts, &task.CreatedAt, &task.UpdatedAt, &task.CompletedAt,
	); err != nil {
		return nil, err
	}
	return &task, nil
}

// CreateTask inserts the submitted task once. A repeated participant/task ID
// returns the original record with created=false so mutating skills remain
// idempotent across caller retries.
func (r *A2ATaskRepo) CreateTask(ctx context.Context, task *models.A2ATask) (*models.A2ATask, bool, error) {
	if task == nil || task.ParticipantID == "" || task.TaskID == "" || len(task.TaskID) > 255 || task.Skill == "" {
		return nil, false, fmt.Errorf("invalid A2A task")
	}
	if !json.Valid(task.Request) {
		return nil, false, fmt.Errorf("invalid A2A task request JSON")
	}
	created, err := scanA2ATask(r.pool.QueryRow(ctx, `
		INSERT INTO a2a_tasks (participant_id, task_id, skill, request, state)
		VALUES ($1, $2, $3, $4, 'submitted')
		ON CONFLICT (participant_id, task_id) DO NOTHING
		RETURNING `+a2aTaskColumns,
		task.ParticipantID, task.TaskID, task.Skill, task.Request,
	))
	if err == nil {
		return created, true, nil
	}
	if err != pgx.ErrNoRows {
		return nil, false, fmt.Errorf("create A2A task: %w", err)
	}
	existing, found, getErr := r.GetTask(ctx, task.ParticipantID, task.TaskID)
	if getErr != nil {
		return nil, false, getErr
	}
	if !found {
		return nil, false, fmt.Errorf("create A2A task: conflict row disappeared")
	}
	return existing, false, nil
}

// GetTask returns only a task owned by the authenticated participant. Missing
// and cross-owner IDs both produce found=false to avoid existence disclosure.
func (r *A2ATaskRepo) GetTask(ctx context.Context, participantID, taskID string) (*models.A2ATask, bool, error) {
	task, err := scanA2ATask(r.pool.QueryRow(ctx, `
		SELECT `+a2aTaskColumns+`
		FROM a2a_tasks WHERE participant_id = $1 AND task_id = $2`, participantID, taskID,
	))
	if err == pgx.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get A2A task: %w", err)
	}
	return task, true, nil
}

// TransitionTask performs an expected-state update so terminal tasks cannot
// regress or be overwritten by a concurrent retry.
func (r *A2ATaskRepo) TransitionTask(
	ctx context.Context,
	participantID, taskID string,
	from, to models.A2ATaskState,
	statusMessage string,
	artifacts json.RawMessage,
) (*models.A2ATask, error) {
	valid := (from == models.A2ATaskSubmitted && to == models.A2ATaskWorking) ||
		(from == models.A2ATaskWorking && (to == models.A2ATaskCompleted || to == models.A2ATaskFailed))
	if !valid {
		return nil, fmt.Errorf("invalid A2A task transition %s -> %s", from, to)
	}
	if len(artifacts) == 0 {
		artifacts = json.RawMessage(`[]`)
	}
	if !json.Valid(artifacts) {
		return nil, fmt.Errorf("invalid A2A task artifacts JSON")
	}
	task, err := scanA2ATask(r.pool.QueryRow(ctx, `
		UPDATE a2a_tasks SET
			state = $4,
			status_message = $5,
			artifacts = $6,
			updated_at = NOW(),
			completed_at = CASE WHEN $4::a2a_task_state IN ('completed', 'failed') THEN NOW() ELSE NULL END
		WHERE participant_id = $1 AND task_id = $2 AND state = $3
		RETURNING `+a2aTaskColumns,
		participantID, taskID, from, to, statusMessage, artifacts,
	))
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("A2A task %s is missing or no longer in state %s", taskID, from)
	}
	if err != nil {
		return nil, fmt.Errorf("transition A2A task: %w", err)
	}
	return task, nil
}
