package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// HybridSearchRepo provides hybrid search combining full-text ranking
// with title similarity using Reciprocal Rank Fusion (RRF).
type HybridSearchRepo struct {
	pool     *pgxpool.Pool
	hasTrgm  bool
	trgmOnce sync.Once
}

// NewHybridSearchRepo creates a new HybridSearchRepo.
func NewHybridSearchRepo(pool *pgxpool.Pool) *HybridSearchRepo {
	return &HybridSearchRepo{pool: pool}
}

// rrfK is the constant used in the RRF formula: score = 1/(k + rank).
// A value of 60 is standard in information retrieval literature.
const rrfK = 60

// PostEmbeddingDimensions is the dimensionality of posts.embedding. Keeping
// the value here gives request-time callers a cheap validation seam before a
// malformed provider response reaches PostgreSQL's vector cast.
const PostEmbeddingDimensions = 3072

const (
	semanticCandidateFloor = 100
	semanticCandidateCap   = 1000
)

// detectTrgm checks once whether pg_trgm's similarity() function is actually
// CALLABLE on this connection — not merely whether the extension is registered
// in pg_extension.
//
// The distinction is load-bearing: on some managed Postgres setups (observed in
// production) the pg_extension row for pg_trgm exists, but similarity() is not on
// the connection's search_path, so calling it raises
// "function similarity(text, text) does not exist" (SQLSTATE 42883). The old
// probe queried pg_extension, got a false positive, and every hybrid query then
// 500'd on the similarity() call. Probing the function directly lets hybrid
// search degrade to the LIKE fallback (titleMatchCTEFallback) instead — degraded
// fuzzy matching, but a working search.
func (r *HybridSearchRepo) detectTrgm(ctx context.Context) {
	r.trgmOnce.Do(func() {
		var ok bool
		// If similarity() is callable this returns true; if the function is
		// undefined the query errors and we leave hasTrgm false (fallback).
		err := r.pool.QueryRow(ctx, `SELECT similarity('a', 'a') >= 0`).Scan(&ok)
		if err == nil && ok {
			r.hasTrgm = true
		}
	})
}

// SearchFilters holds optional filter parameters for search queries.
type SearchFilters struct {
	Community  string // community slug
	AuthorType string // "human" or "agent"
	PostType   string // "text", "link", "synthesis", etc.
	Period     string // "day", "week", "month", "year"
}

// hybridSearchSQL returns the two-signal SQL query used when no query embedder
// is configured or when embedding generation fails.
func (r *HybridSearchRepo) hybridSearchSQL(filters SearchFilters) (string, []any) {
	return r.buildHybridSearchSQL(filters, false)
}

// semanticHybridSearchSQL returns the three-signal SQL query. The semantic
// candidate CTE deliberately uses the same halfvec expression as migration 89,
// allowing PostgreSQL to satisfy its cosine ORDER BY from the HNSW index.
func (r *HybridSearchRepo) semanticHybridSearchSQL(filters SearchFilters) (string, []any) {
	return r.buildHybridSearchSQL(filters, true)
}

// buildHybridSearchSQL returns the SQL query for hybrid search and any extra filter args.
// When pg_trgm is available, it uses similarity() for fuzzy title matching.
// Otherwise it falls back to LIKE-based title matching with length-ratio scoring.
//
// The RRF constant (rrfK) is embedded directly in the SQL as a literal to avoid
// pgx type-encoding issues with parameterized float casts ($N::float).
// Parameters: $1 = query, $2 = limit, $3 = offset. With semantic search,
// $4 = query embedding and $5 = ANN candidate limit. Filters follow those.
func (r *HybridSearchRepo) buildHybridSearchSQL(filters SearchFilters, includeSemantic bool) (string, []any) {
	return r.buildHybridSearchPageSQL(filters, includeSemantic, false)
}

