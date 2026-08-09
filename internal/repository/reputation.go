package repository

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reputation event types. New event names use the post_*/correction_*
// shape introduced with the uncapped rep system in migration 000062.
// The legacy aliases (upvote_received, content_verified, etc.) are
// preserved for backwards compat with existing call sites until they
// migrate.
const (
	// New uncapped-system events
	EventPostSupported          = "post_supported"
	EventPostRefuted            = "post_refuted"
	EventPostContested          = "post_contested"
	EventCorrectionAcknowledged = "correction_acknowledged"
	EventVoteReceived           = "vote_received"

	// Legacy events (kept for compat with existing handlers)
	EventUpvoteReceived   = "upvote_received"
	EventDownvoteReceived = "downvote_received"
	EventAcceptedAnswer   = "accepted_answer"
	EventFlagUpheld       = "flag_upheld"
	EventAgentEndorsed    = "agent_endorsed"
	EventContentVerified  = "content_verified"
	EventInviteeSignedUp  = "invitee_signed_up"
)

// Baseline reputation for any participant with no events. Sits at 100
// (neutral) rather than 0 so a brand-new agent doesn't read as
// "destroyed reputation". Reputation can fall below 100 (and below 0
// floors at 0) but everyone starts here.
const ReputationBaseline = 100.0

// baseEventValue returns the unscaled magnitude of a reputation event.
// Negative values mean the event hurts reputation. Positive helps.
//
// Asymmetry is intentional: penalties don't get easier as rep grows
// (a refuted post costs a 5000-rep agent the same as a 50-rep agent),
// but gains diminish at high rep via difficultyMultiplier(). A
// 5000-rep agent who gets refuted has a long way to fall — that's how
// the score stays honest at the top.
func baseEventValue(eventType string) float64 {
	switch eventType {
	// New events
	case EventPostSupported:
		return 50 // humans marked the post supported (≥5 verifications)
	case EventPostRefuted:
		return -100 // humans refuted the post — heavy, never softens
	case EventPostContested:
		return -15
	case EventCorrectionAcknowledged:
		return 10 // agent owned a correction within 24h
	case EventVoteReceived:
		return 1 // single human upvote on author's post

	// Legacy events
	case EventUpvoteReceived:
		return 1
	case EventDownvoteReceived:
		return -1
	case EventAcceptedAnswer:
		return 5
	case EventContentVerified:
		return 25 // historical "verified" — between supported and a single vote
	case EventAgentEndorsed:
		return 5
	case EventFlagUpheld:
		return -50
	case EventInviteeSignedUp:
		return 5
	}
	return 0
}

// difficultyMultiplier shrinks gain magnitude as current reputation
// grows. Penalties are NOT reduced by this — only positive deltas. The
// curve is gentle: 1.0× at baseline (100), 0.5× at 1000 rep, 0.2× at
// 5000, 0.1× at 10000.
//
// The formula is 100 / (rep + 100) so it never hits zero — even a
// 50000-rep agent still gains something. It just takes a lot more
// events.
func difficultyMultiplier(currentRep float64) float64 {
	if currentRep < ReputationBaseline {
		return 1.0
	}
	return ReputationBaseline / (currentRep + ReputationBaseline) * 2.0
}

// dailyGainCap limits how much rep a participant can gain in a single
// 24h window. Prevents a viral post + sock-puppet swarm from inflating
// rep into the stratosphere overnight. No cap on losses.
const dailyGainCap = 200.0

