package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
)

var ErrHumanVerificationRequired = errors.New("post requires human verification before publication")

// PostRepo handles database operations for posts.
type PostRepo struct {
	pool *pgxpool.Pool
}

// NewPostRepo creates a new PostRepo.
func NewPostRepo(pool *pgxpool.Pool) *PostRepo {
	return &PostRepo{pool: pool}
}

// totalLivePostCount returns the number of live posts. Tries the
// denormalized platform_stats snapshot first (sub-ms primary-key
// lookup, maintained by PlatformStatsWorker). If that's zero —
// which happens in tests, on fresh installs before the worker has
// run, and in any environment without the worker running — falls
// back to a real COUNT(*).
//
// The fallback is bounded by the actual row count, so it's fast in
// the cases where it triggers (small datasets) and never triggers
// in production where the worker keeps the snapshot non-zero.
func (r *PostRepo) totalLivePostCount(ctx context.Context) int {
	var n int
	_ = r.pool.QueryRow(ctx,
		`SELECT total_posts FROM platform_stats WHERE id = 1`,
	).Scan(&n)
	if n > 0 {
		return n
	}
	_ = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM posts WHERE deleted_at IS NULL`,
	).Scan(&n)
	return n
}

// recencyWindowFor returns an extra WHERE-clause fragment that caps
// the candidate set for sort modes whose ranking math is dominated
// by a time term. Without this, hot/rising sorts seq-scan the full
// posts table (~46k rows in prod) and compute their score expression
// for every row before the LIMIT 20 — measured at 3.5s on prod.
//
// 30 days is conservative for hot (the time term's ~12.5h half-life
// means anything older than 7 days has effectively zero chance of
// ranking against fresh content) but generous enough that a
// medium-cycle resurgence still reaches the feed. The
// idx_posts_live partial index covers this filter so it's an
// index-only scan.
//
// Empty string for "new" (already index-friendly) and "top" (which
// genuinely should consider all posts).
func recencyWindowFor(sort string) string {
	switch sort {
	case "hot":
		return `p.created_at > NOW() - INTERVAL '30 days'`
	case "rising":
		return `p.created_at > NOW() - INTERVAL '48 hours'`
	}
	return ""
}

// orderByClause returns the ORDER BY expression for the given sort mode.
func orderByClause(sort string) string {
	switch sort {
	case "new":
		return "p.created_at DESC, p.id DESC"
	case "top":
		return "p.vote_score DESC, p.created_at DESC, p.id DESC"
	case "rising":
		return "(p.vote_score::float / GREATEST(EXTRACT(EPOCH FROM (NOW() - p.created_at)) / 3600, 1)) DESC, p.id DESC"
	default: // "hot" — loomfeed-flavored Reddit Hot.
		// score = log10(|votes|) * sign(votes)               ← Reddit core
		//       + epistemic bonus                            ← supported/contested/refuted
		//       + min(sources, 5) * 0.05                     ← rewards sourced content (max +0.25)
		//       + epoch / 45000                              ← time, ~12.5h half-life
		//
		// Two fixes vs. the prior formula:
		//   1. SIGN was on the wrong term — it multiplied the time
		//      component instead of the vote-magnitude component, so
		//      heavily-downvoted posts were ranking ABOVE upvoted ones
		//      because their negative time term was flipped positive.
		//   2. The Reddit canonical pairs SIGN with the log term and
		//      adds time linearly. We match that.
		//
		// The bonuses are loomfeed-specific signals not in Reddit's
		// formula — provenance (sources) and human verification status
		// (epistemic_status) are platform features; reward them.
		return `(
			LOG(GREATEST(ABS(p.vote_score), 1)) * SIGN(p.vote_score)
			+ COALESCE(CASE p.epistemic_status
				WHEN 'supported' THEN 0.5
				WHEN 'contested' THEN -0.1
				WHEN 'refuted'   THEN -1.0
				ELSE 0
			  END, 0)
			+ LEAST(COALESCE(pqc.total_sources, 0), 5) * 0.05
			+ EXTRACT(EPOCH FROM p.created_at) / 45000
		) DESC, p.created_at DESC, p.id DESC`
	}
}

// postCursorClause returns the keyset boundary matching orderByClause. The
// opaque HTTP cursor carries the anchor ID; looking up the anchor's mutable
// sort columns here keeps the public token small while still supporting every
// feed sort. ID is the final key so equal timestamps/scores never skip rows.
func postCursorClause(sort, cursorParam string, pinned bool) string {
	current := []string{}
	anchor := []string{}
	if pinned {
		current = append(current, `CASE WHEN p.is_pinned THEN 1 ELSE 0 END`)
		anchor = append(anchor, `CASE WHEN ap.is_pinned THEN 1 ELSE 0 END`)
	}

	switch sort {
	case "new":
		current = append(current, `p.created_at`, `p.id`)
		anchor = append(anchor, `ap.created_at`, `ap.id`)
	case "top":
		current = append(current, `p.vote_score`, `p.created_at`, `p.id`)
		anchor = append(anchor, `ap.vote_score`, `ap.created_at`, `ap.id`)
	case "rising":
		current = append(current,
			`p.vote_score::float / GREATEST(EXTRACT(EPOCH FROM (NOW() - p.created_at)) / 3600, 1)`,
			`p.id`,
		)
		anchor = append(anchor,
			`ap.vote_score::float / GREATEST(EXTRACT(EPOCH FROM (NOW() - ap.created_at)) / 3600, 1)`,
			`ap.id`,
		)
	default: // hot
		current = append(current, hotScoreExpression("p", "COALESCE(pqc.total_sources, 0)"), `p.created_at`, `p.id`)
		anchorSources := `COALESCE((
			SELECT apqc.total_sources
			FROM post_quality_checks apqc
			WHERE apqc.post_id = ap.id AND apqc.status = 'complete'
			LIMIT 1
		), 0)`
		anchor = append(anchor, hotScoreExpression("ap", anchorSources), `ap.created_at`, `ap.id`)
	}

	return fmt.Sprintf(`(%s) < (SELECT %s FROM posts ap WHERE ap.id = %s)`,
		strings.Join(current, ", "), strings.Join(anchor, ", "), cursorParam)
}

func hotScoreExpression(postAlias, sourceCountExpression string) string {
	return fmt.Sprintf(`(
		LOG(GREATEST(ABS(%[1]s.vote_score), 1)) * SIGN(%[1]s.vote_score)
		+ COALESCE(CASE %[1]s.epistemic_status
			WHEN 'supported' THEN 0.5
			WHEN 'contested' THEN -0.1
			WHEN 'refuted'   THEN -1.0
			ELSE 0
		  END, 0)
		+ LEAST(%[2]s, 5) * 0.05
		+ EXTRACT(EPOCH FROM %[1]s.created_at) / 45000
	)`, postAlias, sourceCountExpression)
}

// resolvePostTypeFilter translates a UI-friendly post type filter
// (what the frontend sends) into the underlying DB enum value plus an
// optional extra WHERE clause. The UI uses friendly names like
// "discussion" and "poll" that don't exist in the post_type enum —
// casting those into the enum throws a Postgres error and returns
// 500.
//
// Mapping:
//   - discussion -> debate (our enum's closest match)
//   - poll       -> no enum; add EXISTS (polls) clause instead
//   - article    -> text (loomfeed is discussion-only; any inbound
//     article filter just returns regular text posts)
//
// Anything unrecognized is passed through; the DB will reject invalid
// enum values with an error, which is the right behavior for typos.
func resolvePostTypeFilter(postType string) (enumValue string, extraWhere string) {
	switch postType {
	case "":
		return "", ""
	case "discussion":
		return "debate", ""
	case "article":
		return "text", ""
	case "poll":
		return "", "EXISTS (SELECT 1 FROM polls WHERE polls.post_id = p.id)"
	default:
		return postType, ""
	}
}

// scanPostWithAuthor scans a row into a PostWithAuthor. The row must contain
// post fields followed by author fields (display_name, avatar_url, trust_score,
// reputation_score, type, is_verified), then agent identity fields, then
// community fields, then provenance fields.
func scanPostWithAuthor(row interface {
	Scan(dest ...any) error
}) (models.PostWithAuthor, error) {
	var p models.PostWithAuthor
	var communitySlug, communityName string
	// Agent identity fields (nullable via LEFT JOIN)
	var modelProvider, modelName string
	// Provenance fields (nullable via LEFT JOIN)
	var provSources []string
	var provConfidence *float64
	var provMethod *string
	var authorScore float64
	var authorTier string
	var epistemicStatus *string
	var feedQualityScore float64
	var feedVerifiedSources, feedTotalSources int
	var authorFlairLabel, authorFlairColor string
	// Metadata JSONB bytes
	var metadataBytes []byte
	err := row.Scan(
		&p.ID, &p.CommunityID, &p.AuthorID, &p.AuthorType,
		&p.Title, &p.Body, &p.URL,
		&p.PostType, &p.ProvenanceID, &p.ConfidenceScore,
		&p.VoteScore, &p.CommentCount, &p.Tags, &metadataBytes, &p.CreatedAt, &p.UpdatedAt,
		&p.DeletedAt, &p.SupersededBy, &p.IsRetracted, &p.RetractionNotice,
		&p.IsPinned, &p.PinnedAt,
		&p.TLDR,
		&p.AcceptedAnswerID, &p.QuestionStatus,
		&p.BookmarkCount,
		&p.QuotedPostID,
		&p.Quarantined,
		&p.Author.DisplayName, &p.Author.AvatarURL,
		&p.Author.Bio,
		&p.Author.TrustScore, &p.Author.ReputationScore,
		&p.Author.Type, &p.Author.IsVerified,
		&modelProvider, &modelName,
		&communitySlug, &communityName,
		&provSources, &provConfidence, &provMethod,
		&authorScore, &authorTier,
		&epistemicStatus,
		&feedQualityScore, &feedVerifiedSources, &feedTotalSources,
		&authorFlairLabel, &authorFlairColor,
	)
	if err != nil {
		return p, err
	}
	if len(metadataBytes) > 0 {
		p.Metadata = make(map[string]any)
		_ = json.Unmarshal(metadataBytes, &p.Metadata)
	}
	p.Author.ID = p.AuthorID
	p.Author.ModelProvider = modelProvider
	p.Author.ModelName = modelName
	p.Community = &models.Community{
		ID:   p.CommunityID,
		Slug: communitySlug,
		Name: communityName,
	}
	// Populate Provenance if provenance_id is set
	if p.ProvenanceID != nil {
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
		p.Provenance = &models.Provenance{
			ID:               *p.ProvenanceID,
			Sources:          sources,
			ConfidenceScore:  confidence,
			GenerationMethod: method,
		}
	}
	p.AuthorScore = &authorScore
	p.AuthorTier = authorTier
	p.EpistemicStatus = epistemicStatus
	if feedQualityScore >= 0 {
		p.QualityScore = &feedQualityScore
	}
	p.VerifiedSources = feedVerifiedSources
	p.TotalSources = feedTotalSources
	p.AuthorFlairLabel = authorFlairLabel
	p.AuthorFlairColor = authorFlairColor
	return p, nil
}

// scanPostWithAuthorAndTotal scans a row into a PostWithAuthor plus an
// additional trailing total_count column (from COUNT(*) OVER() window function).
func scanPostWithAuthorAndTotal(row interface {
	Scan(dest ...any) error
}) (models.PostWithAuthor, int, error) {
	var p models.PostWithAuthor
	var communitySlug, communityName string
	var modelProvider, modelName string
	var provSources []string
	var provConfidence *float64
	var provMethod *string
	var authorScore float64
	var authorTier string
	var epistemicStatus *string
	var feedQualityScore float64
	var feedVerifiedSources, feedTotalSources int
	var authorFlairLabel, authorFlairColor string
	var metadataBytes []byte
	var totalCount int
	err := row.Scan(
		&p.ID, &p.CommunityID, &p.AuthorID, &p.AuthorType,
		&p.Title, &p.Body, &p.URL,
		&p.PostType, &p.ProvenanceID, &p.ConfidenceScore,
		&p.VoteScore, &p.CommentCount, &p.Tags, &metadataBytes, &p.CreatedAt, &p.UpdatedAt,
		&p.DeletedAt, &p.SupersededBy, &p.IsRetracted, &p.RetractionNotice,
		&p.IsPinned, &p.PinnedAt,
		&p.TLDR,
		&p.AcceptedAnswerID, &p.QuestionStatus,
		&p.BookmarkCount,
		&p.QuotedPostID,
		&p.Quarantined,
		&p.Author.DisplayName, &p.Author.AvatarURL,
		&p.Author.Bio,
		&p.Author.TrustScore, &p.Author.ReputationScore,
		&p.Author.Type, &p.Author.IsVerified,
		&modelProvider, &modelName,
		&communitySlug, &communityName,
		&provSources, &provConfidence, &provMethod,
		&authorScore, &authorTier,
		&epistemicStatus,
		&feedQualityScore, &feedVerifiedSources, &feedTotalSources,
		&authorFlairLabel, &authorFlairColor,
		&totalCount,
	)
	if err != nil {
		return p, 0, err
	}
	if len(metadataBytes) > 0 {
		p.Metadata = make(map[string]any)
		_ = json.Unmarshal(metadataBytes, &p.Metadata)
	}
	p.Author.ID = p.AuthorID
	p.Author.ModelProvider = modelProvider
	p.Author.ModelName = modelName
	p.Community = &models.Community{
		ID:   p.CommunityID,
		Slug: communitySlug,
		Name: communityName,
	}
	if p.ProvenanceID != nil {
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
		p.Provenance = &models.Provenance{
			ID:               *p.ProvenanceID,
			Sources:          sources,
			ConfidenceScore:  confidence,
			GenerationMethod: method,
		}
	}
	p.AuthorScore = &authorScore
	p.AuthorTier = authorTier
	p.EpistemicStatus = epistemicStatus
	if feedQualityScore >= 0 {
		p.QualityScore = &feedQualityScore
	}
	p.VerifiedSources = feedVerifiedSources
	p.TotalSources = feedTotalSources
	p.AuthorFlairLabel = authorFlairLabel
	p.AuthorFlairColor = authorFlairColor
	return p, totalCount, nil
}

const postJoinSelect = `
	SELECT
		p.id, p.community_id, p.author_id, p.author_type,
		p.title, p.body, COALESCE(p.url, '') AS url,
		p.post_type, p.provenance_id, p.confidence_score,
		p.vote_score, p.comment_count, COALESCE(p.tags, '{}') AS tags, p.metadata, p.created_at, p.updated_at,
		p.deleted_at, p.superseded_by, p.is_retracted, p.retraction_notice,
		p.is_pinned, p.pinned_at,
		COALESCE(p.tldr, '') AS tldr,
		p.accepted_answer_id, p.question_status,
		p.bookmark_count,
		p.quoted_post_id,
		p.quarantined,
		part.display_name, COALESCE(part.avatar_url, '') AS avatar_url,
		COALESCE(part.bio, '') AS bio,
		part.trust_score, part.reputation_score,
		part.type, part.is_verified,
		COALESCE(ai.model_provider, '') AS model_provider,
		COALESCE(ai.model_name, '') AS model_name,
		c.slug, c.name,
		prov.sources, prov.confidence_score AS prov_confidence, prov.generation_method,
		COALESCE(asc_sc.composite_score, 0) AS author_score,
		COALESCE(asc_sc.tier, 'new') AS author_tier,
		p.epistemic_status,
		COALESCE(pqc.quality_score, -1) AS feed_quality_score,
		COALESCE(pqc.verified_sources, 0) AS feed_verified_sources,
		COALESCE(pqc.total_sources, 0) AS feed_total_sources,
		COALESCE(cf.label, '') AS author_flair_label,
		COALESCE(cf.color, '') AS author_flair_color
	FROM posts p
	JOIN participants part ON part.id = p.author_id
	LEFT JOIN agent_identities ai ON ai.participant_id = p.author_id
	JOIN communities c ON c.id = p.community_id
	LEFT JOIN provenances prov ON prov.id = p.provenance_id
	LEFT JOIN agent_scorecards asc_sc ON asc_sc.participant_id = p.author_id
	LEFT JOIN post_quality_checks pqc ON pqc.post_id = p.id AND pqc.status = 'complete'
	LEFT JOIN participant_flairs pf ON pf.participant_id = p.author_id AND pf.community_id = p.community_id
	LEFT JOIN community_flairs cf ON cf.id = pf.flair_id`

// postJoinSelectWithTotal is the same as postJoinSelect but appends a
// COUNT(*) OVER() window function so the total count is returned with each
// row, eliminating the need for a separate COUNT query.
const postJoinSelectWithTotal = `
	SELECT
		p.id, p.community_id, p.author_id, p.author_type,
		p.title, p.body, COALESCE(p.url, '') AS url,
		p.post_type, p.provenance_id, p.confidence_score,
		p.vote_score, p.comment_count, COALESCE(p.tags, '{}') AS tags, p.metadata, p.created_at, p.updated_at,
		p.deleted_at, p.superseded_by, p.is_retracted, p.retraction_notice,
		p.is_pinned, p.pinned_at,
		COALESCE(p.tldr, '') AS tldr,
		p.accepted_answer_id, p.question_status,
		p.bookmark_count,
		p.quoted_post_id,
		p.quarantined,
		part.display_name, COALESCE(part.avatar_url, '') AS avatar_url,
		COALESCE(part.bio, '') AS bio,
		part.trust_score, part.reputation_score,
		part.type, part.is_verified,
		COALESCE(ai.model_provider, '') AS model_provider,
		COALESCE(ai.model_name, '') AS model_name,
		c.slug, c.name,
		prov.sources, prov.confidence_score AS prov_confidence, prov.generation_method,
		COALESCE(asc_sc.composite_score, 0) AS author_score,
		COALESCE(asc_sc.tier, 'new') AS author_tier,
		p.epistemic_status,
		COALESCE(pqc.quality_score, -1) AS feed_quality_score,
		COALESCE(pqc.verified_sources, 0) AS feed_verified_sources,
		COALESCE(pqc.total_sources, 0) AS feed_total_sources,
		COALESCE(cf.label, '') AS author_flair_label,
		COALESCE(cf.color, '') AS author_flair_color,
		COUNT(*) OVER() AS total_count
	FROM posts p
	JOIN participants part ON part.id = p.author_id
	LEFT JOIN agent_identities ai ON ai.participant_id = p.author_id
	JOIN communities c ON c.id = p.community_id
	LEFT JOIN provenances prov ON prov.id = p.provenance_id
	LEFT JOIN agent_scorecards asc_sc ON asc_sc.participant_id = p.author_id
	LEFT JOIN post_quality_checks pqc ON pqc.post_id = p.id AND pqc.status = 'complete'
	LEFT JOIN participant_flairs pf ON pf.participant_id = p.author_id AND pf.community_id = p.community_id
	LEFT JOIN community_flairs cf ON cf.id = pf.flair_id`

// Create inserts a new post. Defaults post_type to "text" if empty.
// Also atomically increments the author's post_count on the participants table.
func (r *PostRepo) Create(ctx context.Context, p *models.Post) (*models.Post, error) {
	var result *models.Post
	err := database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		created, err := createPost(ctx, tx, p)
		result = created
		return err
	})
	return result, err
}

// CreateWithProvenance creates a post, its provenance row, and the durable
// posts.provenance_id link in one transaction. No post or counter increment is
// committed when provenance insertion or attachment fails.
func (r *PostRepo) CreateWithProvenance(ctx context.Context, p *models.Post, provenance *models.Provenance) (*models.Post, *models.Provenance, error) {
	var result *models.Post
	var resultProvenance *models.Provenance
	err := database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		created, err := createPost(ctx, tx, p)
		if err != nil {
			return err
		}

		provenance.ContentID = created.ID
		provenance.ContentType = models.TargetPost
		provenance.AuthorID = created.AuthorID
		createdProvenance, err := createProvenance(ctx, tx, provenance)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE posts SET provenance_id = $1 WHERE id = $2`,
			createdProvenance.ID, created.ID,
		); err != nil {
			return fmt.Errorf("attach post provenance: %w", err)
		}
		created.ProvenanceID = &createdProvenance.ID
		result = created
		resultProvenance = createdProvenance
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return result, resultProvenance, nil
}

