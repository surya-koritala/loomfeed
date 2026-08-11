package repository_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestA2ATaskRepoPersistsLifecycleScopesOwnerAndDeduplicates(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"a2a_tasks", "api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	ownerA := createTestOwner(t, participants, ctx, "a2a-task-owner-a")
	ownerB := createTestOwner(t, participants, ctx, "a2a-task-owner-b")
	tasks := repository.NewA2ATaskRepo(pool)

	request := json.RawMessage(`{"skill":"search","input":{"query":"persistent"}}`)
	createdTask, created, err := tasks.CreateTask(ctx, &models.A2ATask{
		TaskID: ownerA.ID + "-task", ParticipantID: ownerA.ID,
		Skill: "search", Request: request, State: models.A2ATaskSubmitted,
	})
	if err != nil || !created || createdTask.State != models.A2ATaskSubmitted {
		t.Fatalf("create task: task=%#v created=%v err=%v", createdTask, created, err)
	}

	duplicate, created, err := tasks.CreateTask(ctx, &models.A2ATask{
		TaskID: createdTask.TaskID, ParticipantID: ownerA.ID,
		Skill: "vote", Request: json.RawMessage(`{"different":true}`), State: models.A2ATaskSubmitted,
	})
	if err != nil || created || duplicate.Skill != "search" {
		t.Fatalf("duplicate must return original task without replacement: task=%#v created=%v err=%v", duplicate, created, err)
	}

	if _, found, err := tasks.GetTask(ctx, ownerB.ID, createdTask.TaskID); err != nil || found {
		t.Fatalf("task must be owner-scoped: found=%v err=%v", found, err)
	}
	working, err := tasks.TransitionTask(ctx, ownerA.ID, createdTask.TaskID,
		models.A2ATaskSubmitted, models.A2ATaskWorking, "executing search", nil)
	if err != nil || working.State != models.A2ATaskWorking {
		t.Fatalf("mark working: task=%#v err=%v", working, err)
	}
	artifacts := json.RawMessage(`[{"parts":[{"text":"{\"results\":[]}"}]}]`)
	completed, err := tasks.TransitionTask(ctx, ownerA.ID, createdTask.TaskID,
		models.A2ATaskWorking, models.A2ATaskCompleted, "", artifacts)
	if err != nil || completed.State != models.A2ATaskCompleted || completed.CompletedAt == nil || !equalJSON(completed.Artifacts, artifacts) {
		t.Fatalf("complete task: task=%#v err=%v", completed, err)
	}
	if _, err := tasks.TransitionTask(ctx, ownerA.ID, createdTask.TaskID,
		models.A2ATaskCompleted, models.A2ATaskWorking, "invalid regression", nil); err == nil {
		t.Fatal("terminal task must reject an invalid state regression")
	}

	reloaded, found, err := tasks.GetTask(ctx, ownerA.ID, createdTask.TaskID)
	if err != nil || !found || reloaded.State != models.A2ATaskCompleted || !equalJSON(reloaded.Artifacts, artifacts) {
		t.Fatalf("reload completed task: task=%#v found=%v err=%v", reloaded, found, err)
	}
}

func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}
