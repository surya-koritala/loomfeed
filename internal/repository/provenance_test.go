package repository_test

import (
	"context"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestProvenanceRepo_CreateAndGet(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "provenances", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	provRepo := repository.NewProvenanceRepo(pool)
	ctx := context.Background()

	// Create a human author (provenances.author_id references participants)
	author := createTestOwner(t, pRepo, ctx, "prov-author")

	// Use an arbitrary UUID as content_id (no FK constraint on provenances.content_id)
	contentID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

	prov := &models.Provenance{
		ContentID:       contentID,
		ContentType:     models.TargetPost,
		AuthorID:        author.ID,
		Sources:         []string{"https://example.com/source1", "https://example.com/source2"},
		ModelUsed:       "gpt-4",
		ModelVersion:    "2024-01",
		PromptHash:      "abc123hash",
		ConfidenceScore: 0.92,
		// GenerationMethod left empty to test default
	}

	created, err := provRepo.Create(ctx, prov)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.GenerationMethod != models.MethodOriginal {
		t.Errorf("expected default generation_method 'original', got %q", created.GenerationMethod)
	}
	if created.ConfidenceScore != 0.92 {
		t.Errorf("expected confidence_score 0.92, got %f", created.ConfidenceScore)
	}
	if len(created.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(created.Sources))
	}
	if created.Sources[0] != "https://example.com/source1" {
		t.Errorf("expected first source 'https://example.com/source1', got %q", created.Sources[0])
	}
	if created.ModelUsed != "gpt-4" {
		t.Errorf("expected model_used 'gpt-4', got %q", created.ModelUsed)
	}

	// Get by content ID
	got, err := provRepo.GetByContentID(ctx, contentID, models.TargetPost)
	if err != nil {
		t.Fatalf("GetByContentID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetByContentID returned ID %q, want %q", got.ID, created.ID)
	}
	if got.ContentID != contentID {
		t.Errorf("GetByContentID returned ContentID %q, want %q", got.ContentID, contentID)
	}
	if got.ContentType != models.TargetPost {
		t.Errorf("GetByContentID returned ContentType %q, want %q", got.ContentType, models.TargetPost)
	}
	if len(got.Sources) != 2 {
		t.Errorf("GetByContentID returned %d sources, want 2", len(got.Sources))
	}
	if got.ConfidenceScore != 0.92 {
		t.Errorf("GetByContentID returned confidence_score %f, want 0.92", got.ConfidenceScore)
	}
}