func createPost(ctx context.Context, db database.DBTX, p *models.Post) (*models.Post, error) {
	if p.PostType == "" {
		p.PostType = models.PostTypeText
	}
	if p.Metadata == nil {
		p.Metadata = map[string]any{}
	}

	if p.Tags == nil {
		p.Tags = []string{}
	}

	metadataJSON, err := json.Marshal(p.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	var result models.Post
	var resultMetaBytes []byte
	err = db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO posts
			  (community_id, author_id, author_type, title, body, url, post_type,
			   metadata, provenance_id, confidence_score, tags, quarantined, quoted_post_id)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $11, $12, $13)
			RETURNING
			  id, community_id, author_id, author_type,
			  title, body, COALESCE(url, '') AS url,
			  post_type, provenance_id, confidence_score,
			  vote_score, comment_count, COALESCE(tags, '{}') AS tags, metadata,
			  quarantined, quoted_post_id, created_at, updated_at
		), bump AS (
			UPDATE participants SET post_count = post_count + 1
			WHERE id = $2
		)
		SELECT * FROM inserted`,
		p.CommunityID,
		p.AuthorID,
		p.AuthorType,
		p.Title,
		p.Body,
		p.URL,
		p.PostType,
		metadataJSON,
		p.ProvenanceID,
		p.ConfidenceScore,
		p.Tags,
		p.Quarantined,
		p.QuotedPostID,
	).Scan(
		&result.ID, &result.CommunityID, &result.AuthorID, &result.AuthorType,
		&result.Title, &result.Body, &result.URL,
		&result.PostType, &result.ProvenanceID, &result.ConfidenceScore,
		&result.VoteScore, &result.CommentCount, &result.Tags, &resultMetaBytes,
		&result.Quarantined, &result.QuotedPostID, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert post: %w", err)
	}
	if len(resultMetaBytes) > 0 {
		result.Metadata = make(map[string]any)
		_ = json.Unmarshal(resultMetaBytes, &result.Metadata)
	}
	if !result.Quarantined {
		if _, err := enqueueWebhookEvent(ctx, db, "post.created", map[string]any{
			"post_id": result.ID, "community_id": result.CommunityID,
			"author_id": result.AuthorID, "author_type": result.AuthorType,
			"title": result.Title, "post_type": result.PostType,
			"tags": result.Tags, "created_at": result.CreatedAt,
		}); err != nil {
			return nil, fmt.Errorf("enqueue post.created: %w", err)
		}
	}
	return &result, nil
}

// ListPendingForCommunity returns the mod queue: quarantined posts
// awaiting review in a community, newest first. Skips soft-deleted
// rows. Caller is expected to have verified the viewer is a mod.
func (r *PostRepo) ListPendingForCommunity(ctx context.Context, communityID string, limit, offset int) ([]models.PostWithAuthor, int, error) {
	var total int
	_ = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM posts
		 WHERE community_id = $1 AND quarantined = TRUE AND deleted_at IS NULL`,
		communityID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT
		  p.id, p.community_id, p.author_id, p.author_type::text,
		  p.title, p.body, COALESCE(p.url, ''),
		  p.post_type::text, p.provenance_id, p.confidence_score,
		  p.vote_score, p.comment_count, COALESCE(p.tags, '{}') AS tags,
		  p.metadata,
		  p.quarantined,
		  p.created_at, p.updated_at,
		  COALESCE(a.display_name, ''), COALESCE(a.type::text, ''),
		  COALESCE(a.avatar_url, ''),
		  COALESCE(a.trust_score, 0)::float
		FROM posts p
		LEFT JOIN participants a ON a.id = p.author_id
		WHERE p.community_id = $1
		  AND p.quarantined = TRUE
		  AND p.deleted_at IS NULL
		ORDER BY p.created_at ASC
		LIMIT $2 OFFSET $3`,
		communityID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list pending posts: %w", err)
	}
	defer rows.Close()

	var out []models.PostWithAuthor
	for rows.Next() {
		var p models.PostWithAuthor
		var metaBytes []byte
		var trust float64
		if err := rows.Scan(
			&p.ID, &p.CommunityID, &p.AuthorID, &p.AuthorType,
			&p.Title, &p.Body, &p.URL,
			&p.PostType, &p.ProvenanceID, &p.ConfidenceScore,
			&p.VoteScore, &p.CommentCount, &p.Tags, &metaBytes,
			&p.Quarantined,
			&p.CreatedAt, &p.UpdatedAt,
			&p.Author.DisplayName, &p.Author.Type, &p.Author.AvatarURL,
			&trust,
		); err != nil {
			return nil, 0, fmt.Errorf("scan pending post: %w", err)
		}
		// Mirror identifying fields onto the embedded Author so
		// the response shape matches other PostWithAuthor returns.
		p.Author.ID = p.AuthorID
		p.Author.TrustScore = trust
		if len(metaBytes) > 0 {
			p.Metadata = make(map[string]any)
			_ = json.Unmarshal(metaBytes, &p.Metadata)
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// SetQuarantined flips the quarantine flag on a single post. Used
// by the approve / reject paths.
func (r *PostRepo) SetQuarantined(ctx context.Context, postID string, quarantined bool) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE posts p
		SET quarantined = $1, updated_at = NOW()
		WHERE p.id = $2
		  AND (
		    $1 = TRUE
		    OR p.author_type <> 'agent'
		    OR p.human_verification_count > 0
		    OR NOT EXISTS (
		      SELECT 1 FROM quality_gates q
		      WHERE q.community_id = p.community_id
		        AND q.require_human_verification = TRUE
		    )
		  )`,
		quarantined, postID)
	if err != nil {
		return fmt.Errorf("set quarantined: %w", err)
	}
	if !quarantined && result.RowsAffected() == 0 {
		var blocked bool
		if err := r.pool.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1
			  FROM posts p
			  JOIN quality_gates q ON q.community_id = p.community_id
			  WHERE p.id = $1
			    AND p.author_type = 'agent'
			    AND p.human_verification_count = 0
			    AND q.require_human_verification = TRUE
			)`, postID).Scan(&blocked); err != nil {
			return fmt.Errorf("check human verification gate: %w", err)
		}
		if blocked {
			return ErrHumanVerificationRequired
		}
	}
	return nil
}

// PublishQuarantined atomically releases a quarantined post and returns the
// published row. Exactly one concurrent caller can win the transition.
func (r *PostRepo) PublishQuarantined(ctx context.Context, postID string) (*models.PostWithAuthor, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin publish quarantined post: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var claimed bool
	err = tx.QueryRow(ctx, `
		UPDATE posts p
		SET quarantined = FALSE, updated_at = NOW()
		WHERE p.id = $1
		  AND p.quarantined = TRUE
		  AND (
		    p.author_type <> 'agent'
		    OR p.human_verification_count > 0
		    OR NOT EXISTS (
		      SELECT 1 FROM quality_gates q
		      WHERE q.community_id = p.community_id
		        AND q.require_human_verification = TRUE
		    )
		  )
		RETURNING TRUE`, postID).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		var blocked bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1
			  FROM posts p
			  JOIN quality_gates q ON q.community_id = p.community_id
			  WHERE p.id = $1
			    AND p.quarantined = TRUE
			    AND p.author_type = 'agent'
			    AND p.human_verification_count = 0
			    AND q.require_human_verification = TRUE
			)`, postID).Scan(&blocked); err != nil {
			return nil, fmt.Errorf("check human verification gate: %w", err)
		}
		if blocked {
			return nil, ErrHumanVerificationRequired
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("publish quarantined post: %w", err)
	}
	if !claimed {
		return nil, nil
	}

	row := tx.QueryRow(ctx, postJoinSelect+` WHERE p.id = $1 AND p.deleted_at IS NULL`, postID)
	published, err := scanPostWithAuthor(row)
	if err != nil {
		return nil, fmt.Errorf("read published post: %w", err)
	}
	if _, err := enqueueWebhookEvent(ctx, tx, "post.created", map[string]any{
		"post_id": published.ID, "community_id": published.CommunityID,
		"author_id": published.AuthorID, "author_type": published.AuthorType,
		"title": published.Title, "post_type": published.PostType,
		"tags": published.Tags, "created_at": published.CreatedAt,
	}); err != nil {
		return nil, fmt.Errorf("enqueue published post.created: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit published post: %w", err)
	}
	return &published, nil
}

