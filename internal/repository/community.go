package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// CommunityRepo handles database operations for communities.
type CommunityRepo struct {
	pool *pgxpool.Pool
}

// NewCommunityRepo creates a new CommunityRepo.
func NewCommunityRepo(pool *pgxpool.Pool) *CommunityRepo {
	return &CommunityRepo{pool: pool}
}

// All columns are qualified with `communities.` so this field list is
// safe to reuse inside queries that JOIN other tables (e.g.
// `ListByParticipant` joins `community_moderators`, which also has a
// `created_at` column — without the prefix Postgres throws an
// ambiguous-column error and the handler returns 500).
const communityScanFields = `
	communities.id, communities.name, communities.slug,
	COALESCE(communities.description, '') as description,
	COALESCE(communities.rules, '') as rules,
	communities.agent_policy, communities.quality_threshold, communities.post_template,
	communities.category, communities.last_post_at,
	communities.created_by, communities.subscriber_count,
	communities.created_at, communities.updated_at`

// Create inserts a new community. Defaults agent_policy to "open" if empty.
func (r *CommunityRepo) Create(ctx context.Context, c *models.Community) (*models.Community, error) {
	if c.AgentPolicy == "" {
		c.AgentPolicy = models.AgentPolicyOpen
	}

	if c.Category == "" {
		c.Category = "other"
	}

	var result models.Community
	err := r.pool.QueryRow(ctx, `
		INSERT INTO communities
		  (name, slug, description, rules, agent_policy, quality_threshold, category, created_by)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, $8)
		RETURNING`+communityScanFields,
		c.Name,
		c.Slug,
		c.Description,
		c.Rules,
		c.AgentPolicy,
		c.QualityThreshold,
		c.Category,
		c.CreatedBy,
	).Scan(
		&result.ID, &result.Name, &result.Slug,
		&result.Description, &result.Rules,
		&result.AgentPolicy, &result.QualityThreshold, &result.PostTemplate,
		&result.Category, &result.LastPostAt,
		&result.CreatedBy, &result.SubscriberCount, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert community: %w", err)
	}
	return &result, nil
}

// GetBySlug returns the community with the given slug.
func (r *CommunityRepo) GetBySlug(ctx context.Context, slug string) (*models.Community, error) {
	var c models.Community
	err := r.pool.QueryRow(ctx, `
		SELECT`+communityScanFields+`
		FROM communities
		WHERE slug = $1`,
		slug,
	).Scan(
		&c.ID, &c.Name, &c.Slug,
		&c.Description, &c.Rules,
		&c.AgentPolicy, &c.QualityThreshold, &c.PostTemplate,
		&c.Category, &c.LastPostAt,
		&c.CreatedBy, &c.SubscriberCount, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get community by slug: %w", err)
	}
	return &c, nil
}

// GetByID returns the community with the given UUID.
func (r *CommunityRepo) GetByID(ctx context.Context, id string) (*models.Community, error) {
	var c models.Community
	err := r.pool.QueryRow(ctx, `
		SELECT`+communityScanFields+`
		FROM communities
		WHERE id = $1`,
		id,
	).Scan(
		&c.ID, &c.Name, &c.Slug,
		&c.Description, &c.Rules,
		&c.AgentPolicy, &c.QualityThreshold, &c.PostTemplate,
		&c.Category, &c.LastPostAt,
		&c.CreatedBy, &c.SubscriberCount, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get community by id: %w", err)
	}
	return &c, nil
}

// CommunityListFilters describes the optional axes the discovery
// page can slice the community catalog along.
type CommunityListFilters struct {
	Sort     string // "subscribers" (default), "trending", "new", "alphabetical"
	Category string // empty = all categories
	Limit    int
	Offset   int
}

// ListWithFilters returns communities matching the given filters.
// Trending uses last_post_at as a cheap proxy for "active right now"
// — falling back on subscriber_count when nothing has been posted yet.
// New = created in the last 30 days, ordered newest first.
func (r *CommunityRepo) ListWithFilters(ctx context.Context, f CommunityListFilters) ([]models.Community, error) {
	if f.Limit <= 0 {
		f.Limit = 100
	}
	if f.Limit > 500 {
		f.Limit = 500
	}

	var (
		where  string
		args   []any
		argIdx = 1
	)
	if f.Category != "" {
		where = fmt.Sprintf("WHERE communities.category = $%d", argIdx)
		args = append(args, f.Category)
		argIdx++
	}

	var orderBy string
	switch f.Sort {
	case "trending":
		// Active first — communities with a recent post bubble to the
		// top, dead ones sink. Tiebreak on subscribers so popular-but-
		// quiet communities still rank reasonably.
		orderBy = "ORDER BY communities.last_post_at DESC NULLS LAST, communities.subscriber_count DESC"
	case "new":
		if where == "" {
			where = "WHERE communities.created_at > NOW() - INTERVAL '30 days'"
		} else {
			where += " AND communities.created_at > NOW() - INTERVAL '30 days'"
		}
		orderBy = "ORDER BY communities.created_at DESC"
	case "alphabetical":
		orderBy = "ORDER BY communities.name ASC"
	default:
		orderBy = "ORDER BY communities.subscriber_count DESC"
	}

	args = append(args, f.Limit, f.Offset)
	limitClause := fmt.Sprintf("LIMIT $%d OFFSET $%d", argIdx, argIdx+1)

	q := fmt.Sprintf("SELECT%s FROM communities %s %s %s",
		communityScanFields, where, orderBy, limitClause)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list communities with filters: %w", err)
	}
	defer rows.Close()

	var communities []models.Community
	for rows.Next() {
		var c models.Community
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug,
			&c.Description, &c.Rules,
			&c.AgentPolicy, &c.QualityThreshold, &c.PostTemplate,
			&c.Category, &c.LastPostAt,
			&c.CreatedBy, &c.SubscriberCount, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning community row: %w", err)
		}
		communities = append(communities, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating community rows: %w", err)
	}
	return communities, nil
}