// buildHybridSearchPageSQL optionally adds a keyset boundary while preserving
// the legacy SQL shape (and placeholder numbers) for offset callers.
func (r *HybridSearchRepo) buildHybridSearchPageSQL(filters SearchFilters, includeSemantic, includeCursor bool) (string, []any) {
	titleMatchCTE := r.titleMatchCTEWithTrgm()
	if !r.hasTrgm {
		titleMatchCTE = r.titleMatchCTEFallback()
	}

	semanticCTEs := ""
	combinedCTE := `combined AS (
		SELECT
			COALESCE(ts.id, tm.id) AS id,
			-- RRF: 1/(k+rank) for each signal, plus title-contains boost
			(CASE WHEN ts.text_pos IS NOT NULL THEN 1.0 / (_RRF_K_.0 + ts.text_pos) ELSE 0.0 END) +
			(CASE WHEN tm.title_pos IS NOT NULL THEN 1.0 / (_RRF_K_.0 + tm.title_pos) ELSE 0.0 END) +
			(COALESCE(tm.title_contains, 0.0) * 0.3) AS rrf_score
		FROM text_search ts
		FULL OUTER JOIN title_match tm ON ts.id = tm.id
	)`
	if includeSemantic {
		semanticCTEs = `,
	semantic_candidates AS (
		SELECT
			p.id,
			p.embedding::halfvec(3072) <=> $4::halfvec(3072) AS semantic_distance
		FROM posts p
		WHERE p.deleted_at IS NULL
		  AND p.quarantined = FALSE
		  AND p.embedding IS NOT NULL
		  _SEMANTIC_FILTER_CLAUSE_
		ORDER BY p.embedding::halfvec(3072) <=> $4::halfvec(3072)
		LIMIT $5
	),
	semantic_search AS (
		SELECT
			id,
			ROW_NUMBER() OVER (ORDER BY semantic_distance ASC) AS semantic_pos
		FROM semantic_candidates
	)`
		combinedCTE = `combined AS (
		SELECT
			COALESCE(ts.id, tm.id, ss.id) AS id,
			-- RRF: lexical relevance + fuzzy title relevance + semantic relevance.
			(CASE WHEN ts.text_pos IS NOT NULL THEN 1.0 / (_RRF_K_.0 + ts.text_pos) ELSE 0.0 END) +
			(CASE WHEN tm.title_pos IS NOT NULL THEN 1.0 / (_RRF_K_.0 + tm.title_pos) ELSE 0.0 END) +
			(CASE WHEN ss.semantic_pos IS NOT NULL THEN 1.0 / (_RRF_K_.0 + ss.semantic_pos) ELSE 0.0 END) +
			(COALESCE(tm.title_contains, 0.0) * 0.3) AS rrf_score
		FROM text_search ts
		FULL OUTER JOIN title_match tm ON ts.id = tm.id
		FULL OUTER JOIN semantic_search ss ON ss.id = COALESCE(ts.id, tm.id)
	)`
	}

	sql := `
	WITH
	text_search AS (
		SELECT p.id,
			ts_rank_cd(
				p.search_vector,
				plainto_tsquery('english', $1)
			) AS text_rank,
			ROW_NUMBER() OVER (
				ORDER BY ts_rank_cd(p.search_vector, plainto_tsquery('english', $1)) DESC
			) AS text_pos
		FROM posts p
		WHERE p.deleted_at IS NULL
		  AND p.quarantined = FALSE
		  AND p.search_vector @@ plainto_tsquery('english', $1)
	),
	` + titleMatchCTE + semanticCTEs + `,
	` + combinedCTE + `
	SELECT
		p.id, p.community_id, p.author_id, p.author_type,
		p.title, p.body, COALESCE(p.url, '') AS url,
		p.post_type, p.provenance_id, p.confidence_score,
		p.vote_score, p.comment_count, COALESCE(p.tags, '{}') AS tags, p.metadata, p.created_at, p.updated_at,
		p.deleted_at, p.superseded_by, p.is_retracted, p.retraction_notice,
		p.is_pinned, p.pinned_at,
		part.display_name, COALESCE(part.avatar_url, '') AS avatar_url,
		part.trust_score, part.reputation_score,
		part.type, part.is_verified,
		COALESCE(ai.model_provider, '') AS model_provider,
		COALESCE(ai.model_name, '') AS model_name,
		c.slug, c.name,
		prov.sources, prov.confidence_score AS prov_confidence, prov.generation_method,
		comb.rrf_score,
		COUNT(*) OVER() AS total_count
	FROM combined comb
	JOIN posts p ON p.id = comb.id
	JOIN participants part ON part.id = p.author_id
	LEFT JOIN agent_identities ai ON ai.participant_id = p.author_id
	JOIN communities c ON c.id = p.community_id
	LEFT JOIN provenances prov ON prov.id = p.provenance_id
	_FILTER_CLAUSE_
	ORDER BY comb.rrf_score DESC, p.id DESC
	LIMIT $2 OFFSET $3
	`

	// Build dynamic filter clauses
	var filterClauses []string
	var semanticFilterClauses []string
	var filterArgs []any
	paramIdx := 4 // $1=query, $2=limit, $3=offset already used
	if includeSemantic {
		paramIdx = 6 // $4=query embedding, $5=semantic candidate limit
	}
	if includeCursor {
		filterClauses = append(filterClauses, fmt.Sprintf(
			`(comb.rrf_score, p.id) < (SELECT rrf_score, id FROM combined WHERE id = $%d)`,
			paramIdx,
		))
		paramIdx++
	}

	if filters.Community != "" {
		filterClauses = append(filterClauses, fmt.Sprintf("c.slug = $%d", paramIdx))
		semanticFilterClauses = append(semanticFilterClauses, fmt.Sprintf(
			"p.community_id = (SELECT id FROM communities WHERE slug = $%d)", paramIdx,
		))
		filterArgs = append(filterArgs, filters.Community)
		paramIdx++
	}
	if filters.AuthorType != "" {
		filterClauses = append(filterClauses, fmt.Sprintf("p.author_type = $%d", paramIdx))
		semanticFilterClauses = append(semanticFilterClauses, fmt.Sprintf("p.author_type = $%d", paramIdx))
		filterArgs = append(filterArgs, filters.AuthorType)
		paramIdx++
	}
	if filters.PostType != "" {
		filterClauses = append(filterClauses, fmt.Sprintf("p.post_type = $%d", paramIdx))
		semanticFilterClauses = append(semanticFilterClauses, fmt.Sprintf("p.post_type = $%d", paramIdx))
		filterArgs = append(filterArgs, filters.PostType)
		paramIdx++
	}
	if filters.Period != "" {
		var interval string
		switch filters.Period {
		case "day":
			interval = "1 day"
		case "week":
			interval = "7 days"
		case "month":
			interval = "30 days"
		case "year":
			interval = "365 days"
		}
		if interval != "" {
			clause := fmt.Sprintf("p.created_at > NOW() - INTERVAL '%s'", interval)
			filterClauses = append(filterClauses, clause)
			semanticFilterClauses = append(semanticFilterClauses, clause)
		}
	}

	whereClause := ""
	semanticFilterClause := ""
	if len(filterClauses) > 0 {
		whereClause = "WHERE " + strings.Join(filterClauses, " AND ")
	}
	if len(semanticFilterClauses) > 0 {
		semanticFilterClause = "AND " + strings.Join(semanticFilterClauses, " AND ")
	}

	sql = strings.ReplaceAll(sql, "_SEMANTIC_FILTER_CLAUSE_", semanticFilterClause)
	// Replace the longer semantic token first: _FILTER_CLAUSE_ is a suffix of
	// _SEMANTIC_FILTER_CLAUSE_ and would otherwise leave invalid SQL behind.
	sql = strings.ReplaceAll(sql, "_FILTER_CLAUSE_", whereClause)
	rrfStr := strconv.Itoa(rrfK)
	return strings.ReplaceAll(sql, "_RRF_K_", rrfStr), filterArgs
}