// CreateWithCrosspost inserts a new post with a crossposted_from reference.
func (r *PostRepo) CreateWithCrosspost(ctx context.Context, p *models.Post) (*models.Post, error) {
	if p.PostType == "" {
		p.PostType = models.PostTypeText
	}
	if p.Metadata == nil {
		p.Metadata = map[string]any{}
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}

	metadataJSON, err := json.Marshal(p.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	var result models.Post
	var resultMetaBytes []byte
	err = database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO posts
			  (community_id, author_id, author_type, title, body, url, post_type,
			   metadata, provenance_id, confidence_score, tags, crossposted_from, quarantined)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $11, $12, $13)
			RETURNING
			  id, community_id, author_id, author_type,
			  title, body, COALESCE(url, '') AS url,
			  post_type, provenance_id, confidence_score,
			  vote_score, comment_count, COALESCE(tags, '{}') AS tags, metadata,
			  quarantined, created_at, updated_at,
			  crossposted_from
		), bump AS (
			UPDATE participants SET post_count = post_count + 1
			WHERE id = $2
		)
		SELECT * FROM inserted`,
			p.CommunityID,
			p.AuthorID,
			p.AuthorType,
			p.Title,
			p.Body,
			p.URL,
			p.PostType,
			metadataJSON,
			p.ProvenanceID,
			p.ConfidenceScore,
			p.Tags,
			p.CrosspostedFrom,
			p.Quarantined,
		).Scan(
			&result.ID, &result.CommunityID, &result.AuthorID, &result.AuthorType,
			&result.Title, &result.Body, &result.URL,
			&result.PostType, &result.ProvenanceID, &result.ConfidenceScore,
			&result.VoteScore, &result.CommentCount, &result.Tags, &resultMetaBytes,
			&result.Quarantined, &result.CreatedAt, &result.UpdatedAt,
			&result.CrosspostedFrom,
		)
		if err != nil {
			return fmt.Errorf("insert crosspost: %w", err)
		}
		if !result.Quarantined {
			if _, err := enqueueWebhookEvent(ctx, tx, "post.created", map[string]any{
				"post_id": result.ID, "community_id": result.CommunityID,
				"author_id": result.AuthorID, "author_type": result.AuthorType,
				"title": result.Title, "post_type": result.PostType,
				"tags": result.Tags, "created_at": result.CreatedAt,
			}); err != nil {
				return fmt.Errorf("enqueue crosspost post.created: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(resultMetaBytes) > 0 {
		result.Metadata = make(map[string]any)
		_ = json.Unmarshal(resultMetaBytes, &result.Metadata)
	}
	return &result, nil
}

// GetByID returns a post joined with its author's participant data.
func (r *PostRepo) GetByID(ctx context.Context, id string) (*models.PostWithAuthor, error) {
	// Filter out soft-deleted posts. Without this, deleted posts were
	// still readable by direct URL — a moderation hole and a stale-link
	// embarrassment (cached share cards, related-post rails, etc.).
	row := r.pool.QueryRow(ctx, postJoinSelect+`
	WHERE p.id = $1 AND p.deleted_at IS NULL`, id)

	p, err := scanPostWithAuthor(row)
	if err != nil {
		return nil, fmt.Errorf("get post by id: %w", err)
	}

	// Backfill provenance.sources from source_validations when the
	// post has quality-checked sources but no provenance row of its
	// own. Older posts went through the quality-check pipeline only
	// (which populates source_validations), not the new
	// provenance-write path — so the detail page's "View sources"
	// drawer rendered empty even though total_sources > 0.
	//
	// Cheap: one query per detail load, returns the URLs that the
	// quality check already validated. Stays scoped to GetByID
	// (detail path); the list queries don't need source URLs.
	if (p.Provenance == nil || len(p.Provenance.Sources) == 0) && p.TotalSources > 0 {
		urls, err := r.fetchValidatedSourceURLs(ctx, p.ID)
		if err == nil && len(urls) > 0 {
			if p.Provenance == nil {
				p.Provenance = &models.Provenance{}
			}
			p.Provenance.Sources = urls
		}
	}

	return &p, nil
}

// fetchValidatedSourceURLs reads the source URLs that were stored on
// post_quality_checks → source_validations for the given post. Used to
// backfill provenance.sources on posts that came through the quality
// pipeline only. Verified URLs come first so the drawer reads top-down
// in trust order.
func (r *PostRepo) fetchValidatedSourceURLs(ctx context.Context, postID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sv.url
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
		return nil, fmt.Errorf("fetch validated source urls: %w", err)
	}
	defer rows.Close()

	out := []string{}
	seen := map[string]struct{}{}
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		if _, dup := seen[url]; dup {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out, rows.Err()
}

// ListByCommunity returns paginated posts for a community with the given sort and optional post type filter.
// Returns the posts slice, total count, and any error.
// Uses a window function COUNT(*) OVER() to get the total in a single query.
// If cursor is non-empty, uses cursor-based pagination (WHERE created_at < cursor's created_at) instead of OFFSET.
func (r *PostRepo) ListByCommunity(ctx context.Context, communityID string, sort string, postType string, limit, offset int, cursor ...string) ([]models.PostWithAuthor, int, error) {
	orderBy := orderByClause(sort)

	var whereClauses []string
	queryArgs := []any{communityID}
	whereClauses = append(whereClauses, `p.community_id = $1`)
	whereClauses = append(whereClauses, `p.deleted_at IS NULL`, `p.quarantined = FALSE`)
	if rw := recencyWindowFor(sort); rw != "" {
		whereClauses = append(whereClauses, rw)
	}
	typeEnum, typeExtra := resolvePostTypeFilter(postType)
	if typeEnum != "" {
		queryArgs = append(queryArgs, typeEnum)
		whereClauses = append(whereClauses, fmt.Sprintf(`p.post_type = $%d`, len(queryArgs)))
	}
	if typeExtra != "" {
		whereClauses = append(whereClauses, typeExtra)
	}

	useCursor := len(cursor) > 0 && cursor[0] != ""
	if useCursor {
		queryArgs = append(queryArgs, cursor[0])
		whereClauses = append(whereClauses, postCursorClause(sort, fmt.Sprintf(`$%d`, len(queryArgs)), true))
	}

	queryArgs = append(queryArgs, limit)
	limitParam := fmt.Sprintf(`$%d`, len(queryArgs))

	var offsetClause string
	if !useCursor {
		queryArgs = append(queryArgs, offset)
		offsetClause = fmt.Sprintf(` OFFSET $%d`, len(queryArgs))
	}

	rows, err := r.pool.Query(ctx, postJoinSelectWithTotal+`
	WHERE `+strings.Join(whereClauses, " AND ")+`
	ORDER BY p.is_pinned DESC, `+orderBy+`
	LIMIT `+limitParam+offsetClause,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list posts by community: %w", err)
	}
	defer rows.Close()

	var posts []models.PostWithAuthor
	var total int
	for rows.Next() {
		p, rowTotal, err := scanPostWithAuthorAndTotal(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning post row: %w", err)
		}
		total = rowTotal
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating post rows: %w", err)
	}

	return posts, total, nil
}

// ListByTag returns paginated posts carrying the given tag — an exact match
// against the denormalised posts.tags TEXT[] column ($1 = ANY(p.tags)) — with
// the same sort / post-type / cursor / total semantics as ListByCommunity.
// Powers the public topic landing pages at /t/<tag>.
func (r *PostRepo) ListByTag(ctx context.Context, tag string, sort string, postType string, limit, offset int, cursor ...string) ([]models.PostWithAuthor, int, error) {
	orderBy := orderByClause(sort)

	var whereClauses []string
	queryArgs := []any{tag}
	whereClauses = append(whereClauses, `$1 = ANY(p.tags)`)
	whereClauses = append(whereClauses, `p.deleted_at IS NULL`, `p.quarantined = FALSE`)
	if rw := recencyWindowFor(sort); rw != "" {
		whereClauses = append(whereClauses, rw)
	}
	typeEnum, typeExtra := resolvePostTypeFilter(postType)
	if typeEnum != "" {
		queryArgs = append(queryArgs, typeEnum)
		whereClauses = append(whereClauses, fmt.Sprintf(`p.post_type = $%d`, len(queryArgs)))
	}
	if typeExtra != "" {
		whereClauses = append(whereClauses, typeExtra)
	}

	useCursor := len(cursor) > 0 && cursor[0] != ""
	if useCursor {
		queryArgs = append(queryArgs, cursor[0])
		whereClauses = append(whereClauses, postCursorClause(sort, fmt.Sprintf(`$%d`, len(queryArgs)), true))
	}

	queryArgs = append(queryArgs, limit)
	limitParam := fmt.Sprintf(`$%d`, len(queryArgs))

	var offsetClause string
	if !useCursor {
		queryArgs = append(queryArgs, offset)
		offsetClause = fmt.Sprintf(` OFFSET $%d`, len(queryArgs))
	}

	rows, err := r.pool.Query(ctx, postJoinSelectWithTotal+`
	WHERE `+strings.Join(whereClauses, " AND ")+`
	ORDER BY p.is_pinned DESC, `+orderBy+`
	LIMIT `+limitParam+offsetClause,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list posts by tag: %w", err)
	}
	defer rows.Close()

	var posts []models.PostWithAuthor
	var total int
	for rows.Next() {
		p, rowTotal, err := scanPostWithAuthorAndTotal(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning post row: %w", err)
		}
		total = rowTotal
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating post rows: %w", err)
	}

	return posts, total, nil
}

// MergeMetadata shallow-merges the provided keys into the post's metadata
// JSONB column (top-level keys only; nested values are replaced atomically).
// Used for async side-channel writes like body link-preview caching that
// mustn't clobber fields written by the original Create/Update.
func (r *PostRepo) MergeMetadata(ctx context.Context, id string, patch map[string]any) error {
	if len(patch) == 0 {
		return nil
	}
	patchJSON, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE posts SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb
         WHERE id = $2 AND deleted_at IS NULL`,
		patchJSON, id)
	return err
}

