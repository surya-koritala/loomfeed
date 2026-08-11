package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

var (
	ErrPredictionPostNotFound    = errors.New("prediction post not found")
	ErrPredictionPostNotOwned    = errors.New("prediction post not owned")
	ErrPredictionNotFound        = errors.New("prediction not found")
	ErrPredictionNotOwned        = errors.New("prediction not owned")
	ErrPredictionNotResolvable   = errors.New("prediction is not yet resolvable")
	ErrPredictionAlreadyResolved = errors.New("prediction already has a different resolution")
	ErrInvalidPrediction         = errors.New("invalid prediction")
)

// PredictionRepo owns post-attached generic predictions. The sports repository
// writes the same predictions table through its match-specific columns.
type PredictionRepo struct {
	pool *pgxpool.Pool
}

func NewPredictionRepo(pool *pgxpool.Pool) *PredictionRepo {
	return &PredictionRepo{pool: pool}
}

const predictionColumns = `id,
	COALESCE(post_id::text, ''), COALESCE(match_id::text, ''),
	participant_id::text, predictor_kind, subject, predicted_outcome,
	confidence::float8, resolve_by, resolution, outcome, brier::float8,
	reasoning, created_at, updated_at, resolved_at`

const qualifiedPredictionColumns = `predictions.id,
	COALESCE(predictions.post_id::text, ''), COALESCE(predictions.match_id::text, ''),
	predictions.participant_id::text, predictions.predictor_kind,
	predictions.subject, predictions.predicted_outcome,
	predictions.confidence::float8, predictions.resolve_by,
	predictions.resolution, predictions.outcome, predictions.brier::float8,
	predictions.reasoning, predictions.created_at, predictions.updated_at,
	predictions.resolved_at`

func scanPrediction(row pgx.Row) (*models.Prediction, error) {
	var p models.Prediction
	if err := row.Scan(
		&p.ID, &p.PostID, &p.MatchID, &p.ParticipantID, &p.PredictorKind,
		&p.Subject, &p.PredictedOutcome, &p.Confidence, &p.ResolveBy,
		&p.Resolution, &p.Outcome, &p.Brier, &p.Reasoning,
		&p.CreatedAt, &p.UpdatedAt, &p.ResolvedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

func validPredictionInput(p *models.Prediction) bool {
	if p == nil || p.PostID == "" || p.ParticipantID == "" || p.ResolveBy.IsZero() {
		return false
	}
	if p.PredictorKind != "agent" && p.PredictorKind != "human" {
		return false
	}
	if strings.TrimSpace(p.Subject) == "" || strings.TrimSpace(p.PredictedOutcome) == "" {
		return false
	}
	return p.Confidence >= 0 && p.Confidence <= 1 && !math.IsNaN(p.Confidence) && !math.IsInf(p.Confidence, 0)
}

// UpsertPostPrediction creates or revises the post author's one prediction on
// that post. Both insert and update are locked atomically at resolve_by; a
// caller cannot extend or change a prediction after its original deadline.
func (r *PredictionRepo) UpsertPostPrediction(ctx context.Context, p *models.Prediction) (*models.Prediction, error) {
	if !validPredictionInput(p) {
		return nil, ErrInvalidPrediction
	}
	p.Subject = strings.TrimSpace(p.Subject)
	p.PredictedOutcome = strings.TrimSpace(p.PredictedOutcome)
	p.Reasoning = strings.TrimSpace(p.Reasoning)

	stored, err := scanPrediction(r.pool.QueryRow(ctx, `
		INSERT INTO predictions
			(post_id, participant_id, predictor_kind, subject, predicted_outcome,
			 confidence, resolve_by, reasoning)
		SELECT p.id, $2, $3, $4, $5, $6, $7, $8
		FROM posts p
		WHERE p.id = $1 AND p.author_id = $2 AND p.deleted_at IS NULL
		  AND $7::timestamptz > NOW()
		ON CONFLICT (post_id, participant_id) DO UPDATE SET
			predictor_kind = EXCLUDED.predictor_kind,
			subject = EXCLUDED.subject,
			predicted_outcome = EXCLUDED.predicted_outcome,
			confidence = EXCLUDED.confidence,
			resolve_by = EXCLUDED.resolve_by,
			reasoning = EXCLUDED.reasoning,
			updated_at = NOW()
		WHERE predictions.resolve_by > NOW()
		  AND predictions.resolution IS NULL
		  AND EXCLUDED.resolve_by > NOW()
		RETURNING `+predictionColumns,
		p.PostID, p.ParticipantID, p.PredictorKind, p.Subject,
		p.PredictedOutcome, p.Confidence, p.ResolveBy, p.Reasoning,
	))
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("upsert post prediction: %w", err)
	}

	var authorID string
	err = r.pool.QueryRow(ctx, `
		SELECT author_id FROM posts WHERE id = $1 AND deleted_at IS NULL`, p.PostID,
	).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPredictionPostNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("check prediction post: %w", err)
	}
	if authorID != p.ParticipantID {
		return nil, ErrPredictionPostNotOwned
	}
	return nil, ErrPredictionLocked
}

