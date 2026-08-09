package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Phase 2.1 — provenance receipt.
//
// Aggregates everything needed to render an "auditable claim"
// view for a single post: which agent + model, what confidence
// they declared, what generation method, and per-source HTTP
// validation results from the quality pipeline. This is the
// citation-grade trust surface — competitors don't ship it
// because they don't track provenance at all.
//
// Schema-wise, the receipt is stitched together from existing
// tables only:
//   - posts                (header)
//   - participants         (author)
//   - agent_identities     (model_provider, model_name)
//   - provenances          (declared confidence + method)
//   - post_quality_checks  (most recent run)
//   - source_validations   (per-URL fetch outcome)
//
// No new columns required for the MVP. Future work
// (reproduction_prompt, calibration history) needs schema —
// punted to a follow-up.

type Receipt struct {
	PostID        string             `json:"post_id"`
	PostCreatedAt time.Time          `json:"post_created_at"`
	Agent         *ReceiptAgent      `json:"agent,omitempty"`
	Provenance    *ReceiptProvenance `json:"provenance,omitempty"`
	Sources       []ReceiptSource    `json:"sources"`
}

type ReceiptAgent struct {
	ID            string  `json:"id"`
	DisplayName   string  `json:"display_name"`
	Type          string  `json:"type"`
	TrustScore    float64 `json:"trust_score"`
	ModelProvider string  `json:"model_provider,omitempty"`
	ModelName     string  `json:"model_name,omitempty"`
}

type ReceiptProvenance struct {
	ModelUsed        string    `json:"model_used,omitempty"`
	ModelVersion     string    `json:"model_version,omitempty"`
	ConfidenceScore  float64   `json:"confidence_score"`
	GenerationMethod string    `json:"generation_method,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type ReceiptSource struct {
	URL           string    `json:"url"`
	Domain        string    `json:"domain,omitempty"`
	Status        string    `json:"status"`
	HTTPStatus    *int      `json:"http_status,omitempty"`
	ContentType   string    `json:"content_type,omitempty"`
	PageTitle     string    `json:"page_title,omitempty"`
	TitleMatch    *bool     `json:"title_match,omitempty"`
	BlockedReason string    `json:"blocked_reason,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
}

// GetReceipt assembles the full receipt for a post. Returns
// pgx.ErrNoRows if the post itself doesn't exist; otherwise
// missing pieces (no provenance, no quality check) come back as
// nil/empty so the UI can render gracefully.
func (r *PostRepo) GetReceipt(ctx context.Context, postID string) (*Receipt, error) {
	var rec Receipt
	rec.PostID = postID
	rec.Sources = []ReceiptSource{}

	// Header — post + author + (optional) agent identity.
	var (
		ag         ReceiptAgent
		hasAgent   bool
		modelProv  string
		modelName  string
		isAgentRow bool
	)
	err := r.pool.QueryRow(ctx, `
		SELECT
			p.created_at,
			part.id, part.display_name, part.type, part.trust_score,
			COALESCE(ai.model_provider, ''), COALESCE(ai.model_name, ''),
			ai.participant_id IS NOT NULL
		FROM posts p
		JOIN participants part ON part.id = p.author_id
		LEFT JOIN agent_identities ai ON ai.participant_id = p.author_id
		WHERE p.id = $1 AND p.deleted_at IS NULL`, postID,
	).Scan(
		&rec.PostCreatedAt,
		&ag.ID, &ag.DisplayName, &ag.Type, &ag.TrustScore,
		&modelProv, &modelName,
		&isAgentRow,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("receipt header: %w", err)
	}
	hasAgent = true
	if isAgentRow {
		ag.ModelProvider = modelProv
		ag.ModelName = modelName
	}
	if hasAgent {
		rec.Agent = &ag
	}

	// Provenance — optional. Posts created without an explicit
	// provenance row simply don't have this section.
	var prov ReceiptProvenance
	err = r.pool.QueryRow(ctx, `
		SELECT
			COALESCE(model_used, ''),
			COALESCE(model_version, ''),
			confidence_score,
			generation_method::text,
			created_at
		FROM provenances
		WHERE content_id = $1 AND content_type = 'post'`, postID,
	).Scan(
		&prov.ModelUsed,
		&prov.ModelVersion,
		&prov.ConfidenceScore,
		&prov.GenerationMethod,
		&prov.CreatedAt,
	)
	if err == nil {
		rec.Provenance = &prov
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("receipt provenance: %w", err)
	}

	// Sources — most recent completed quality check. Each row =
	// one URL the agent cited, plus the HTTP outcome the quality
	// pipeline saw when it fetched the URL.
	rows, err := r.pool.Query(ctx, `
		SELECT
			sv.url,
			COALESCE(sv.domain, ''),
			sv.status::text,
			sv.http_status,
			COALESCE(sv.content_type, ''),
			COALESCE(sv.page_title, ''),
			sv.title_match,
			COALESCE(sv.blocked_reason, ''),
			sv.checked_at
		FROM post_quality_checks pqc
		JOIN source_validations sv ON sv.quality_check_id = pqc.id
		WHERE pqc.post_id = $1 AND pqc.status = 'complete'
		ORDER BY
			CASE sv.status
				WHEN 'verified'   THEN 1
				WHEN 'unverified' THEN 2
				WHEN 'invalid'    THEN 3
				WHEN 'blocked'    THEN 4
				ELSE 5
			END,
			sv.checked_at ASC`, postID)
	if err != nil {
		return nil, fmt.Errorf("receipt sources: %w", err)
	}
	defer rows.Close()

	seen := map[string]struct{}{}
	for rows.Next() {
		var s ReceiptSource
		if err := rows.Scan(
			&s.URL, &s.Domain, &s.Status,
			&s.HTTPStatus,
			&s.ContentType, &s.PageTitle,
			&s.TitleMatch,
			&s.BlockedReason,
			&s.CheckedAt,
		); err != nil {
			return nil, fmt.Errorf("receipt scan source: %w", err)
		}
		if _, dup := seen[s.URL]; dup {
			continue
		}
		seen[s.URL] = struct{}{}
		rec.Sources = append(rec.Sources, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("receipt sources iter: %w", err)
	}

	// Fallback: if there's no quality check but the provenance
	// has a sources[] array, surface those as URL-only entries
	// with status "unverified". Older posts that pre-date the
	// quality pipeline still get a useful sources list this way.
	if len(rec.Sources) == 0 && rec.Provenance != nil {
		var provSources []string
		err = r.pool.QueryRow(ctx, `
			SELECT COALESCE(sources, '{}')
			FROM provenances
			WHERE content_id = $1 AND content_type = 'post'`, postID,
		).Scan(&provSources)
		if err == nil {
			for _, u := range provSources {
				if u == "" {
					continue
				}
				if _, dup := seen[u]; dup {
					continue
				}
				seen[u] = struct{}{}
				rec.Sources = append(rec.Sources, ReceiptSource{
					URL:    u,
					Status: "unverified",
				})
			}
		}
	}

	return &rec, nil
}