// ApplyReputationEventTx records a reputation event and updates
// participant.reputation_score atomically inside the supplied
// transaction. Use this from any handler that already owns a tx
// (e.g. vote casting) so reputation stays consistent with the
// triggering write.
//
// Returns the new uncapped reputation_score.
func ApplyReputationEventTx(ctx context.Context, tx pgx.Tx, participantID, eventType string) (float64, error) {
	canonical := baseEventValue(eventType)
	if canonical == 0 {
		// Unknown event type — nothing to apply, but still record so
		// history is complete.
		_, err := tx.Exec(ctx,
			`INSERT INTO reputation_events (participant_id, event_type, score_delta)
             VALUES ($1, $2, 0)`, participantID, eventType)
		if err != nil {
			return 0, fmt.Errorf("insert no-op rep event: %w", err)
		}
		var current float64
		err = tx.QueryRow(ctx,
			`SELECT COALESCE(reputation_score, $1) FROM participants WHERE id = $2`,
			ReputationBaseline, participantID).Scan(&current)
		return current, err
	}

	var currentRep float64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(reputation_score, $1) FROM participants WHERE id = $2`,
		ReputationBaseline, participantID).Scan(&currentRep); err != nil {
		return 0, fmt.Errorf("read current rep: %w", err)
	}

	delta := canonical
	if delta > 0 {
		delta = delta * difficultyMultiplier(currentRep)
		var gainedToday float64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(score_delta), 0)
			FROM reputation_events
			WHERE participant_id = $1
			  AND created_at > NOW() - INTERVAL '24 hours'
			  AND score_delta > 0`,
			participantID).Scan(&gainedToday); err != nil {
			return 0, fmt.Errorf("query daily gains: %w", err)
		}
		remaining := dailyGainCap - gainedToday
		if remaining <= 0 {
			delta = 0
		} else if delta > remaining {
			delta = remaining
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO reputation_events (participant_id, event_type, score_delta)
         VALUES ($1, $2, $3)`,
		participantID, eventType, delta); err != nil {
		return 0, fmt.Errorf("insert reputation event: %w", err)
	}

	newRep := math.Max(0, currentRep+delta)
	if _, err := tx.Exec(ctx,
		`UPDATE participants SET reputation_score = $1 WHERE id = $2`,
		newRep, participantID); err != nil {
		return 0, fmt.Errorf("update reputation score: %w", err)
	}

	return newRep, nil
}

type ReputationRepo struct {
	pool *pgxpool.Pool
}

func NewReputationRepo(pool *pgxpool.Pool) *ReputationRepo {
	return &ReputationRepo{pool: pool}
}

// RecordEvent inserts a reputation event and updates the participant's
// reputation_score using the uncapped formula. The legacy `scoreDelta`
// argument is now ignored — the canonical magnitude lives in
// baseEventValue() so all call sites behave consistently. The
// signature is preserved so existing callers don't need to change.
func (r *ReputationRepo) RecordEvent(ctx context.Context, participantID, eventType string, _ float64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := ApplyReputationEventTx(ctx, tx, participantID, eventType); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Recalculate rebuilds reputation_score for a participant from scratch
// by replaying their event history under the current formula. Useful
// for the one-shot recalc after migrating to the uncapped system, and
// for any future changes to event values.
func (r *ReputationRepo) Recalculate(ctx context.Context, participantID string) error {
	rows, err := r.pool.Query(ctx, `
		SELECT event_type, score_delta, created_at
		FROM reputation_events
		WHERE participant_id = $1
		ORDER BY created_at ASC`, participantID)
	if err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	defer rows.Close()

	rep := ReputationBaseline
	dayBucket := make(map[string]float64) // YYYY-MM-DD -> gained today
	for rows.Next() {
		var eventType string
		var oldDelta float64
		var createdAt time.Time
		if err := rows.Scan(&eventType, &oldDelta, &createdAt); err != nil {
			return err
		}

		base := baseEventValue(eventType)
		if base == 0 {
			base = oldDelta
		}

		applied := base
		if applied > 0 {
			applied = applied * difficultyMultiplier(rep)
			day := createdAt.UTC().Format("2006-01-02")
			gained := dayBucket[day]
			remaining := dailyGainCap - gained
			if remaining <= 0 {
				applied = 0
			} else if applied > remaining {
				applied = remaining
			}
			dayBucket[day] = gained + applied
		}

		rep = math.Max(0, rep+applied)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx,
		`UPDATE participants SET reputation_score = $1 WHERE id = $2`,
		rep, participantID)
	return err
}

// GetHistory returns recent reputation events for a participant.
func (r *ReputationRepo) GetHistory(ctx context.Context, participantID string, limit int) ([]ReputationEvent, error) {
	return r.GetHistoryFiltered(ctx, participantID, "", limit)
}

// GetHistoryFiltered returns reputation events newest-first, optionally
// constrained to a single event_type. eventType "" returns everything;
// the same query gets reused so we don't fan out a parallel codepath.
func (r *ReputationRepo) GetHistoryFiltered(ctx context.Context, participantID, eventType string, limit int) ([]ReputationEvent, error) {
	const baseSQL = `SELECT id, participant_id, event_type, score_delta, created_at
         FROM reputation_events
         WHERE participant_id = $1`
	var (
		rows pgx.Rows
		err  error
	)
	if eventType == "" {
		rows, err = r.pool.Query(ctx, baseSQL+` ORDER BY created_at DESC LIMIT $2`, participantID, limit)
	} else {
		rows, err = r.pool.Query(ctx, baseSQL+` AND event_type = $2 ORDER BY created_at DESC LIMIT $3`, participantID, eventType, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list reputation events: %w", err)
	}
	defer rows.Close()

	events := []ReputationEvent{}
	for rows.Next() {
		var e ReputationEvent
		if err := rows.Scan(&e.ID, &e.ParticipantID, &e.EventType, &e.ScoreDelta, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

type ReputationEvent struct {
	ID            string    `json:"id"`
	ParticipantID string    `json:"participant_id"`
	EventType     string    `json:"event_type"`
	ScoreDelta    float64   `json:"score_delta"`
	CreatedAt     time.Time `json:"created_at"`
}
