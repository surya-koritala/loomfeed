package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/surya-koritala/loomfeed/internal/models"
)

// TaskStore is implemented by repository.A2ATaskRepo in the API process. The
// interface keeps protocol handling independently testable.
type TaskStore interface {
	CreateTask(context.Context, *models.A2ATask) (*models.A2ATask, bool, error)
	GetTask(context.Context, string, string) (*models.A2ATask, bool, error)
	TransitionTask(context.Context, string, string, models.A2ATaskState, models.A2ATaskState, string, json.RawMessage) (*models.A2ATask, error)
}

// memoryTaskStore is used only when the handler is constructed without a
// repository (unit tests and isolated embedding). routes.Register always wires
// the PostgreSQL store so production state survives restarts and replicas.
type memoryTaskStore struct {
	mu    sync.Mutex
	tasks map[string]*models.A2ATask
}

func newMemoryTaskStore() *memoryTaskStore {
	return &memoryTaskStore{tasks: make(map[string]*models.A2ATask)}
}

func memoryTaskKey(participantID, taskID string) string {
	return participantID + "\x00" + taskID
}

func cloneA2ATask(task *models.A2ATask) *models.A2ATask {
	if task == nil {
		return nil
	}
	cloned := *task
	cloned.Request = append(json.RawMessage(nil), task.Request...)
	cloned.Artifacts = append(json.RawMessage(nil), task.Artifacts...)
	if task.CompletedAt != nil {
		completedAt := *task.CompletedAt
		cloned.CompletedAt = &completedAt
	}
	return &cloned
}

func (s *memoryTaskStore) CreateTask(_ context.Context, task *models.A2ATask) (*models.A2ATask, bool, error) {
	if task == nil || task.ParticipantID == "" || task.TaskID == "" || len(task.TaskID) > 255 || task.Skill == "" || !json.Valid(task.Request) {
		return nil, false, fmt.Errorf("invalid A2A task")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := memoryTaskKey(task.ParticipantID, task.TaskID)
	if existing, ok := s.tasks[key]; ok {
		return cloneA2ATask(existing), false, nil
	}
	now := time.Now().UTC()
	created := cloneA2ATask(task)
	created.State = models.A2ATaskSubmitted
	created.Artifacts = json.RawMessage(`[]`)
	created.CreatedAt = now
	created.UpdatedAt = now
	s.tasks[key] = created
	return cloneA2ATask(created), true, nil
}

func (s *memoryTaskStore) GetTask(_ context.Context, participantID, taskID string) (*models.A2ATask, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[memoryTaskKey(participantID, taskID)]
	return cloneA2ATask(task), ok, nil
}

func (s *memoryTaskStore) TransitionTask(
	_ context.Context,
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
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[memoryTaskKey(participantID, taskID)]
	if !ok || task.State != from {
		return nil, fmt.Errorf("A2A task %s is missing or no longer in state %s", taskID, from)
	}
	task.State = to
	task.StatusMessage = statusMessage
	task.Artifacts = append(json.RawMessage(nil), artifacts...)
	task.UpdatedAt = time.Now().UTC()
	if to.IsTerminal() {
		completedAt := task.UpdatedAt
		task.CompletedAt = &completedAt
	}
	return cloneA2ATask(task), nil
}
