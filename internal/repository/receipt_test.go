package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// Phase 2.1 — receipt builder. Tests cover the three real-world
// branches: (1) bare post with neither provenance nor a quality
// check, (2) post with provenance.sources but no quality run, and
// (3) full pipeline with completed quality_check + source_validations.

func TestPostRepo_GetReceipt_BarePost(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "post_quality_checks", "provenances", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "rcpt-bare-1")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "rcpt-bare-1")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Bare post")

	rec, err := postRepo.GetReceipt(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetReceipt: %v", err)
	}
	if rec == nil {
		t.Fatal("expected receipt, got nil")
	}
	if rec.PostID != post.ID {
		t.Errorf("PostID = %q, want %q", rec.PostID, post.ID)
	}
	if rec.Agent == nil {
		t.Fatal("expected Agent populated for human author, got nil")
	}
	if rec.Agent.DisplayName != owner.DisplayName {
		t.Errorf("Agent.DisplayName = %q, want %q", rec.Agent.DisplayName, owner.DisplayName)
	}
	if rec.Agent.ModelProvider != "" || rec.Agent.ModelName != "" {
		t.Errorf("expected no model fields for human author, got %+v", rec.Agent)
	}
	if rec.Provenance != nil {
		t.Errorf("expected no Provenance, got %+v", rec.Provenance)
	}
	if len(rec.Sources) != 0 {
		t.Errorf("expected no Sources, got %d", len(rec.Sources))
	}
}

func TestPostRepo_GetReceipt_ProvenanceFallbackSources(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "post_quality_checks", "provenances", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	provRepo := repository.NewProvenanceRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "rcpt-prov-1")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "rcpt-prov-1")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Prov post")

	_, err := provRepo.Create(ctx, &models.Provenance{
		ContentID:       post.ID,
		ContentType:     models.TargetPost,
		AuthorID:        owner.ID,
		Sources:         []string{"https://example.com/a", "https://example.com/b"},
		ModelUsed:       "gpt-4",
		ConfidenceScore: 0.75,
	})
	if err != nil {
		t.Fatalf("provRepo.Create: %v", err)
	}

	rec, err := postRepo.GetReceipt(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetReceipt: %v", err)
	}
	if rec.Provenance == nil {
		t.Fatal("expected Provenance, got nil")
	}
	if rec.Provenance.ConfidenceScore != 0.75 {
		t.Errorf("ConfidenceScore = %v, want 0.75", rec.Provenance.ConfidenceScore)
	}
	if rec.Provenance.ModelUsed != "gpt-4" {
		t.Errorf("ModelUsed = %q, want gpt-4", rec.Provenance.ModelUsed)
	}
	if len(rec.Sources) != 2 {
		t.Fatalf("expected 2 fallback sources, got %d", len(rec.Sources))
	}
	for _, s := range rec.Sources {
		if s.Status != "unverified" {
			t.Errorf("expected fallback status 'unverified', got %q for %q", s.Status, s.URL)
		}
	}
}

func TestPostRepo_GetReceipt_FullQualityCheck(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "source_validations", "post_quality_checks", "provenances", "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	provRepo := repository.NewProvenanceRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "rcpt-full-1")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "rcpt-full-1")
	post := createTestPost(t, postRepo, ctx, community.ID, owner.ID, "Full pipeline post")

	_, err := provRepo.Create(ctx, &models.Provenance{
		ContentID:        post.ID,
		ContentType:      models.TargetPost,
		AuthorID:         owner.ID,
		Sources:          []string{"https://example.com/a"},
		ModelUsed:        "claude-sonnet-4",
		ConfidenceScore:  0.88,
		GenerationMethod: models.MethodSynthesis,
	})
	if err != nil {
		t.Fatalf("provRepo.Create: %v", err)
	}

	// Insert a completed quality check + two validations: one
	// verified and one blocked. Bypassing the quality package
	// so the test stays scoped to the receipt SQL.
	var checkID string
	err = pool.QueryRow(ctx, `
		INSERT INTO post_quality_checks (post_id, status, quality_score)
		VALUES ($1, 'complete', 0.9)
		RETURNING id`, post.ID,
	).Scan(&checkID)
	if err != nil {
		t.Fatalf("insert quality check: %v", err)
	}

	for i, sv := range []struct {
		url    string
		domain string
		status string
		http   int
		title  string
		match  bool
		blockd string
	}{
		{"https://example.com/a", "example.com", "verified", 200, "Article A", true, ""},
		{"https://blocked.example/b", "blocked.example", "blocked", 0, "", false, "domain blocklist"},
	} {
		_, err := pool.Exec(ctx, `
			INSERT INTO source_validations
			    (quality_check_id, url, domain, status, http_status, page_title, title_match, blocked_reason)
			VALUES ($1, $2, $3, $4::source_validation_status, $5, $6, $7, $8)`,
			checkID, sv.url, sv.domain, sv.status, sv.http, sv.title, sv.match, sv.blockd,
		)
		if err != nil {
			t.Fatalf("insert source_validation %d: %v", i, err)
		}
	}

	rec, err := postRepo.GetReceipt(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetReceipt: %v", err)
	}
	if rec.Provenance == nil || rec.Provenance.GenerationMethod != string(models.MethodSynthesis) {
		t.Errorf("expected synthesis provenance, got %+v", rec.Provenance)
	}
	if len(rec.Sources) != 2 {
		t.Fatalf("expected 2 sources from quality check, got %d", len(rec.Sources))
	}
	// Verified must come first per status-priority ORDER BY.
	if rec.Sources[0].Status != "verified" {
		t.Errorf("expected first source verified, got %q", rec.Sources[0].Status)
	}
	if rec.Sources[0].HTTPStatus == nil || *rec.Sources[0].HTTPStatus != 200 {
		t.Errorf("expected http_status 200, got %v", rec.Sources[0].HTTPStatus)
	}
	if rec.Sources[1].Status != "blocked" {
		t.Errorf("expected second source blocked, got %q", rec.Sources[1].Status)
	}
	if rec.Sources[1].BlockedReason != "domain blocklist" {
		t.Errorf("expected blocked_reason populated, got %q", rec.Sources[1].BlockedReason)
	}
}

func TestPostRepo_GetReceipt_NotFound(t *testing.T) {
	pool := database.TestPool(t)
	postRepo := repository.NewPostRepo(pool)

	_, err := postRepo.GetReceipt(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("expected pgx.ErrNoRows for missing post, got %v", err)
	}
}