// titleMatchCTEWithTrgm returns the title_match CTE using pg_trgm similarity().
func (r *HybridSearchRepo) titleMatchCTEWithTrgm() string {
	return `title_match AS (
		SELECT p.id,
			similarity(lower(p.title), lower($1)) AS title_sim,
			CASE WHEN lower(p.title) LIKE '%' || lower($1) || '%' THEN 1.0 ELSE 0.0 END AS title_contains,
			ROW_NUMBER() OVER (
				ORDER BY similarity(lower(p.title), lower($1)) DESC
			) AS title_pos
		FROM posts p
		WHERE p.deleted_at IS NULL
		  AND p.quarantined = FALSE
		  AND (
			similarity(lower(p.title), lower($1)) > 0.1
			OR lower(p.title) LIKE '%' || lower($1) || '%'
		  )
	)`
}

// titleMatchCTEFallback returns the title_match CTE using LIKE-based matching
// when pg_trgm is not available. Uses a length-ratio heuristic as a rough
// proxy for similarity scoring.
func (r *HybridSearchRepo) titleMatchCTEFallback() string {
	return `title_match AS (
		SELECT p.id,
			-- Approximate similarity: ratio of query length to title length (capped at 1.0)
			LEAST(1.0, length($1)::float / GREATEST(length(p.title), 1)::float) AS title_sim,
			CASE WHEN lower(p.title) LIKE '%' || lower($1) || '%' THEN 1.0 ELSE 0.0 END AS title_contains,
			ROW_NUMBER() OVER (
				ORDER BY
					CASE WHEN lower(p.title) LIKE '%' || lower($1) || '%' THEN 0 ELSE 1 END,
					length(p.title) ASC
			) AS title_pos
		FROM posts p
		WHERE p.deleted_at IS NULL
		  AND p.quarantined = FALSE
		  AND lower(p.title) LIKE '%' || lower($1) || '%'
	)`
}

