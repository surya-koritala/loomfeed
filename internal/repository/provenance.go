package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// ProvenanceRepo handles database operations for provenances.
type ProvenanceRepo struct {
	pool *pgxpool.Pool
}

// NewProvenanceRepo creates a new ProvenanceRepo.
func NewProvenanceRepo(pool *pgxpool.Pool) *ProvenanceRepo {
	return &ProvenanceRepo{pool: pool}
}

// Create inserts a new provenance record. Defaults generation_method to "original" if empty.
func (r *ProvenanceRepo) Create(ctx context.Context, p *models.Provenance) (*models.Provenance, error) {
	if p.GenerationMethod == "" {
		p.GenerationMethod = models.MethodOriginal
	}

	var result models.Provenance
	err := r.pool.QueryRow(ctx, `
		INSERT INTO provenances
		  (content_id, content_type, author_id, sources,
		   model_used, model_version, prompt_hash,
		   confidence_score, generation_method)
		VALUES ($1, $2, $3, $4,
		        NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
		        $8, $9)
		RETURNING
		  id, content_id, content_type, author_id,
		  COALESCE(sources, '{}') AS sources,
		  COALESCE(model_used, '') AS model_used,
		  COALESCE(model_version, '') AS model_version,
		  COALESCE(prompt_hash, '') AS prompt_hash,
		  confidence_score, generation_method, created_at`,
		p.ContentID,
		p.ContentType,
		p.AuthorID,
		p.Sources,
		p.ModelUsed,
		p.ModelVersion,
		p.PromptHash,
		p.ConfidenceScore,
		p.GenerationMethod,
	).Scan(
		&result.ID, &result.ContentID, &result.ContentType, &result.AuthorID,
		&result.Sources,
		&result.ModelUsed, &result.ModelVersion, &result.PromptHash,
		&result.ConfidenceScore, &result.GenerationMethod, &result.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert provenance: %w", err)
	}
	return &result, nil
}

// GetByContentID returns the provenance record for a given content_id and content_type.
func (r *ProvenanceRepo) GetByContentID(ctx context.Context, contentID string, contentType models.TargetType) (*models.Provenance, error) {
	var result models.Provenance
	err := r.pool.QueryRow(ctx, `
		SELECT
		  id, content_id, content_type, author_id,
		  COALESCE(sources, '{}') AS sources,
		  COALESCE(model_used, '') AS model_used,
		  COALESCE(model_version, '') AS model_version,
		  COALESCE(prompt_hash, '') AS prompt_hash,
		  confidence_score, generation_method, created_at
		FROM provenances
		WHERE content_id = $1 AND content_type = $2`,
		contentID,
		contentType,
	).Scan(
		&result.ID, &result.ContentID, &result.ContentType, &result.AuthorID,
		&result.Sources,
		&result.ModelUsed, &result.ModelVersion, &result.PromptHash,
		&result.ConfidenceScore, &result.GenerationMethod, &result.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get provenance by content_id: %w", err)
	}
	return &result, nil
}
