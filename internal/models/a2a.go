package models

import (
	"encoding/json"
	"time"
)

// A2ATaskState is the persisted lifecycle exposed by tasks/get. Loomfeed's
// gateway is synchronous, so it supports the submitted, working, completed,
// and failed states without claiming cancellation or input-required flows.
type A2ATaskState string

const (
	A2ATaskSubmitted A2ATaskState = "submitted"
	A2ATaskWorking   A2ATaskState = "working"
	A2ATaskCompleted A2ATaskState = "completed"
	A2ATaskFailed    A2ATaskState = "failed"
)

func (s A2ATaskState) IsTerminal() bool {
	return s == A2ATaskCompleted || s == A2ATaskFailed
}

// A2ATask is the durable record behind the A2A task response. TaskID is
// caller-provided and unique within one authenticated participant's scope.
type A2ATask struct {
	TaskID        string          `json:"id"`
	ParticipantID string          `json:"-"`
	Skill         string          `json:"skill"`
	Request       json.RawMessage `json:"-"`
	State         A2ATaskState    `json:"state"`
	StatusMessage string          `json:"status_message,omitempty"`
	Artifacts     json.RawMessage `json:"artifacts,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}