// GetPrediction returns one prediction regardless of subject source.
func (r *PredictionRepo) GetPrediction(ctx context.Context, id string) (*models.Prediction, error) {
	p, err := scanPrediction(r.pool.QueryRow(ctx, `
		SELECT `+predictionColumns+` FROM predictions WHERE id = $1`, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPredictionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get prediction: %w", err)
	}
	return p, nil
}

// ListPostPredictions returns post-attached predictions newest first with the
// predictor's display name and aggregate track record.
func (r *PredictionRepo) ListPostPredictions(ctx context.Context, postID string, limit, offset int) ([]models.Prediction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+qualifiedPredictionColumns+`,
		       COALESCE(part.display_name, ''),
		       COALESCE(st.n, 0), COALESCE(st.correct, 0),
		       CASE WHEN COALESCE(st.n, 0) > 0 THEN (st.brier_sum / st.n)::float8 ELSE 0 END
		FROM predictions
		JOIN participants part ON part.id = predictions.participant_id
		LEFT JOIN prediction_stats st ON st.participant_id = predictions.participant_id
		WHERE post_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, postID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list post predictions: %w", err)
	}
	defer rows.Close()

	result := make([]models.Prediction, 0)
	for rows.Next() {
		var p models.Prediction
		if err := rows.Scan(
			&p.ID, &p.PostID, &p.MatchID, &p.ParticipantID, &p.PredictorKind,
			&p.Subject, &p.PredictedOutcome, &p.Confidence, &p.ResolveBy,
			&p.Resolution, &p.Outcome, &p.Brier, &p.Reasoning,
			&p.CreatedAt, &p.UpdatedAt, &p.ResolvedAt,
			&p.DisplayName, &p.StatsN, &p.StatsCorrect, &p.StatsAvgBrier,
		); err != nil {
			return nil, fmt.Errorf("scan post prediction: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate post predictions: %w", err)
	}
	return result, nil
}

// ResolvePrediction grades an owned prediction after resolve_by. Matching is
// trimmed and case-insensitive so stable labels such as "Yes" and "yes" are
// equivalent. The binary Brier score is (confidence - observed)^2. Repeating
// the same resolution is a no-op; changing it is rejected.
func (r *PredictionRepo) ResolvePrediction(ctx context.Context, id, participantID, resolution string) (*models.Prediction, bool, error) {
	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		return nil, false, ErrInvalidPrediction
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin prediction resolution: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	p, err := scanPrediction(tx.QueryRow(ctx, `
		SELECT `+predictionColumns+` FROM predictions WHERE id = $1 FOR UPDATE`, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrPredictionNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("lock prediction: %w", err)
	}
	if p.ParticipantID != participantID {
		return nil, false, ErrPredictionNotOwned
	}
	if p.Resolution != nil {
		if strings.EqualFold(strings.TrimSpace(*p.Resolution), resolution) {
			return p, false, nil
		}
		return nil, false, ErrPredictionAlreadyResolved
	}

	var due bool
	if err := tx.QueryRow(ctx, `SELECT resolve_by <= NOW() FROM predictions WHERE id = $1`, id).Scan(&due); err != nil {
		return nil, false, fmt.Errorf("check prediction deadline: %w", err)
	}
	if !due {
		return nil, false, ErrPredictionNotResolvable
	}

	correct := strings.EqualFold(strings.TrimSpace(p.PredictedOutcome), resolution)
	observed := 0.0
	outcome := "wrong"
	if correct {
		observed = 1
		outcome = "correct"
	}
	brier := math.Pow(p.Confidence-observed, 2)

	resolved, err := scanPrediction(tx.QueryRow(ctx, `
		UPDATE predictions SET
			resolution = $2,
			outcome = $3,
			brier = $4,
			resolved_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
		RETURNING `+predictionColumns,
		id, resolution, outcome, brier,
	))
	if err != nil {
		return nil, false, fmt.Errorf("resolve prediction: %w", err)
	}

	correctInt := 0
	streak := -1
	if correct {
		correctInt = 1
		streak = 1
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO prediction_stats
			(participant_id, predictor_kind, n, correct, brier_sum, streak)
		VALUES ($1, $2, 1, $3, $4, $5)
		ON CONFLICT (participant_id) DO UPDATE SET
			predictor_kind = EXCLUDED.predictor_kind,
			n = prediction_stats.n + 1,
			correct = prediction_stats.correct + EXCLUDED.correct,
			brier_sum = prediction_stats.brier_sum + EXCLUDED.brier_sum,
			streak = CASE WHEN EXCLUDED.correct = 1
			              THEN GREATEST(prediction_stats.streak, 0) + 1
			              ELSE LEAST(prediction_stats.streak, 0) - 1 END,
			updated_at = NOW()`,
		p.ParticipantID, p.PredictorKind, correctInt, brier, streak,
	)
	if err != nil {
		return nil, false, fmt.Errorf("update prediction stats: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit prediction resolution: %w", err)
	}
	return resolved, true, nil
}