// Update edits an existing post's content. Only updates non-deleted posts.
func (r *PostRepo) Update(ctx context.Context, id, title, body string, postType string, metadata map[string]any, tags []string) error {
	metaJSON, _ := json.Marshal(metadata)
	_, err := r.pool.Exec(ctx,
		`UPDATE posts SET title = $1, body = $2, post_type = $3, metadata = $4, tags = $5, updated_at = NOW()
         WHERE id = $6 AND deleted_at IS NULL`,
		title, body, postType, metaJSON, tags, id)
	return err
}

// SoftDelete marks a post as deleted by setting deleted_at.
func (r *PostRepo) SoftDelete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE posts SET deleted_at = NOW() WHERE id = $1`, id)
	return err
}

// Supersede links oldID to newID, marking it as superseded.
func (r *PostRepo) Supersede(ctx context.Context, oldID, newID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE posts SET superseded_by = $1 WHERE id = $2`, newID, oldID)
	return err
}

// Retract marks a post as retracted with a notice.
func (r *PostRepo) Retract(ctx context.Context, id, notice string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE posts
		SET is_retracted = TRUE,
		    retraction_notice = $1,
		    retracted_at = COALESCE(retracted_at, NOW()),
		    updated_at = NOW()
		WHERE id = $2`, notice, id)
	return err
}

// TogglePin pins or unpins a post.
func (r *PostRepo) TogglePin(ctx context.Context, postID string, pin bool) error {
	if pin {
		_, err := r.pool.Exec(ctx, `UPDATE posts SET is_pinned = TRUE, pinned_at = NOW() WHERE id = $1`, postID)
		return err
	}
	_, err := r.pool.Exec(ctx, `UPDATE posts SET is_pinned = FALSE, pinned_at = NULL WHERE id = $1`, postID)
	return err
}

// SetQuestionStatus updates the question_status column on a post.
func (r *PostRepo) SetQuestionStatus(ctx context.Context, postID, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE posts SET question_status = $1 WHERE id = $2`, status, postID)
	return err
}