// SlugExists checks whether a community slug is already in use. Used
// by the create page to validate slug uniqueness inline (without
// having to attempt an insert and parse the unique-constraint error).
func (r *CommunityRepo) SlugExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM communities WHERE slug = $1)`,
		slug).Scan(&exists)
	return exists, err
}

// List returns all communities ordered by subscriber_count DESC with pagination.
func (r *CommunityRepo) List(ctx context.Context, limit, offset int) ([]models.Community, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+communityScanFields+`
		FROM communities
		ORDER BY subscriber_count DESC
		LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list communities: %w", err)
	}
	defer rows.Close()

	var communities []models.Community
	for rows.Next() {
		var c models.Community
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug,
			&c.Description, &c.Rules,
			&c.AgentPolicy, &c.QualityThreshold, &c.PostTemplate,
			&c.Category, &c.LastPostAt,
			&c.CreatedBy, &c.SubscriberCount, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning community row: %w", err)
		}
		communities = append(communities, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating community rows: %w", err)
	}

	return communities, nil
}

// ListByParticipant returns communities created or moderated by the given participant.
func (r *CommunityRepo) ListByParticipant(ctx context.Context, participantID string) ([]models.Community, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+communityScanFields+`
		FROM communities
		WHERE communities.created_by = $1
		UNION
		SELECT`+communityScanFields+`
		FROM communities
		JOIN community_moderators cm ON cm.community_id = communities.id
		WHERE cm.participant_id = $1
		ORDER BY name`,
		participantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list communities by participant: %w", err)
	}
	defer rows.Close()

	var communities []models.Community
	for rows.Next() {
		var c models.Community
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug,
			&c.Description, &c.Rules,
			&c.AgentPolicy, &c.QualityThreshold, &c.PostTemplate,
			&c.Category, &c.LastPostAt,
			&c.CreatedBy, &c.SubscriberCount, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning community row: %w", err)
		}
		communities = append(communities, c)
	}
	return communities, nil
}

// ListSubscriptions returns every community the participant has
// subscribed to (joined). Separate from ListByParticipant, which
// returns communities they created or moderate.
func (r *CommunityRepo) ListSubscriptions(ctx context.Context, participantID string) ([]models.Community, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT`+communityScanFields+`
		FROM communities
		JOIN community_subscriptions cs ON cs.community_id = communities.id
		WHERE cs.participant_id = $1
		ORDER BY communities.name`,
		participantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscribed communities: %w", err)
	}
	defer rows.Close()

	var communities []models.Community
	for rows.Next() {
		var c models.Community
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug,
			&c.Description, &c.Rules,
			&c.AgentPolicy, &c.QualityThreshold, &c.PostTemplate,
			&c.Category, &c.LastPostAt,
			&c.CreatedBy, &c.SubscriberCount, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning community row: %w", err)
		}
		communities = append(communities, c)
	}
	return communities, nil
}

// UpdateSettings updates mutable community settings identified by id.
// Only keys present in the updates map are changed.
func (r *CommunityRepo) UpdateSettings(ctx context.Context, id string, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	// Allowlist of updatable columns
	allowed := map[string]bool{
		"description":     true,
		"rules":           true,
		"agent_policy":    true,
		"require_tags":    true,
		"min_body_length": true,
		"post_template":   true,
	}

	setClauses := make([]string, 0, len(updates)+1)
	args := make([]any, 0, len(updates)+2)
	i := 1

	for col, val := range updates {
		if !allowed[col] {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf(
		"UPDATE communities SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "),
		i,
	)

	_, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update community settings: %w", err)
	}
	return nil
}

// Delete permanently removes a community by ID.
// CASCADE constraints on the database handle posts, subscriptions, etc.
func (r *CommunityRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM communities WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete community: %w", err)
	}
	return nil
}

// Subscribe adds a participant subscription and updates subscriber_count.
// IsSubscribed checks if a participant is subscribed to a community.
func (r *CommunityRepo) IsSubscribed(ctx context.Context, communityID, participantID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM community_subscriptions WHERE community_id = $1 AND participant_id = $2)`,
		communityID, participantID).Scan(&exists)
	return exists, err
}

func (r *CommunityRepo) Subscribe(ctx context.Context, communityID, participantID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO community_subscriptions (community_id, participant_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		communityID, participantID,
	)
	if err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE communities
		SET subscriber_count = (
			SELECT COUNT(*) FROM community_subscriptions WHERE community_id = $1
		),
		updated_at = NOW()
		WHERE id = $1`,
		communityID,
	)
	if err != nil {
		return fmt.Errorf("update subscriber_count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Unsubscribe removes a participant subscription and updates subscriber_count.
func (r *CommunityRepo) Unsubscribe(ctx context.Context, communityID, participantID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		DELETE FROM community_subscriptions
		WHERE community_id = $1 AND participant_id = $2`,
		communityID, participantID,
	)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE communities
		SET subscriber_count = (
			SELECT COUNT(*) FROM community_subscriptions WHERE community_id = $1
		),
		updated_at = NOW()
		WHERE id = $1`,
		communityID,
	)
	if err != nil {
		return fmt.Errorf("update subscriber_count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
