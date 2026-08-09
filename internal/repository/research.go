package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ResearchTask represents a collaborative research task.
type ResearchTask struct {
	ID               string     `json:"id"`
	PostID           string     `json:"post_id"`
	CommunityID      string     `json:"community_id"`
	Status           string     `json:"status"`
	Question         string     `json:"question"`
	SynthesisPostID  *string    `json:"synthesis_post_id,omitempty"`
	MaxInvestigators int        `json:"max_investigators"`
	Deadline         *time.Time `json:"deadline,omitempty"`
	CreatedBy        string     `json:"created_by"`
	CreatedByName    string     `json:"created_by_name,omitempty"`
	CommunityName    string     `json:"community_name,omitempty"`
	CommunitySlug    string     `json:"community_slug,omitempty"`
	ContributionCount int       `json:"contribution_count"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ResearchContribution represents a contribution to a research task.
type ResearchContribution struct {
	ID             string    `json:"id"`
	ResearchTaskID string    `json:"research_task_id"`
	ContributorID  string    `json:"contributor_id"`
	ContributorName string   `json:"contributor_name,omitempty"`
	PostID         string    `json:"post_id"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// ResearchRepo handles database operations for research tasks.
type ResearchRepo struct {
	pool *pgxpool.Pool
}

// NewResearchRepo creates a new ResearchRepo.
func NewResearchRepo(pool *pgxpool.Pool) *ResearchRepo {
	return &ResearchRepo{pool: pool}
}

// CreateTask inserts a new research task.
func (r *ResearchRepo) CreateTask(ctx context.Context, postID, communityID, question, createdBy string, maxInvestigators int, deadline *time.Time) (*ResearchTask, error) {
	var t ResearchTask
	err := r.pool.QueryRow(ctx, `
		INSERT INTO research_tasks (post_id, community_id, question, created_by, max_investigators, deadline)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, post_id, community_id, status, question, synthesis_post_id,
		          max_investigators, deadline, created_by, created_at, updated_at`,
		postID, communityID, question, createdBy, maxInvestigators, deadline,
	).Scan(
		&t.ID, &t.PostID, &t.CommunityID, &t.Status, &t.Question, &t.SynthesisPostID,
		&t.MaxInvestigators, &t.Deadline, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create research task: %w", err)
	}
	return &t, nil
}

// GetTask returns a research task by ID with community and creator info.
func (r *ResearchRepo) GetTask(ctx context.Context, taskID string) (*ResearchTask, error) {
	var t ResearchTask
	err := r.pool.QueryRow(ctx, `
		SELECT rt.id, rt.post_id, rt.community_id, rt.status, rt.question,
		       rt.synthesis_post_id, rt.max_investigators, rt.deadline,
		       rt.created_by, COALESCE(p.display_name, '') as created_by_name,
		       COALESCE(c.name, '') as community_name,
		       COALESCE(c.slug, '') as community_slug,
		       (SELECT COUNT(*) FROM research_contributions rc WHERE rc.research_task_id = rt.id) as contribution_count,
		       rt.created_at, rt.updated_at
		FROM research_tasks rt
		LEFT JOIN participants p ON p.id = rt.created_by
		LEFT JOIN communities c ON c.id = rt.community_id
		WHERE rt.id = $1`, taskID,
	).Scan(
		&t.ID, &t.PostID, &t.CommunityID, &t.Status, &t.Question,
		&t.SynthesisPostID, &t.MaxInvestigators, &t.Deadline,
		&t.CreatedBy, &t.CreatedByName,
		&t.CommunityName, &t.CommunitySlug,
		&t.ContributionCount,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get research task: %w", err)
	}
	return &t, nil
}

// ListTasks returns research tasks filtered by community and status.
func (r *ResearchRepo) ListTasks(ctx context.Context, communityID, status string, limit, offset int) ([]ResearchTask, int, error) {
	conditions := []string{}
	args := []any{}
	argIdx := 1

	if communityID != "" {
		conditions = append(conditions, fmt.Sprintf("rt.community_id = $%d", argIdx))
		args = append(args, communityID)
		argIdx++
	}

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("rt.status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE "
		for i, cond := range conditions {
			if i > 0 {
				whereClause += " AND "
			}
			whereClause += cond
		}
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM research_tasks rt " + whereClause
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count research tasks: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT rt.id, rt.post_id, rt.community_id, rt.status, rt.question,
		       rt.synthesis_post_id, rt.max_investigators, rt.deadline,
		       rt.created_by, COALESCE(p.display_name, '') as created_by_name,
		       COALESCE(c.name, '') as community_name,
		       COALESCE(c.slug, '') as community_slug,
		       (SELECT COUNT(*) FROM research_contributions rc WHERE rc.research_task_id = rt.id) as contribution_count,
		       rt.created_at, rt.updated_at
		FROM research_tasks rt
		LEFT JOIN participants p ON p.id = rt.created_by
		LEFT JOIN communities c ON c.id = rt.community_id
		%s
		ORDER BY rt.created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list research tasks: %w", err)
	}
	defer rows.Close()

	var tasks []ResearchTask
	for rows.Next() {
		var t ResearchTask
		if err := rows.Scan(
			&t.ID, &t.PostID, &t.CommunityID, &t.Status, &t.Question,
			&t.SynthesisPostID, &t.MaxInvestigators, &t.Deadline,
			&t.CreatedBy, &t.CreatedByName,
			&t.CommunityName, &t.CommunitySlug,
			&t.ContributionCount,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan research task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate research tasks: %w", err)
	}

	if tasks == nil {
		tasks = []ResearchTask{}
	}
	return tasks, total, nil
}

// Contribute adds a contribution to a research task.
func (r *ResearchRepo) Contribute(ctx context.Context, taskID, contributorID, postID string) (*ResearchContribution, error) {
	var c ResearchContribution
	err := r.pool.QueryRow(ctx, `
		INSERT INTO research_contributions (research_task_id, contributor_id, post_id)
		VALUES ($1, $2, $3)
		RETURNING id, research_task_id, contributor_id, post_id, status, created_at`,
		taskID, contributorID, postID,
	).Scan(
		&c.ID, &c.ResearchTaskID, &c.ContributorID, &c.PostID, &c.Status, &c.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create research contribution: %w", err)
	}

	// Auto-update task status to investigating if it was open
	_, _ = r.pool.Exec(ctx, `
		UPDATE research_tasks SET status = 'investigating', updated_at = NOW()
		WHERE id = $1 AND status = 'open'`, taskID)

	return &c, nil
}

// ListContributions returns all contributions for a research task.
func (r *ResearchRepo) ListContributions(ctx context.Context, taskID string) ([]ResearchContribution, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT rc.id, rc.research_task_id, rc.contributor_id,
		       COALESCE(p.display_name, '') as contributor_name,
		       rc.post_id, rc.status, rc.created_at
		FROM research_contributions rc
		LEFT JOIN participants p ON p.id = rc.contributor_id
		WHERE rc.research_task_id = $1
		ORDER BY rc.created_at ASC`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list research contributions: %w", err)
	}
	defer rows.Close()

	var contributions []ResearchContribution
	for rows.Next() {
		var c ResearchContribution
		if err := rows.Scan(
			&c.ID, &c.ResearchTaskID, &c.ContributorID, &c.ContributorName,
			&c.PostID, &c.Status, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan research contribution: %w", err)
		}
		contributions = append(contributions, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate research contributions: %w", err)
	}

	if contributions == nil {
		contributions = []ResearchContribution{}
	}
	return contributions, nil
}

// UpdateStatus updates the status of a research task.
func (r *ResearchRepo) UpdateStatus(ctx context.Context, taskID, status string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE research_tasks SET status = $1, updated_at = NOW()
		WHERE id = $2`, status, taskID)
	if err != nil {
		return fmt.Errorf("update research task status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("research task not found")
	}
	return nil
}

// SetSynthesis links the final synthesis post to the research task and marks it completed.
func (r *ResearchRepo) SetSynthesis(ctx context.Context, taskID, synthesisPostID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE research_tasks SET synthesis_post_id = $1, status = 'completed', updated_at = NOW()
		WHERE id = $2`, synthesisPostID, taskID)
	if err != nil {
		return fmt.Errorf("set research synthesis: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("research task not found")
	}
	return nil
}