// HybridSearch performs the resilient two-signal search used when a query
// embedding is unavailable. It combines full-text ts_rank_cd ranking
// (BM25-like) with title similarity via Reciprocal Rank Fusion.
//
// Parameters:
//   - query: the user's search string
//   - limit: max results to return
//   - offset: pagination offset
//
// Returns search results with relevance scores, total count, and any error.
func (r *HybridSearchRepo) HybridSearch(ctx context.Context, query string, limit, offset int, filters SearchFilters, cursor ...string) ([]models.SearchResult, int, error) {
	return r.runHybridSearch(ctx, query, nil, limit, offset, filters, cursor...)
}

// HybridSearchWithEmbedding adds cosine-nearest semantic candidates as the
// third RRF signal. Query embeddings must match posts.embedding's 3072-column
// contract; callers should fall back to HybridSearch when generation fails.
func (r *HybridSearchRepo) HybridSearchWithEmbedding(ctx context.Context, query string, queryEmbedding []float32, limit, offset int, filters SearchFilters, cursor ...string) ([]models.SearchResult, int, error) {
	if len(queryEmbedding) != PostEmbeddingDimensions {
		return nil, 0, fmt.Errorf("hybrid search: query embedding has %d dimensions, want %d", len(queryEmbedding), PostEmbeddingDimensions)
	}
	return r.runHybridSearch(ctx, query, queryEmbedding, limit, offset, filters, cursor...)
}

