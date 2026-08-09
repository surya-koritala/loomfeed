package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RoamXAI/loomfeed/internal/models"
)

// LoomRepo persists loom_summons rows. The table doubles as audit
// log, rate-limit counter, and worker queue — see migration 000079
// for the rationale. CRUD here is intentionally narrow; aggregation
// queries (cost-per-day, rate-limit-window) live in the call sites
// that need them so the repo doesn't grow a metrics surface.
type LoomRepo struct {
	pool *pgxpool.Pool
}

func NewLoomRepo(pool *pgxpool.Pool) *LoomRepo {
	return &LoomRepo{pool: pool}
}

// CreateSummonParams is the minimum a caller needs to record a new
// summon. Optional pointers stay nil when the summon has no
// originating post / comment (e.g. anon "ask the looms" landing).
type CreateSummonParams struct {
	ParticipantID *string
	PostID        *string
	CommentID     *string
	Intent        models.LoomIntent
	Prompt        string
}

// CreateSummon inserts a pending row and returns its ID. The worker
// picks it up via idx_loom_summons_pending.
func (r *LoomRepo) CreateSummon(ctx context.Context, p CreateSummonParams) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO loom_summons (participant_id, post_id, comment_id, intent, prompt, state)
		VALUES ($1, $2, $3, $4, $5, 'pending')
		RETURNING id`,
		p.ParticipantID, p.PostID, p.CommentID, p.Intent, p.Prompt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert loom summon: %w", err)
	}
	return id, nil
}

// GetSummon returns one row by ID. Used by the polling endpoint that
// the frontend hits while waiting for the reply.
func (r *LoomRepo) GetSummon(ctx context.Context, id string) (*models.LoomSummon, error) {
	var s models.LoomSummon
	err := r.pool.QueryRow(ctx, `
		SELECT id, participant_id, post_id, comment_id, reply_comment_id,
		       intent, prompt, response, model, input_tokens, output_tokens,
		       cost_usd, cached, state, error_code, latency_ms,
		       created_at, finished_at
		FROM loom_summons
		WHERE id = $1`, id,
	).Scan(
		&s.ID, &s.ParticipantID, &s.PostID, &s.CommentID, &s.ReplyCommentID,
		&s.Intent, &s.Prompt, &s.Response, &s.Model, &s.InputTokens, &s.OutputTokens,
		&s.CostUSD, &s.Cached, &s.State, &s.ErrorCode, &s.LatencyMs,
		&s.CreatedAt, &s.FinishedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get loom summon: %w", err)
	}
	return &s, nil
}

// LatestPostCardForPost returns the most recent summon for a post,
// regardless of whether it was the new "post-card" shape
// (reply_comment_id IS NULL) or the legacy "comment-reply" shape
// (reply_comment_id IS NOT NULL — the worker also posted a Loom-
// authored child comment, since hidden from render). Both shapes
// stored the AI response on loom_summons.response, so for the card
// UI they're interchangeable.
//
// State: we return whatever the latest row is (pending / done /
// error). The previous version filtered to state='done' which broke
// the Ask Loom flow — the frontend, after POSTing a fresh summon,
// would poll this endpoint, see state='none' (because the new
// pending row was hidden), assume nothing was in flight, and stop
// polling. The worker would finish minutes later with no one
// watching. Returning the pending row lets the frontend show the
// "Summoning…" state and keep polling until state becomes done or
// error.
//
// Returns (nil, nil) when no summon exists for the post yet — that's
// not an error, it's a "no card to show" signal.
func (r *LoomRepo) LatestPostCardForPost(ctx context.Context, postID string) (*models.LoomSummon, error) {
	var s models.LoomSummon
	err := r.pool.QueryRow(ctx, `
		SELECT id, participant_id, post_id, comment_id, reply_comment_id,
		       intent, prompt, response, model, input_tokens, output_tokens,
		       cost_usd, cached, state, error_code, latency_ms,
		       created_at, finished_at
		FROM loom_summons
		WHERE post_id = $1
		ORDER BY created_at DESC
		LIMIT 1`, postID,
	).Scan(
		&s.ID, &s.ParticipantID, &s.PostID, &s.CommentID, &s.ReplyCommentID,
		&s.Intent, &s.Prompt, &s.Response, &s.Model, &s.InputTokens, &s.OutputTokens,
		&s.CostUSD, &s.Cached, &s.State, &s.ErrorCode, &s.LatencyMs,
		&s.CreatedAt, &s.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("latest post-card summon: %w", err)
	}
	return &s, nil
}

// FinalizeParams collects the fields the worker writes back once a
// summon has produced a reply. Grouping them in a struct keeps the
// Finalize signature stable as new fields appear (e.g. cache_hit_id
// when we add deduplication across summons).
type FinalizeParams struct {
	ReplyCommentID *string
	Response       string
	Model          string
	InputTokens    int
	OutputTokens   int
	CostUSD        float64
	Cached         bool
	LatencyMs      int
}

// FinalizeSummon flips state to 'done' and writes the reply + cost.
// Called by the worker on the happy path.
func (r *LoomRepo) FinalizeSummon(ctx context.Context, id string, p FinalizeParams) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE loom_summons
		SET state = 'done',
		    reply_comment_id = $2,
		    response = $3,
		    model = $4,
		    input_tokens = $5,
		    output_tokens = $6,
		    cost_usd = $7,
		    cached = $8,
		    latency_ms = $9,
		    finished_at = NOW()
		WHERE id = $1`,
		id, p.ReplyCommentID, p.Response, p.Model,
		p.InputTokens, p.OutputTokens, p.CostUSD, p.Cached, p.LatencyMs,
	)
	if err != nil {
		return fmt.Errorf("finalize loom summon: %w", err)
	}
	return nil
}

// MarkErrored flips state to 'error' and records why. The summon row
// stays around for debugging + rate-limit accounting — we do not
// refund a rate-limit slot on error, since hammering a failing summon
// is itself a cost.
func (r *LoomRepo) MarkErrored(ctx context.Context, id, errorCode string, latencyMs int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE loom_summons
		SET state = 'error',
		    error_code = $2,
		    latency_ms = $3,
		    finished_at = NOW()
		WHERE id = $1`,
		id, errorCode, latencyMs,
	)
	if err != nil {
		return fmt.Errorf("mark loom summon errored: %w", err)
	}
	return nil
}

// CountRecentByParticipant returns the number of summons a participant
// has made since `since`. Drives the daily rate-limit check.
func (r *LoomRepo) CountRecentByParticipant(ctx context.Context, participantID string, since time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM loom_summons
		WHERE participant_id = $1 AND created_at >= $2`,
		participantID, since,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count recent loom summons: %w", err)
	}
	return n, nil
}
