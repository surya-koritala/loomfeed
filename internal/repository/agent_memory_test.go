package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

func TestAgentMemoryRepo_SetAndGet(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "memory-setget")

	// Set a key
	value := json.RawMessage(`{"preference":"dark_mode","version":2}`)
	if err := memRepo.Set(ctx, owner.ID, "settings", value); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get the key
	entry, err := memRepo.Get(ctx, owner.ID, "settings")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Key != "settings" {
		t.Errorf("expected key 'settings', got %q", entry.Key)
	}
	if entry.AgentID != owner.ID {
		t.Errorf("expected agent_id %q, got %q", owner.ID, entry.AgentID)
	}
	var got map[string]any
	if err := json.Unmarshal(entry.Value, &got); err != nil {
		t.Fatalf("unmarshal value: %v", err)
	}
	if got["preference"] != "dark_mode" || got["version"] != float64(2) {
		t.Errorf("unexpected value: %s", string(entry.Value))
	}
}

func TestAgentMemoryRepo_SetUpsert(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "memory-upsert")

	// Set initial value
	if err := memRepo.Set(ctx, owner.ID, "counter", json.RawMessage(`1`)); err != nil {
		t.Fatalf("Set initial: %v", err)
	}

	// Update value
	if err := memRepo.Set(ctx, owner.ID, "counter", json.RawMessage(`42`)); err != nil {
		t.Fatalf("Set update: %v", err)
	}

	// Verify updated
	entry, err := memRepo.Get(ctx, owner.ID, "counter")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(entry.Value) != "42" {
		t.Errorf("expected value '42', got %q", string(entry.Value))
	}

	// Verify only one entry exists
	entries, err := memRepo.List(ctx, owner.ID, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after upsert, got %d", len(entries))
	}
}

func TestAgentMemoryRepo_GetNotFound(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "memory-notfound")

	entry, err := memRepo.Get(ctx, owner.ID, "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry != nil {
		t.Error("expected nil for nonexistent key")
	}
}

func TestAgentMemoryRepo_ListWithPrefix(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "memory-prefix")

	// Set multiple keys with different prefixes
	_ = memRepo.Set(ctx, owner.ID, "config.theme", json.RawMessage(`"dark"`))
	_ = memRepo.Set(ctx, owner.ID, "config.language", json.RawMessage(`"en"`))
	_ = memRepo.Set(ctx, owner.ID, "cache.results", json.RawMessage(`[]`))

	// List all
	all, err := memRepo.List(ctx, owner.ID, "")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 entries, got %d", len(all))
	}

	// List with prefix
	configEntries, err := memRepo.List(ctx, owner.ID, "config.")
	if err != nil {
		t.Fatalf("List prefix: %v", err)
	}
	if len(configEntries) != 2 {
		t.Errorf("expected 2 config entries, got %d", len(configEntries))
	}

	cacheEntries, err := memRepo.List(ctx, owner.ID, "cache.")
	if err != nil {
		t.Fatalf("List prefix: %v", err)
	}
	if len(cacheEntries) != 1 {
		t.Errorf("expected 1 cache entry, got %d", len(cacheEntries))
	}
}

func TestAgentMemoryRepo_Delete(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "memory-delete")

	_ = memRepo.Set(ctx, owner.ID, "temp", json.RawMessage(`"value"`))

	if err := memRepo.Delete(ctx, owner.ID, "temp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	entry, err := memRepo.Get(ctx, owner.ID, "temp")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if entry != nil {
		t.Error("expected nil after delete")
	}
}

func TestAgentMemoryRepo_DeleteNotFound(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "memory-delnotfound")

	err := memRepo.Delete(ctx, owner.ID, "nonexistent")
	if err == nil {
		t.Error("expected error when deleting nonexistent key")
	}
}

func TestAgentMemoryRepo_DeleteAll(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "memory-deleteall")

	_ = memRepo.Set(ctx, owner.ID, "key1", json.RawMessage(`"a"`))
	_ = memRepo.Set(ctx, owner.ID, "key2", json.RawMessage(`"b"`))
	_ = memRepo.Set(ctx, owner.ID, "key3", json.RawMessage(`"c"`))

	if err := memRepo.DeleteAll(ctx, owner.ID); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}

	entries, err := memRepo.List(ctx, owner.ID, "")
	if err != nil {
		t.Fatalf("List after DeleteAll: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after DeleteAll, got %d", len(entries))
	}
}

func TestAgentMemoryRepo_Count(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "memory-count")

	count, err := memRepo.Count(ctx, owner.ID)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 count, got %d", count)
	}

	_ = memRepo.Set(ctx, owner.ID, "k1", json.RawMessage(`1`))
	_ = memRepo.Set(ctx, owner.ID, "k2", json.RawMessage(`2`))

	count, err = memRepo.Count(ctx, owner.ID)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 count, got %d", count)
	}
}

func TestAgentMemoryRepo_IsolationBetweenAgents(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "agent_memory", "api_keys", "agent_identities", "human_users", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	memRepo := repository.NewAgentMemoryRepo(pool)
	ctx := context.Background()

	agent1 := createTestOwner(t, pRepo, ctx, "memory-iso-1")
	agent2 := createTestOwner(t, pRepo, ctx, "memory-iso-2")

	_ = memRepo.Set(ctx, agent1.ID, "shared_key", json.RawMessage(`"agent1_value"`))
	_ = memRepo.Set(ctx, agent2.ID, "shared_key", json.RawMessage(`"agent2_value"`))

	entry1, _ := memRepo.Get(ctx, agent1.ID, "shared_key")
	entry2, _ := memRepo.Get(ctx, agent2.ID, "shared_key")

	if string(entry1.Value) != `"agent1_value"` {
		t.Errorf("agent1 got wrong value: %s", string(entry1.Value))
	}
	if string(entry2.Value) != `"agent2_value"` {
		t.Errorf("agent2 got wrong value: %s", string(entry2.Value))
	}

	// Deleting agent1's key should not affect agent2
	_ = memRepo.Delete(ctx, agent1.ID, "shared_key")
	entry2After, _ := memRepo.Get(ctx, agent2.ID, "shared_key")
	if entry2After == nil {
		t.Error("agent2's key was deleted when agent1's was removed")
	}
}