func (r *HybridSearchRepo) runHybridSearch(ctx context.Context, query string, queryEmbedding []float32, limit, offset int, filters SearchFilters, cursor ...string) ([]models.SearchResult, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	cursorID := ""
	if len(cursor) > 0 {
		cursorID = cursor[0]
	}
	if cursorID != "" {
		offset = 0
	}

	// Detect pg_trgm availability once on first call.
	r.detectTrgm(ctx)

	searchSQL, filterArgs := r.buildHybridSearchPageSQL(filters, false, cursorID != "")
	args := []any{query, limit, offset}
	if len(queryEmbedding) > 0 {
		searchSQL, filterArgs = r.buildHybridSearchPageSQL(filters, true, cursorID != "")
		args = []any{
			query,
			limit,
			offset,
			vectorLiteral(queryEmbedding),
			semanticCandidateLimit(limit, offset),
		}
	}
	if cursorID != "" {
		args = append(args, cursorID)
	}
	args = append(args, filterArgs...)

	rows, err := r.pool.Query(ctx, searchSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("hybrid search: %w", err)
	}
	defer rows.Close()

	var results []models.SearchResult
	var total int
	for rows.Next() {
		sr, rowTotal, err := scanSearchResult(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan search result: %w", err)
		}
		total = rowTotal
		results = append(results, sr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("hybrid search rows: %w", err)
	}

	if results == nil {
		results = []models.SearchResult{}
	}

	// Normalize relevance scores to 0.0-1.0 range
	normalizeScores(results)

	return results, total, nil
}

func semanticCandidateLimit(limit, offset int) int {
	candidates := limit + offset
	if candidates < semanticCandidateFloor {
		return semanticCandidateFloor
	}
	if candidates > semanticCandidateCap {
		return semanticCandidateCap
	}
	return candidates
}

// vectorLiteral uses pgvector's text input format. Dimensionality is validated
// by HybridSearchWithEmbedding before this formatter is reached.
func vectorLiteral(vec []float32) string {
	var b strings.Builder
	b.Grow(len(vec) * 12)
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// scanSearchResult scans a row from the hybrid search query into a SearchResult.
// The row has the standard PostWithAuthor columns, then rrf_score, then total_count.
func scanSearchResult(row interface {
	Scan(dest ...any) error
}) (models.SearchResult, int, error) {
	var sr models.SearchResult
	var communitySlug, communityName string
	var modelProvider, modelName string
	var provSources []string
	var provConfidence *float64
	var provMethod *string
	var metadataBytes []byte
	var totalCount int

	err := row.Scan(
		&sr.ID, &sr.CommunityID, &sr.AuthorID, &sr.AuthorType,
		&sr.Title, &sr.Body, &sr.URL,
		&sr.PostType, &sr.ProvenanceID, &sr.ConfidenceScore,
		&sr.VoteScore, &sr.CommentCount, &sr.Tags, &metadataBytes, &sr.CreatedAt, &sr.UpdatedAt,
		&sr.DeletedAt, &sr.SupersededBy, &sr.IsRetracted, &sr.RetractionNotice,
		&sr.IsPinned, &sr.PinnedAt,
		&sr.Author.DisplayName, &sr.Author.AvatarURL,
		&sr.Author.TrustScore, &sr.Author.ReputationScore,
		&sr.Author.Type, &sr.Author.IsVerified,
		&modelProvider, &modelName,
		&communitySlug, &communityName,
		&provSources, &provConfidence, &provMethod,
		&sr.RelevanceScore,
		&totalCount,
	)
	if err != nil {
		return sr, 0, err
	}

	if len(metadataBytes) > 0 {
		sr.Metadata = make(map[string]any)
		_ = json.Unmarshal(metadataBytes, &sr.Metadata)
	}

	sr.Author.ID = sr.AuthorID
	sr.Author.ModelProvider = modelProvider
	sr.Author.ModelName = modelName
	sr.Community = &models.Community{
		ID:   sr.CommunityID,
		Slug: communitySlug,
		Name: communityName,
	}

	if sr.ProvenanceID != nil {
		confidence := 0.0
		if provConfidence != nil {
			confidence = *provConfidence
		}
		method := models.GenerationMethod("")
		if provMethod != nil {
			method = models.GenerationMethod(*provMethod)
		}
		sources := provSources
		if sources == nil {
			sources = []string{}
		}
		sr.Provenance = &models.Provenance{
			ID:               *sr.ProvenanceID,
			Sources:          sources,
			ConfidenceScore:  confidence,
			GenerationMethod: method,
		}
	}

	return sr, totalCount, nil
}

// normalizeScores rescales relevance scores to 0.0-1.0 range.
// The top result gets 1.0, and others are scaled proportionally.
func normalizeScores(results []models.SearchResult) {
	if len(results) == 0 {
		return
	}

	maxScore := results[0].RelevanceScore
	if maxScore <= 0 {
		return
	}

	for i := range results {
		results[i].RelevanceScore = math.Round(results[i].RelevanceScore/maxScore*100) / 100
	}
}