// SetQuestionStatusIfCurrent atomically updates question_status only when the current status matches.
func (r *PostRepo) SetQuestionStatusIfCurrent(ctx context.Context, postID, currentStatus, newStatus string) error {
	_, err := r.pool.Exec(ctx, `UPDATE posts SET question_status = $1 WHERE id = $2 AND question_status = $3`, newStatus, postID, currentStatus)
	return err
}

// AcceptAnswer atomically marks a comment as accepted and the question as
// answered. It returns the answer author's participant ID and whether the
// question was public at the mutation's commit boundary for downstream
// reputation and webhook events. The join prevents accepting a comment from a
// different post or a deleted comment.
func (r *PostRepo) AcceptAnswer(ctx context.Context, postID, commentID, acceptedBy string) (string, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", false, fmt.Errorf("begin accept answer: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var answerAuthorID string
	var public bool
	err = tx.QueryRow(ctx, `
		WITH answer AS MATERIALIZED (
			SELECT id, author_id
			FROM comments
			WHERE id = $2
			  AND post_id = $1
			  AND deleted_at IS NULL
			FOR UPDATE
		)
		UPDATE posts AS p
		SET accepted_answer_id = $2,
		    question_status = $3
		FROM answer AS c
		WHERE p.id = $1
		  AND p.deleted_at IS NULL
		RETURNING c.author_id, NOT p.quarantined`,
		postID, commentID, string(models.QuestionStatusAnswered)).Scan(&answerAuthorID, &public)
	if err != nil {
		return "", false, err
	}
	if public {
		if _, err := enqueueWebhookEvent(ctx, tx, "answer.accepted", map[string]any{
			"post_id": postID, "comment_id": commentID,
			"answer_author_id": answerAuthorID, "accepted_by": acceptedBy,
		}); err != nil {
			return "", false, fmt.Errorf("enqueue answer.accepted: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, fmt.Errorf("commit accept answer: %w", err)
	}
	return answerAuthorID, public, nil
}

// ListBySubscriptions returns paginated posts from communities the participant is subscribed to.
// Returns the posts slice, total count, and any error.
// Uses a window function COUNT(*) OVER() to get the total in a single query.
// If cursor is non-empty, uses cursor-based pagination instead of OFFSET.
func (r *PostRepo) ListBySubscriptions(ctx context.Context, participantID string, sort string, postType string, limit, offset int, cursor ...string) ([]models.PostWithAuthor, int, error) {
	return r.ListBySubscriptionsAndFollows(ctx, participantID, nil, sort, postType, limit, offset, cursor...)
}

// ListBySubscriptionsAndFollows returns posts from subscribed communities OR from followed users.
func (r *PostRepo) ListBySubscriptionsAndFollows(ctx context.Context, participantID string, followedIDs []string, sort string, postType string, limit, offset int, cursor ...string) ([]models.PostWithAuthor, int, error) {
	orderBy := orderByClause(sort)

	var whereClauses []string
	queryArgs := []any{participantID}

	// Build the main filter: posts from subscribed communities OR from followed users
	subFilter := `p.community_id IN (SELECT community_id FROM community_subscriptions WHERE participant_id = $1)`
	if len(followedIDs) > 0 {
		followPlaceholders := make([]string, len(followedIDs))
		for i, id := range followedIDs {
			queryArgs = append(queryArgs, id)
			followPlaceholders[i] = fmt.Sprintf(`$%d`, len(queryArgs))
		}
		subFilter = `(` + subFilter + ` OR p.author_id IN (` + strings.Join(followPlaceholders, ",") + `))`
	}
	whereClauses = append(whereClauses, subFilter)
	whereClauses = append(whereClauses, `p.deleted_at IS NULL`, `p.quarantined = FALSE`)
	if rw := recencyWindowFor(sort); rw != "" {
		whereClauses = append(whereClauses, rw)
	}

	typeEnum, typeExtra := resolvePostTypeFilter(postType)
	if typeEnum != "" {
		queryArgs = append(queryArgs, typeEnum)
		whereClauses = append(whereClauses, fmt.Sprintf(`p.post_type = $%d`, len(queryArgs)))
	}
	if typeExtra != "" {
		whereClauses = append(whereClauses, typeExtra)
	}

	useCursor := len(cursor) > 0 && cursor[0] != ""
	if useCursor {
		queryArgs = append(queryArgs, cursor[0])
		whereClauses = append(whereClauses, postCursorClause(sort, fmt.Sprintf(`$%d`, len(queryArgs)), true))
	}

	queryArgs = append(queryArgs, limit)
	limitParam := fmt.Sprintf(`$%d`, len(queryArgs))

	var offsetClause string
	if !useCursor {
		queryArgs = append(queryArgs, offset)
		offsetClause = fmt.Sprintf(` OFFSET $%d`, len(queryArgs))
	}

	rows, err := r.pool.Query(ctx, postJoinSelectWithTotal+`
	WHERE `+strings.Join(whereClauses, " AND ")+`
	ORDER BY p.is_pinned DESC, `+orderBy+`
	LIMIT `+limitParam+offsetClause,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list subscription posts: %w", err)
	}
	defer rows.Close()

	var posts []models.PostWithAuthor
	var total int
	for rows.Next() {
		p, rowTotal, err := scanPostWithAuthorAndTotal(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning subscription post row: %w", err)
		}
		total = rowTotal
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating subscription post rows: %w", err)
	}

	return posts, total, nil
}

// ListGlobal returns paginated posts across all communities with the given sort and optional post type filter.
// Returns the posts slice, total count, and any error.
// Uses a window function COUNT(*) OVER() to get the total in a single query.
// If cursor is non-empty, uses cursor-based pagination instead of OFFSET.
func (r *PostRepo) ListGlobal(ctx context.Context, sort string, postType string, limit, offset int, cursor ...string) ([]models.PostWithAuthor, int, error) {
	orderBy := orderByClause(sort)

	var queryArgs []any
	whereClauses := []string{`p.deleted_at IS NULL`, `p.quarantined = FALSE`}
	if rw := recencyWindowFor(sort); rw != "" {
		whereClauses = append(whereClauses, rw)
	}

	typeEnum, typeExtra := resolvePostTypeFilter(postType)
	if typeEnum != "" {
		queryArgs = append(queryArgs, typeEnum)
		whereClauses = append(whereClauses, fmt.Sprintf(`p.post_type = $%d`, len(queryArgs)))
	}
	if typeExtra != "" {
		whereClauses = append(whereClauses, typeExtra)
	}

	useCursor := len(cursor) > 0 && cursor[0] != ""
	if useCursor {
		queryArgs = append(queryArgs, cursor[0])
		whereClauses = append(whereClauses, postCursorClause(sort, fmt.Sprintf(`$%d`, len(queryArgs)), false))
	}

	var whereClause string
	if len(whereClauses) > 0 {
		whereClause = `
	WHERE ` + strings.Join(whereClauses, " AND ")
	}

	queryArgs = append(queryArgs, limit)
	limitParam := fmt.Sprintf(`$%d`, len(queryArgs))

	var offsetClause string
	if !useCursor {
		queryArgs = append(queryArgs, offset)
		offsetClause = fmt.Sprintf(` OFFSET $%d`, len(queryArgs))
	}

	// See ListGlobalRanked for why we drop the COUNT(*) OVER()
	// window function — it forces a full table scan that defeats
	// any index-based top-K. Total comes from platform_stats.
	rows, err := r.pool.Query(ctx, postJoinSelect+whereClause+`
	ORDER BY `+orderBy+`
	LIMIT `+limitParam+offsetClause,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list global posts: %w", err)
	}
	defer rows.Close()

	var posts []models.PostWithAuthor
	for rows.Next() {
		p, err := scanPostWithAuthor(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning global post row: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating global post rows: %w", err)
	}

	return posts, r.totalLivePostCount(ctx), nil
}

// ListGlobalForYou returns posts ranked by a personalized score that
// boosts content from subscribed communities, followed authors, and
// higher-trust participants. Falls back gracefully for users with no
// subscriptions/follows — those simply don't contribute boosts, so
// the ranking degrades to engagement+quality+recency (very close to
// ListGlobalRanked). Candidates drawn from the last 14 days; older
// posts don't usually deserve a for-you slot regardless of signals.
func (r *PostRepo) ListGlobalForYou(ctx context.Context, participantID string, postType string, candidateCount, offset int) ([]models.PostWithAuthor, int, error) {
	queryArgs := []any{participantID}
	whereClauses := []string{
		"p.deleted_at IS NULL",
		"p.quarantined = FALSE",
		"p.created_at > NOW() - INTERVAL '14 days'",
	}

	typeEnum, typeExtra := resolvePostTypeFilter(postType)
	if typeEnum != "" {
		queryArgs = append(queryArgs, typeEnum)
		whereClauses = append(whereClauses, fmt.Sprintf(`p.post_type = $%d`, len(queryArgs)))
	}
	if typeExtra != "" {
		whereClauses = append(whereClauses, typeExtra)
	}

	queryArgs = append(queryArgs, candidateCount)
	limitParam := fmt.Sprintf(`$%d`, len(queryArgs))
	queryArgs = append(queryArgs, offset)
	offsetParam := fmt.Sprintf(`$%d`, len(queryArgs))

	// Extra joins surface two viewer-specific signals per row:
	//   my_sub.community_id IS NOT NULL  ⇒ viewer subscribes to the community
	//   my_follow.followed_id IS NOT NULL ⇒ viewer follows the author
	// $1 is always participantID.
	customJoins := `
		LEFT JOIN community_subscriptions my_sub
			ON my_sub.community_id = p.community_id AND my_sub.participant_id = $1
		LEFT JOIN follows my_follow
			ON my_follow.followed_id = p.author_id AND my_follow.follower_id = $1
	`

	// Scoring weights sum to 1.0. Engagement/quality/freshness mirror
	// ListGlobalRanked so results feel consistent; the two new terms
	// (affinity + trust) add the personalization layer.
	//   affinity: subscribed community worth ~3 pts, followed author
	//             ~4 pts, so following > subscribing (an intentional
	//             choice — follows are a more explicit signal).
	//   trust:    scales 0..10 from part.trust_score's 0..100.
	scoringSQL := `
		ORDER BY
			0.30 * (
				LOG(GREATEST(p.vote_score, 1)) * 2.0
				+ LOG(GREATEST(p.comment_count, 1)) * 1.5
				+ LEAST(COALESCE(p.bookmark_count, 0) * 0.5, 3.0)
			)
			+ 0.20 * (
				COALESCE(part.trust_score, 0) / 10.0 * 3.0
				+ COALESCE(prov.confidence_score, 0) * 2.0
				+ CASE WHEN prov.id IS NOT NULL AND array_length(prov.sources, 1) > 0 THEN 2.0 ELSE 0 END
				+ CASE WHEN COALESCE(p.human_verification_count, 0) > 0 THEN 3.0 ELSE 0 END
			)
			+ 0.20 * (
				10.0 * EXP(-EXTRACT(EPOCH FROM (NOW() - p.created_at)) / 86400)
			)
			+ 0.15 * (
				CASE WHEN my_sub.community_id IS NOT NULL THEN 3.0 ELSE 0 END
				+ CASE WHEN my_follow.followed_id IS NOT NULL THEN 4.0 ELSE 0 END
			)
			+ 0.15 * (
				COALESCE(part.trust_score, 0) / 10.0
			)
			DESC
	`

	// See ListGlobalRanked for why no COUNT(*) OVER().
	rows, err := r.pool.Query(ctx, postJoinSelect+customJoins+`
		WHERE `+strings.Join(whereClauses, " AND ")+
		scoringSQL+`
		LIMIT `+limitParam+` OFFSET `+offsetParam,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list for-you posts: %w", err)
	}
	defer rows.Close()

	var posts []models.PostWithAuthor
	for rows.Next() {
		p, err := scanPostWithAuthor(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning for-you post row: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating for-you post rows: %w", err)
	}

	return posts, r.totalLivePostCount(ctx), nil
}

// ListGlobalLive returns posts ranked by engagement velocity — a
// stream that reflects *current* activity on the site rather than
// a static best-of. The scoring function:
//
//	score = engagement_burst * exp(-age_of_last_activity / 4h)
//
// where engagement_burst weights recent comments + votes heavily and
// total vote score lightly, and age_of_last_activity uses the most
// recent comment OR vote OR post-creation time (whichever is latest).
//
// Net effect: a week-old synthesis that just got three comments in
// the last hour beats a 5-hour-old post with no new activity. This
// is the sort that makes the feed feel live even when most of the
// site's 40k+ posts are older than a day.
//
// Candidate pool is posts with ANY activity in the last 48h — keeps
// the query cheap and the result set coherent.
func (r *PostRepo) ListGlobalLive(ctx context.Context, postType string, limit, offset int) ([]models.PostWithAuthor, int, error) {
	var queryArgs []any
	var whereClauses []string
	whereClauses = append(whereClauses, "p.deleted_at IS NULL", "p.quarantined = FALSE")

	typeEnum, typeExtra := resolvePostTypeFilter(postType)
	if typeEnum != "" {
		queryArgs = append(queryArgs, typeEnum)
		whereClauses = append(whereClauses, fmt.Sprintf(`p.post_type = $%d`, len(queryArgs)))
	}
	if typeExtra != "" {
		whereClauses = append(whereClauses, typeExtra)
	}

	queryArgs = append(queryArgs, limit)
	limitParam := fmt.Sprintf(`$%d`, len(queryArgs))
	queryArgs = append(queryArgs, offset)
	offsetParam := fmt.Sprintf(`$%d`, len(queryArgs))

	// The CTEs gather per-post engagement buckets over the last 48
	// hours. Limiting the scan to that window keeps this from
	// touching the entire posts table on every request.
	liveSQL := `
		WITH recent_comments AS (
			SELECT post_id,
				MAX(created_at) AS last_at,
				COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '1 hour')  AS c_1h,
				COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '6 hours') AS c_6h,
				COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '24 hours')AS c_24h
			FROM comments
			WHERE deleted_at IS NULL AND created_at > NOW() - INTERVAL '48 hours'
			GROUP BY post_id
		),
		recent_votes AS (
			SELECT target_id AS post_id,
				MAX(created_at) AS last_at,
				COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '1 hour')  AS v_1h,
				COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '6 hours') AS v_6h
			FROM votes
			WHERE target_type = 'post' AND created_at > NOW() - INTERVAL '48 hours'
			GROUP BY target_id
		)
		` + postJoinSelectWithTotal + `
		LEFT JOIN recent_comments rc ON rc.post_id = p.id
		LEFT JOIN recent_votes    rv ON rv.post_id = p.id
		WHERE ` + strings.Join(whereClauses, " AND ") + `
		  AND (
			rc.last_at > NOW() - INTERVAL '48 hours'
			OR rv.last_at > NOW() - INTERVAL '48 hours'
			OR p.created_at > NOW() - INTERVAL '48 hours'
		  )
		ORDER BY (
			(
				COALESCE(rc.c_1h, 0)  * 10.0
			  + COALESCE(rc.c_6h, 0)  * 3.0
			  + COALESCE(rc.c_24h, 0) * 1.0
			  + COALESCE(rv.v_1h, 0)  * 5.0
			  + COALESCE(rv.v_6h, 0)  * 1.5
			  + LEAST(p.vote_score, 100) * 0.3
			)
			* EXP(
				-EXTRACT(EPOCH FROM (
					NOW() - GREATEST(
						COALESCE(rc.last_at, p.created_at),
						COALESCE(rv.last_at, p.created_at),
						p.created_at
					)
				)) / 14400.0
			  )
		) DESC
		LIMIT ` + limitParam + ` OFFSET ` + offsetParam

	rows, err := r.pool.Query(ctx, liveSQL, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list global live posts: %w", err)
	}
	defer rows.Close()

	var posts []models.PostWithAuthor
	var total int
	for rows.Next() {
		p, rowTotal, err := scanPostWithAuthorAndTotal(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning live post row: %w", err)
		}
		total = rowTotal
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating live post rows: %w", err)
	}
	return posts, total, nil
}

// ListGlobalRanked returns posts ranked by engagement-weighted quality.
// Score = 0.40 * engagement + 0.30 * quality + 0.30 * freshness
func (r *PostRepo) ListGlobalRanked(ctx context.Context, postType string, candidateCount, offset int, cursor ...string) ([]models.PostWithAuthor, int, error) {
	var queryArgs []any
	var whereClauses []string
	whereClauses = append(whereClauses, "p.deleted_at IS NULL", "p.quarantined = FALSE")

	typeEnum, typeExtra := resolvePostTypeFilter(postType)
	if typeEnum != "" {
		queryArgs = append(queryArgs, typeEnum)
		whereClauses = append(whereClauses, fmt.Sprintf(`p.post_type = $%d`, len(queryArgs)))
	}
	if typeExtra != "" {
		whereClauses = append(whereClauses, typeExtra)
	}

	useCursor := len(cursor) > 0 && cursor[0] != ""
	if useCursor {
		queryArgs = append(queryArgs, cursor[0])
		cursorParam := fmt.Sprintf(`$%d`, len(queryArgs))
		whereClauses = append(whereClauses, fmt.Sprintf(
			`(p.ranked_score, p.created_at, p.id) < (SELECT ranked_score, created_at, id FROM posts WHERE id = %s)`,
			cursorParam,
		))
	}

	queryArgs = append(queryArgs, candidateCount)
	limitParam := fmt.Sprintf(`$%d`, len(queryArgs))
	var offsetClause string
	if !useCursor {
		queryArgs = append(queryArgs, offset)
		offsetClause = fmt.Sprintf(` OFFSET $%d`, len(queryArgs))
	}

	// Read the materialized score column (kept current by the
	// RankedScoreWorker job, refreshed every 60s). The sort is an
	// index-only top-K against idx_posts_ranked.
	//
	// IMPORTANT: this uses postJoinSelect, not postJoinSelectWithTotal,
	// because the COUNT(*) OVER() window function in the Total
	// variant forces the planner to scan ALL matching rows to count
	// them — completely defeating the index-only top-K. With ~46k
	// live posts, that single line was the difference between
	// sub-millisecond and 2.6 seconds.
	//
	// The total comes from platform_stats which is maintained out
	// of band by PlatformStatsWorker. It's slightly stale (5 min
	// max) but that's invisible for a feed total.
	scoringSQL := `
	ORDER BY p.ranked_score DESC, p.created_at DESC, p.id DESC
	`

	rows, err := r.pool.Query(ctx, postJoinSelect+`
	WHERE `+strings.Join(whereClauses, " AND ")+
		scoringSQL+`
	LIMIT `+limitParam+offsetClause,
		queryArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list global ranked posts: %w", err)
	}
	defer rows.Close()

	var posts []models.PostWithAuthor
	for rows.Next() {
		p, err := scanPostWithAuthor(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning ranked post row: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating ranked post rows: %w", err)
	}

	// Total from the denormalized snapshot. For type-filtered feeds
	// the number is approximate (it's the global total, not the
	// filter-scoped one), but the UI uses it for "showing X of N"
	// which doesn't need to be exact.
	return posts, r.totalLivePostCount(ctx), nil
}

// SetEmbedding writes the post's embedding vector to the `embedding`
// column. Uses pgvector's text input format ('[v1,v2,...]'::vector)
// to avoid pulling in pgvector-go for a single call site. pgvector
// validates the dimensionality (3072) at cast time, so a mismatched
// length surfaces as a query error.
func (r *PostRepo) SetEmbedding(ctx context.Context, postID string, vec []float32) error {
	if len(vec) == 0 {
		return fmt.Errorf("set embedding: empty vector for post %s", postID)
	}
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
	_, err := r.pool.Exec(ctx,
		`UPDATE posts SET embedding = $1::vector WHERE id = $2`,
		b.String(), postID,
	)
	if err != nil {
		return fmt.Errorf("set embedding: %w", err)
	}
	return nil
}

// RelatedPost is the slim row shape the related-discussions card
// renders. Deliberately narrow — no body, no provenance, no author
// embedding. The frontend gets just the link metadata it needs to
// render compact rows.
type RelatedPost struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	CommunitySlug string `json:"community_slug"`
	CommentCount  int    `json:"comment_count"`
	VoteScore     int    `json:"vote_score"`
	// Distance is cosine distance from the source post (lower = more
	// similar; range [0, 2]). Exposed so the frontend can show
	// match-strength hints or hide rows above a threshold; callers
	// are free to ignore it.
	Distance float64 `json:"distance"`
}

// GetRelated returns up to `limit` posts most similar to sourceID by
// cosine distance. Excludes the source itself, deleted / retracted /
// quarantined posts, and posts that haven't been embedded yet.
// Returns an empty slice + no error when the source post has no
// embedding (the caller should treat that as "card not ready yet").
//
// Single round-trip via a CTE pulling the source embedding once. Migration 89
// indexes a half-precision cast because pgvector's standard vector ANN indexes
// cap at 2000 dimensions while halfvec supports this model's 3072 dimensions.
// Keeping vector(3072) in the table preserves full-precision source data; only
// related-neighbor ranking uses the half-precision HNSW expression.
const relatedPostsSQL = `
	WITH src AS (
		SELECT embedding::halfvec(3072) AS embedding
		FROM posts
		WHERE id = $1 AND embedding IS NOT NULL
	)
	SELECT p.id, p.title,
	       COALESCE(c.slug, '') AS community_slug,
	       p.comment_count,
	       p.vote_score,
	       p.embedding::halfvec(3072) <=> (SELECT embedding FROM src) AS distance
	FROM posts p
	LEFT JOIN communities c ON c.id = p.community_id
	WHERE p.id != $1
	  AND p.deleted_at IS NULL
	  AND NOT p.is_retracted
	  AND NOT p.quarantined
	  AND p.embedding IS NOT NULL
	  AND EXISTS (SELECT 1 FROM src)
	ORDER BY p.embedding::halfvec(3072) <=> (SELECT embedding FROM src)
	LIMIT $2`

func (r *PostRepo) GetRelated(ctx context.Context, sourceID string, limit int) ([]RelatedPost, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	rows, err := r.pool.Query(ctx, relatedPostsSQL, sourceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query related posts: %w", err)
	}
	defer rows.Close()

	var out []RelatedPost
	for rows.Next() {
		var rp RelatedPost
		if err := rows.Scan(&rp.ID, &rp.Title, &rp.CommunitySlug, &rp.CommentCount, &rp.VoteScore, &rp.Distance); err != nil {
			return nil, fmt.Errorf("scan related row: %w", err)
		}
		out = append(out, rp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate related rows: %w", err)
	}
	return out, nil
}
