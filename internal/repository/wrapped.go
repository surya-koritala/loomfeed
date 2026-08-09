package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WrappedRepo assembles a participant's "Year in Review" — a single
// roll-up of their activity for a given calendar year. Every metric is
// scoped to `created_at >= year-01-01 AND < (year+1)-01-01` except for
// trust_score which is sampled at the window boundaries from
// reputation_events.
type WrappedRepo struct {
	pool *pgxpool.Pool
}

func NewWrappedRepo(pool *pgxpool.Pool) *WrappedRepo {
	return &WrappedRepo{pool: pool}
}

type WrappedPost struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	CommunitySlug string    `json:"community_slug"`
	VoteScore     int       `json:"vote_score"`
	CommentCount  int       `json:"comment_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type WrappedCommunity struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	PostCount int    `json:"post_count"`
}

type WrappedParticipant struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

type WrappedSummary struct {
	Participant            WrappedParticipant `json:"participant"`
	Year                   int                `json:"year"`
	PostsPublished         int                `json:"posts_published"`
	CommentsPosted         int                `json:"comments_posted"`
	TotalPostVoteScore     int                `json:"total_post_vote_score"`
	TotalReactionsReceived int                `json:"total_reactions_received"`
	CommunitiesActiveIn    int                `json:"communities_active_in"`
	CitationsIn            int                `json:"citations_in"`
	TrustScoreStart        float64            `json:"trust_score_start"`
	TrustScoreEnd          float64            `json:"trust_score_end"`
	TopPosts               []WrappedPost      `json:"top_posts"`
	TopCommunities         []WrappedCommunity `json:"top_communities"`
}

// yearBounds returns [start, end) for a calendar year, in UTC.
func yearBounds(year int) (time.Time, time.Time) {
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
	return start, end
}

// Build loads every metric for (participantID, year). Returns a nil
// *WrappedSummary and a not-found-style error if the participant
// doesn't exist; returns a populated summary with zeroed counts if
// they exist but had no activity in the window.
func (r *WrappedRepo) Build(ctx context.Context, participantID string, year int) (*WrappedSummary, error) {
	start, end := yearBounds(year)

	// Participant metadata first — this is the lookup that 404s if the
	// id is bogus.
	var p WrappedParticipant
	err := r.pool.QueryRow(ctx,
		`SELECT id, display_name, type, COALESCE(avatar_url, '')
         FROM participants WHERE id = $1`, participantID).
		Scan(&p.ID, &p.DisplayName, &p.Type, &p.AvatarURL)
	if err != nil {
		return nil, fmt.Errorf("wrapped: participant lookup: %w", err)
	}

	sum := &WrappedSummary{
		Participant:    p,
		Year:           year,
		TopPosts:       []WrappedPost{},
		TopCommunities: []WrappedCommunity{},
	}

	// Posts published + total vote score.
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(vote_score), 0)
         FROM posts
         WHERE author_id = $1
           AND deleted_at IS NULL
           AND created_at >= $2 AND created_at < $3`,
		participantID, start, end).
		Scan(&sum.PostsPublished, &sum.TotalPostVoteScore)
	if err != nil {
		return nil, fmt.Errorf("wrapped: post counts: %w", err)
	}

	// Comments posted.
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM comments
         WHERE author_id = $1
           AND deleted_at IS NULL
           AND created_at >= $2 AND created_at < $3`,
		participantID, start, end).
		Scan(&sum.CommentsPosted)
	if err != nil {
		return nil, fmt.Errorf("wrapped: comment count: %w", err)
	}

	// Reactions received on the participant's posts + comments in-window.
	// Scoped to reactions created in the same year so "received" tracks
	// engagement during the year even if the content is older.
	err = r.pool.QueryRow(ctx, `
        SELECT COUNT(*) FROM reactions rx
        WHERE rx.created_at >= $2 AND rx.created_at < $3
          AND (
            (rx.post_id    IS NOT NULL AND EXISTS (SELECT 1 FROM posts    p WHERE p.id = rx.post_id    AND p.author_id = $1))
         OR (rx.comment_id IS NOT NULL AND EXISTS (SELECT 1 FROM comments c WHERE c.id = rx.comment_id AND c.author_id = $1))
          )`,
		participantID, start, end).
		Scan(&sum.TotalReactionsReceived)
	if err != nil {
		// Reactions table shape might vary across environments; treat
		// a query error here as "zero" rather than failing the whole
		// summary. The count is decorative, not load-bearing.
		sum.TotalReactionsReceived = 0
	}

	// Communities active in (distinct community_id across their posts).
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT community_id) FROM posts
         WHERE author_id = $1
           AND deleted_at IS NULL
           AND created_at >= $2 AND created_at < $3`,
		participantID, start, end).
		Scan(&sum.CommunitiesActiveIn)
	if err != nil {
		return nil, fmt.Errorf("wrapped: communities count: %w", err)
	}

	// Citations into the participant's posts, if citations exists.
	// Best-effort; zero on any error (schema differs across envs).
	_ = r.pool.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM citations ci
        JOIN posts p ON p.id = ci.cited_post_id
        WHERE p.author_id = $1
          AND ci.created_at >= $2 AND ci.created_at < $3`,
		participantID, start, end).
		Scan(&sum.CitationsIn)

	// Top 5 posts by vote_score in-window.
	rows, err := r.pool.Query(ctx, `
        SELECT p.id, p.title, COALESCE(c.slug, ''), COALESCE(p.vote_score, 0),
               COALESCE(p.comment_count, 0), p.created_at
        FROM posts p
        LEFT JOIN communities c ON c.id = p.community_id
        WHERE p.author_id = $1
          AND p.deleted_at IS NULL
          AND p.created_at >= $2 AND p.created_at < $3
        ORDER BY p.vote_score DESC, p.created_at DESC
        LIMIT 5`, participantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("wrapped: top posts: %w", err)
	}
	for rows.Next() {
		var wp WrappedPost
		if err := rows.Scan(&wp.ID, &wp.Title, &wp.CommunitySlug, &wp.VoteScore, &wp.CommentCount, &wp.CreatedAt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("wrapped: scan top post: %w", err)
		}
		sum.TopPosts = append(sum.TopPosts, wp)
	}
	rows.Close()

	// Top 5 communities by post count in-window.
	rows, err = r.pool.Query(ctx, `
        SELECT c.slug, c.name, COUNT(*) AS n
        FROM posts p
        JOIN communities c ON c.id = p.community_id
        WHERE p.author_id = $1
          AND p.deleted_at IS NULL
          AND p.created_at >= $2 AND p.created_at < $3
        GROUP BY c.slug, c.name
        ORDER BY n DESC
        LIMIT 5`, participantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("wrapped: top communities: %w", err)
	}
	for rows.Next() {
		var wc WrappedCommunity
		if err := rows.Scan(&wc.Slug, &wc.Name, &wc.PostCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("wrapped: scan top community: %w", err)
		}
		sum.TopCommunities = append(sum.TopCommunities, wc)
	}
	rows.Close()

	// Trust score at year boundaries. reputation_events carries the
	// running delta; sum everything up to `end` for end-of-year, and
	// everything before `start` for start-of-year. Best-effort on
	// schema variance.
	_ = r.pool.QueryRow(ctx, `
        SELECT COALESCE(SUM(score_delta), 0) FROM reputation_events
        WHERE participant_id = $1 AND created_at < $2`,
		participantID, start).
		Scan(&sum.TrustScoreStart)
	_ = r.pool.QueryRow(ctx, `
        SELECT COALESCE(SUM(score_delta), 0) FROM reputation_events
        WHERE participant_id = $1 AND created_at < $2`,
		participantID, end).
		Scan(&sum.TrustScoreEnd)

	return sum, nil
}
