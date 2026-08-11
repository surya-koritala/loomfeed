package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// ErrPredictionLocked is returned when a prediction is submitted at or after kickoff.
var ErrPredictionLocked = errors.New("prediction window closed")

// ErrSportsMatchNotFound is returned when the referenced match does not exist.
var ErrSportsMatchNotFound = errors.New("match not found")

// SportsRepo handles database operations for World Cup match predictions.
type SportsRepo struct {
	pool *pgxpool.Pool
}

// NewSportsRepo creates a new SportsRepo.
func NewSportsRepo(pool *pgxpool.Pool) *SportsRepo {
	return &SportsRepo{pool: pool}
}

// sportsMatchColumns is shared by every match query; all consumers select
// FROM sports_matches unaliased, so the correlated prediction-count subquery
// qualifies with the full table name.
const sportsMatchColumns = `id, ext_id, competition, stage, group_name,
       home_team, home_code, home_crest, away_team, away_code, away_crest,
       kickoff_utc, status, home_score, away_score, venue, settled_at,
       espn_event_id, lineups,
       (SELECT COUNT(*) FROM predictions sp
         WHERE sp.match_id = sports_matches.id AND sp.predictor_kind = 'agent')::int AS prediction_count`

func scanSportsMatch(row pgx.Row, m *models.SportsMatch) error {
	return row.Scan(
		&m.ID, &m.ExtID, &m.Competition, &m.Stage, &m.GroupName,
		&m.HomeTeam, &m.HomeCode, &m.HomeCrest, &m.AwayTeam, &m.AwayCode, &m.AwayCrest,
		&m.KickoffUTC, &m.Status, &m.HomeScore, &m.AwayScore, &m.Venue, &m.SettledAt,
		&m.ESPNEventID, &m.Lineups, &m.PredictionCount,
	)
}

// UpsertMatch inserts or updates a match by ext_id and returns the row id.
func (r *SportsRepo) UpsertMatch(ctx context.Context, m *models.SportsMatch) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO sports_matches (ext_id, competition, stage, group_name,
		                            home_team, home_code, home_crest,
		                            away_team, away_code, away_crest,
		                            kickoff_utc, status, home_score, away_score, venue)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (ext_id) DO UPDATE SET
			competition = EXCLUDED.competition, stage = EXCLUDED.stage,
			group_name = EXCLUDED.group_name,
			home_team = EXCLUDED.home_team, home_code = EXCLUDED.home_code,
			home_crest = EXCLUDED.home_crest,
			away_team = EXCLUDED.away_team, away_code = EXCLUDED.away_code,
			away_crest = EXCLUDED.away_crest,
			kickoff_utc = EXCLUDED.kickoff_utc, status = EXCLUDED.status,
			home_score = EXCLUDED.home_score, away_score = EXCLUDED.away_score,
			venue = EXCLUDED.venue, updated_at = now()
		RETURNING id`,
		m.ExtID, m.Competition, m.Stage, m.GroupName,
		m.HomeTeam, m.HomeCode, m.HomeCrest,
		m.AwayTeam, m.AwayCode, m.AwayCrest,
		m.KickoffUTC, m.Status, m.HomeScore, m.AwayScore, m.Venue,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert sports match: %w", err)
	}
	return id, nil
}

// ListMatches returns matches for a competition; stage/group/date are optional
// filters ("" = all). date (YYYY-MM-DD) filters kickoff_utc within
// [date 00:00Z, date+24h). Ordered by kickoff_utc ASC, then ext_id.
func (r *SportsRepo) ListMatches(ctx context.Context, competition, stage, group, date string) ([]models.SportsMatch, error) {
	query := `SELECT ` + sportsMatchColumns + ` FROM sports_matches WHERE competition = $1`
	args := []any{competition}
	argIdx := 2

	if stage != "" {
		query += fmt.Sprintf(" AND stage = $%d", argIdx)
		args = append(args, stage)
		argIdx++
	}
	if group != "" {
		query += fmt.Sprintf(" AND group_name = $%d", argIdx)
		args = append(args, group)
		argIdx++
	}
	if date != "" {
		day, err := time.Parse("2006-01-02", date)
		if err != nil {
			return nil, fmt.Errorf("list sports matches: invalid date %q: %w", date, err)
		}
		query += fmt.Sprintf(" AND kickoff_utc >= $%d AND kickoff_utc < $%d", argIdx, argIdx+1)
		args = append(args, day, day.Add(24*time.Hour))
		argIdx += 2
	}

	query += " ORDER BY kickoff_utc ASC, ext_id ASC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sports matches: %w", err)
	}
	defer rows.Close()

	var matches []models.SportsMatch
	for rows.Next() {
		var m models.SportsMatch
		if err := scanSportsMatch(rows, &m); err != nil {
			return nil, fmt.Errorf("scan sports match: %w", err)
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// GetMatch returns a match by ID.
func (r *SportsRepo) GetMatch(ctx context.Context, id string) (*models.SportsMatch, error) {
	var m models.SportsMatch
	err := scanSportsMatch(r.pool.QueryRow(ctx, `
		SELECT `+sportsMatchColumns+` FROM sports_matches WHERE id = $1`, id), &m)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSportsMatchNotFound
		}
		return nil, fmt.Errorf("get sports match: %w", err)
	}
	return &m, nil
}

// LiveOrImminent reports whether any match is in play/paused now, or kicks off
// within d from now.
func (r *SportsRepo) LiveOrImminent(ctx context.Context, d time.Duration) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sports_matches
			WHERE status IN ('IN_PLAY', 'PAUSED')
			   OR (kickoff_utc > now() AND kickoff_utc <= now() + make_interval(secs => $1))
		)`,
		d.Seconds(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check live or imminent: %w", err)
	}
	return exists, nil
}

// UpsertPrediction inserts or updates a participant's prediction. The kickoff
// lock lives inside the SQL so racing requests cannot bypass it: both the
// insert path and the conflict-update path re-check kickoff_utc > now().
func (r *SportsRepo) UpsertPrediction(ctx context.Context, p *models.SportsPrediction) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO predictions (match_id, participant_id, predictor_kind,
		                         home_prob, draw_prob, away_prob, pick, reasoning,
		                         subject, predicted_outcome, confidence, resolve_by)
		SELECT m.id, $2, $3, $4::real, $5::real, $6::real, $7, $8,
		       CONCAT_WS(' vs ', m.home_team, m.away_team), $7,
		       COALESCE(GREATEST($4::real, $5::real, $6::real), (1.0 / 3.0)::real),
		       m.kickoff_utc
		FROM sports_matches m
		WHERE m.id = $1 AND m.kickoff_utc > now()
		ON CONFLICT (match_id, participant_id) DO UPDATE
			SET home_prob = EXCLUDED.home_prob, draw_prob = EXCLUDED.draw_prob,
			    away_prob = EXCLUDED.away_prob, pick = EXCLUDED.pick,
			    reasoning = EXCLUDED.reasoning, subject = EXCLUDED.subject,
			    predicted_outcome = EXCLUDED.predicted_outcome,
			    confidence = EXCLUDED.confidence, resolve_by = EXCLUDED.resolve_by,
			    updated_at = now()
			WHERE (SELECT kickoff_utc FROM sports_matches WHERE id = EXCLUDED.match_id) > now()`,
		p.MatchID, p.ParticipantID, p.PredictorKind,
		p.HomeProb, p.DrawProb, p.AwayProb, p.Pick, p.Reasoning,
	)
	if err != nil {
		return fmt.Errorf("upsert sports prediction: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	// Zero rows: either the match doesn't exist or it has kicked off.
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM sports_matches WHERE id = $1)`, p.MatchID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check sports match exists: %w", err)
	}
	if !exists {
		return ErrSportsMatchNotFound
	}
	return ErrPredictionLocked
}

// ListPredictions returns predictions for a match with display names and track
// records joined; agents first then humans, newest first within each kind.
func (r *SportsRepo) ListPredictions(ctx context.Context, matchID string, limit, offset int) ([]models.SportsPrediction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sp.id, sp.match_id, sp.participant_id, sp.predictor_kind,
		       COALESCE(p.display_name, '') as display_name,
		       sp.home_prob::float8, sp.draw_prob::float8, sp.away_prob::float8,
		       sp.pick, sp.reasoning, sp.outcome, sp.brier::float8, sp.created_at,
		       COALESCE(st.n, 0), COALESCE(st.correct, 0),
		       CASE WHEN sp.predictor_kind = 'agent' AND COALESCE(st.n, 0) > 0
		            THEN (st.brier_sum / st.n)::float8 END
		FROM predictions sp
		LEFT JOIN participants p ON p.id = sp.participant_id
		LEFT JOIN prediction_stats st ON st.participant_id = sp.participant_id
		WHERE sp.match_id = $1
		ORDER BY CASE WHEN sp.predictor_kind = 'agent' THEN 0 ELSE 1 END,
		         sp.created_at DESC
		LIMIT $2 OFFSET $3`,
		matchID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list sports predictions: %w", err)
	}
	defer rows.Close()

	var preds []models.SportsPrediction
	for rows.Next() {
		var p models.SportsPrediction
		if err := rows.Scan(
			&p.ID, &p.MatchID, &p.ParticipantID, &p.PredictorKind, &p.DisplayName,
			&p.HomeProb, &p.DrawProb, &p.AwayProb,
			&p.Pick, &p.Reasoning, &p.Outcome, &p.Brier, &p.CreatedAt,
			&p.StatsN, &p.StatsCorrect, &p.StatsBrier,
		); err != nil {
			return nil, fmt.Errorf("scan sports prediction: %w", err)
		}
		preds = append(preds, p)
	}
	return preds, rows.Err()
}

// PredictionAggregates returns pick counts, the total, average agent
// probabilities (when any agent has predicted), and the viewer's own
// prediction (nil when absent; viewerID may be "").
func (r *SportsRepo) PredictionAggregates(ctx context.Context, matchID, viewerID string) (map[string]any, error) {
	var home, draw, away, total int
	var avgHome, avgDraw, avgAway *float64
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE pick = 'home'),
		       COUNT(*) FILTER (WHERE pick = 'draw'),
		       COUNT(*) FILTER (WHERE pick = 'away'),
		       COUNT(*),
		       AVG(home_prob) FILTER (WHERE predictor_kind = 'agent'),
		       AVG(draw_prob) FILTER (WHERE predictor_kind = 'agent'),
		       AVG(away_prob) FILTER (WHERE predictor_kind = 'agent')
		FROM predictions
		WHERE match_id = $1`,
		matchID,
	).Scan(&home, &draw, &away, &total, &avgHome, &avgDraw, &avgAway)
	if err != nil {
		return nil, fmt.Errorf("aggregate sports predictions: %w", err)
	}

	agg := map[string]any{
		"home":   home,
		"draw":   draw,
		"away":   away,
		"total":  total,
		"viewer": nil,
	}
	if avgHome != nil || avgDraw != nil || avgAway != nil {
		agg["avg_probs"] = map[string]any{
			"home": avgHome,
			"draw": avgDraw,
			"away": avgAway,
		}
	}

	if viewerID != "" {
		var v models.SportsPrediction
		err := r.pool.QueryRow(ctx, `
			SELECT id, match_id, participant_id, predictor_kind,
			       home_prob::float8, draw_prob::float8, away_prob::float8,
			       pick, reasoning, outcome, brier::float8, created_at
			FROM predictions
			WHERE match_id = $1 AND participant_id = $2`,
			matchID, viewerID,
		).Scan(
			&v.ID, &v.MatchID, &v.ParticipantID, &v.PredictorKind,
			&v.HomeProb, &v.DrawProb, &v.AwayProb,
			&v.Pick, &v.Reasoning, &v.Outcome, &v.Brier, &v.CreatedAt,
		)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get viewer sports prediction: %w", err)
		}
		if err == nil {
			agg["viewer"] = &v
		}
	}

	return agg, nil
}

// SettleMatch grades all unsettled predictions for a FINISHED match and
// updates per-participant stats, all in one transaction. Idempotent: only
// rows with outcome IS NULL are graded, so a second call is a no-op.
//
// v1 grades by the stored final score; knockout matches decided after
// penalties grade by the stored full-time score.
func (r *SportsRepo) SettleMatch(ctx context.Context, matchID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var homeScore, awayScore *int
	err = tx.QueryRow(ctx, `
		SELECT status, home_score, away_score FROM sports_matches
		WHERE id = $1 FOR UPDATE`,
		matchID,
	).Scan(&status, &homeScore, &awayScore)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSportsMatchNotFound
		}
		return fmt.Errorf("get sports match for settle: %w", err)
	}
	if status != "FINISHED" || homeScore == nil || awayScore == nil {
		return fmt.Errorf("settle sports match: match %s is not finished", matchID)
	}

	result := "draw"
	if *homeScore > *awayScore {
		result = "home"
	} else if *awayScore > *homeScore {
		result = "away"
	}

	// One-hot encoding of the result for the Brier score (range 0–2).
	var oHome, oDraw, oAway float64
	switch result {
	case "home":
		oHome = 1
	case "draw":
		oDraw = 1
	case "away":
		oAway = 1
	}

	rows, err := tx.Query(ctx, `
		UPDATE predictions SET
			outcome = CASE WHEN pick = $2 THEN 'correct' ELSE 'wrong' END,
			resolution = $2,
			brier = CASE WHEN home_prob IS NOT NULL AND draw_prob IS NOT NULL AND away_prob IS NOT NULL
			        THEN power(home_prob - $3::float8, 2) + power(draw_prob - $4::float8, 2) + power(away_prob - $5::float8, 2)
			        END,
			resolved_at = now(),
			updated_at = now()
		WHERE match_id = $1 AND outcome IS NULL
		RETURNING participant_id, predictor_kind, outcome, brier::float8`,
		matchID, result, oHome, oDraw, oAway,
	)
	if err != nil {
		return fmt.Errorf("grade sports predictions: %w", err)
	}

	type graded struct {
		participantID string
		predictorKind string
		correct       bool
		brier         *float64
	}
	var gradedRows []graded
	for rows.Next() {
		var g graded
		var outcome string
		if err := rows.Scan(&g.participantID, &g.predictorKind, &outcome, &g.brier); err != nil {
			rows.Close()
			return fmt.Errorf("scan graded sports prediction: %w", err)
		}
		g.correct = outcome == "correct"
		gradedRows = append(gradedRows, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("grade sports predictions: %w", err)
	}

	// Upsert all stats rows in one statement via unnested arrays. Each
	// graded row contributes n=1, correct as 1/0, its brier (0 when the
	// prediction had no probabilities, e.g. humans), and ±1 as both the
	// initial streak for new rows and the correctness signal on conflict.
	if len(gradedRows) > 0 {
		ids := make([]string, len(gradedRows))
		kinds := make([]string, len(gradedRows))
		corrects := make([]int32, len(gradedRows))
		briers := make([]float64, len(gradedRows))
		streaks := make([]int32, len(gradedRows))
		for i, g := range gradedRows {
			ids[i] = g.participantID
			kinds[i] = g.predictorKind
			if g.correct {
				corrects[i] = 1
				streaks[i] = 1
			} else {
				streaks[i] = -1
			}
			if g.brier != nil {
				briers[i] = *g.brier
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO prediction_stats
				(participant_id, predictor_kind, n, correct, brier_sum, streak)
			SELECT unnest($1::uuid[]), unnest($2::text[]), 1,
			       unnest($3::int[]), unnest($4::float8[]), unnest($5::int[])
			ON CONFLICT (participant_id) DO UPDATE SET
				predictor_kind = EXCLUDED.predictor_kind,
				n = prediction_stats.n + 1,
				correct = prediction_stats.correct + EXCLUDED.correct,
				brier_sum = prediction_stats.brier_sum + EXCLUDED.brier_sum,
				streak = CASE WHEN EXCLUDED.correct = 1
				              THEN GREATEST(prediction_stats.streak, 0) + 1
				              ELSE LEAST(prediction_stats.streak, 0) - 1 END,
				updated_at = now()`,
			ids, kinds, corrects, briers, streaks,
		)
		if err != nil {
			return fmt.Errorf("upsert sports prediction stats: %w", err)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE sports_matches SET settled_at = COALESCE(settled_at, now()), updated_at = now()
		WHERE id = $1`,
		matchID,
	)
	if err != nil {
		return fmt.Errorf("mark sports match settled: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// SettleMatchParticipants settles a match and returns the participants whose
// previously-unsettled predictions were graded by this call. The sports
// poller uses the IDs to trigger scorecard recomputation without changing the
// long-standing SettleMatch API used by other callers.
func (r *SportsRepo) SettleMatchParticipants(ctx context.Context, matchID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT participant_id::text
		FROM predictions
		WHERE match_id = $1 AND outcome IS NULL
		ORDER BY participant_id`, matchID)
	if err != nil {
		return nil, fmt.Errorf("list unsettled sports predictors: %w", err)
	}
	var participantIDs []string
	for rows.Next() {
		var participantID string
		if err := rows.Scan(&participantID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan unsettled sports predictor: %w", err)
		}
		participantIDs = append(participantIDs, participantID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unsettled sports predictors: %w", err)
	}
	if err := r.SettleMatch(ctx, matchID); err != nil {
		return nil, err
	}
	return participantIDs, nil
}

// Leaderboard returns participants ranked by accuracy. kind filters by
// predictor_kind ("" = all); minN hides small samples.
func (r *SportsRepo) Leaderboard(ctx context.Context, kind string, minN, limit int) ([]models.SportsLeaderboardRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.participant_id, COALESCE(p.display_name, '') as display_name,
		       s.predictor_kind, s.n, s.correct,
		       CASE WHEN s.n > 0 THEN s.correct::float8 / s.n ELSE 0 END as accuracy,
		       CASE WHEN s.predictor_kind = 'agent' AND s.n > 0
		            THEN (s.brier_sum / s.n)::float8 END as avg_brier,
		       s.streak
		FROM prediction_stats s
		LEFT JOIN participants p ON p.id = s.participant_id
		WHERE s.n >= $1 AND ($2 = '' OR s.predictor_kind = $2)
		ORDER BY accuracy DESC, s.n DESC, s.participant_id
		LIMIT $3`,
		minN, kind, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get sports leaderboard: %w", err)
	}
	defer rows.Close()

	var entries []models.SportsLeaderboardRow
	for rows.Next() {
		var e models.SportsLeaderboardRow
		if err := rows.Scan(
			&e.ParticipantID, &e.DisplayName, &e.PredictorKind,
			&e.N, &e.Correct, &e.Accuracy, &e.AvgBrier, &e.Streak,
		); err != nil {
			return nil, fmt.Errorf("scan sports leaderboard row: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// TopAgentIDs returns participant ids of in-house agents (type 'agent' with
// an agent_identities row), ranked by trust_score DESC with an id ASC
// tiebreak so the ordering is deterministic. Used by the sports
// auto-predictor to pick which personas publish predictions.
func (r *SportsRepo) TopAgentIDs(ctx context.Context, limit, offset int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id
		FROM participants p
		JOIN agent_identities ai ON ai.participant_id = p.id
		WHERE p.type = 'agent'
		ORDER BY p.trust_score DESC, p.id ASC
		LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list top agent ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan top agent id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// HumansVsAgents returns aggregate accuracy for agents vs humans over all
// settled predictions.
func (r *SportsRepo) HumansVsAgents(ctx context.Context) (map[string]any, error) {
	out := map[string]any{
		"agents": map[string]any{"n": 0, "correct": 0, "accuracy": 0.0},
		"humans": map[string]any{"n": 0, "correct": 0, "accuracy": 0.0},
	}

	rows, err := r.pool.Query(ctx, `
		SELECT predictor_kind, COUNT(*), COUNT(*) FILTER (WHERE outcome = 'correct')
		FROM predictions
		WHERE outcome IS NOT NULL
		GROUP BY predictor_kind`,
	)
	if err != nil {
		return nil, fmt.Errorf("humans vs agents: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var kind string
		var n, correct int
		if err := rows.Scan(&kind, &n, &correct); err != nil {
			return nil, fmt.Errorf("scan humans vs agents row: %w", err)
		}
		accuracy := 0.0
		if n > 0 {
			accuracy = float64(correct) / float64(n)
		}
		out[kind+"s"] = map[string]any{"n": n, "correct": correct, "accuracy": accuracy}
	}
	return out, rows.Err()
}

// --- Live match center ---

// UpsertEvents inserts or updates a match's timeline events in one batch.
// UNIQUE(match_id, seq) makes re-polls idempotent: an event whose body
// changed upstream (ESPN edits commentary lines) is updated in place.
// Every array is cast explicitly — pgx mis-infers UNNEST argument types
// otherwise (same gotcha as SettleMatch's stats upsert).
func (r *SportsRepo) UpsertEvents(ctx context.Context, matchID string, events []models.SportsMatchEvent) error {
	if len(events) == 0 {
		return nil
	}

	seqs := make([]int32, len(events))
	minutes := make([]*string, len(events))
	kinds := make([]string, len(events))
	sides := make([]*string, len(events))
	players := make([]*string, len(events))
	bodies := make([]string, len(events))
	for i, e := range events {
		seqs[i] = int32(e.Seq)
		minutes[i] = e.Minute
		kinds[i] = e.Kind
		sides[i] = e.Side
		players[i] = e.Player
		bodies[i] = e.Body
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO sports_match_events (match_id, seq, minute, kind, side, player, body)
		SELECT $1, u.seq, u.minute, u.kind, u.side, u.player, u.body
		FROM UNNEST($2::int[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[])
		     AS u(seq, minute, kind, side, player, body)
		ON CONFLICT (match_id, seq) DO UPDATE SET
			minute = EXCLUDED.minute, kind = EXCLUDED.kind,
			side = EXCLUDED.side, player = EXCLUDED.player, body = EXCLUDED.body`,
		matchID, seqs, minutes, kinds, sides, players, bodies,
	)
	if err != nil {
		return fmt.Errorf("upsert sports match events: %w", err)
	}
	return nil
}

// MaxEventSeq returns the highest stored event seq for a match, or -1 when
// the match has no events yet. The ESPN poller uses it to fetch only new
// commentary lines.
func (r *SportsRepo) MaxEventSeq(ctx context.Context, matchID string) (int, error) {
	var maxSeq int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(seq), -1) FROM sports_match_events WHERE match_id = $1`,
		matchID,
	).Scan(&maxSeq)
	if err != nil {
		return 0, fmt.Errorf("max sports event seq: %w", err)
	}
	return maxSeq, nil
}

// EventsSince returns key (non-play) events for a match with seq > afterSeq,
// ascending. The agent reactor uses it to find new goals/cards/HT/FT it has
// not reacted to yet ('play' filler never warrants a take).
func (r *SportsRepo) EventsSince(ctx context.Context, matchID string, afterSeq int) ([]models.SportsMatchEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, match_id, seq, minute, kind, side, player, body, created_at
		FROM sports_match_events
		WHERE match_id = $1 AND seq > $2 AND kind != 'play'
		ORDER BY seq`,
		matchID, afterSeq,
	)
	if err != nil {
		return nil, fmt.Errorf("list sports events since: %w", err)
	}
	defer rows.Close()

	var events []models.SportsMatchEvent
	for rows.Next() {
		var e models.SportsMatchEvent
		if err := rows.Scan(
			&e.ID, &e.MatchID, &e.Seq, &e.Minute, &e.Kind, &e.Side, &e.Player, &e.Body, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sports match event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// MaxTakeSeq returns the highest event_seq among a match's takes, or -1 when
// none. NULL event_seqs (pre-match takes) are ignored by MAX. The reactor
// uses it as its high-water mark, so restarts never re-react to old events.
func (r *SportsRepo) MaxTakeSeq(ctx context.Context, matchID string) (int, error) {
	var maxSeq int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(event_seq), -1) FROM sports_agent_takes WHERE match_id = $1`,
		matchID,
	).Scan(&maxSeq)
	if err != nil {
		return 0, fmt.Errorf("max sports take seq: %w", err)
	}
	return maxSeq, nil
}

// InsertTake stores an agent's live reaction and fills t.ID/t.CreatedAt.
func (r *SportsRepo) InsertTake(ctx context.Context, t *models.SportsAgentTake) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO sports_agent_takes (match_id, participant_id, event_seq, body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		t.MatchID, t.ParticipantID, t.EventSeq, t.Body,
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert sports agent take: %w", err)
	}
	return nil
}

// sportsTakeColumns joins display names and the author's prediction for the
// same match (pick/outcome shown next to live takes).
const sportsTakeColumns = `
	SELECT t.id, t.match_id, t.participant_id, t.event_seq, t.body, t.created_at,
	       p.display_name, sp.pick, sp.outcome
	FROM sports_agent_takes t
	JOIN participants p ON p.id = t.participant_id
	LEFT JOIN predictions sp ON sp.match_id = t.match_id AND sp.participant_id = t.participant_id`

func scanSportsTake(row pgx.Row, t *models.SportsAgentTake) error {
	return row.Scan(
		&t.ID, &t.MatchID, &t.ParticipantID, &t.EventSeq, &t.Body, &t.CreatedAt,
		&t.DisplayName, &t.Pick, &t.Outcome,
	)
}

// Timeline returns the merged event+take stream for a match in ascending
// order. Events sort by (seq, 0, created_at); takes by
// (COALESCE(event_seq, -1), 1, created_at), so a take lands right after the
// event it reacts to and pre-match takes (NULL → -1) precede kickoff (seq 0).
// When limit > 0 only the most recent window is kept, still ascending.
func (r *SportsRepo) Timeline(ctx context.Context, matchID string, limit int) ([]models.SportsTimelineItem, error) {
	type keyed struct {
		item models.SportsTimelineItem
		seq  int
		pos  int
		at   time.Time
	}
	var items []keyed

	rows, err := r.pool.Query(ctx, `
		SELECT id, match_id, seq, minute, kind, side, player, body, created_at
		FROM sports_match_events
		WHERE match_id = $1
		ORDER BY seq`,
		matchID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sports match events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e models.SportsMatchEvent
		if err := rows.Scan(
			&e.ID, &e.MatchID, &e.Seq, &e.Minute, &e.Kind, &e.Side, &e.Player, &e.Body, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sports match event: %w", err)
		}
		items = append(items, keyed{
			item: models.SportsTimelineItem{Kind: "event", Event: &e},
			seq:  e.Seq, pos: 0, at: e.CreatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sports match events: %w", err)
	}

	takeRows, err := r.pool.Query(ctx,
		sportsTakeColumns+` WHERE t.match_id = $1 ORDER BY t.created_at`, matchID)
	if err != nil {
		return nil, fmt.Errorf("list sports agent takes: %w", err)
	}
	defer takeRows.Close()
	for takeRows.Next() {
		var t models.SportsAgentTake
		if err := scanSportsTake(takeRows, &t); err != nil {
			return nil, fmt.Errorf("scan sports agent take: %w", err)
		}
		seq := -1
		if t.EventSeq != nil {
			seq = *t.EventSeq
		}
		items = append(items, keyed{
			item: models.SportsTimelineItem{Kind: "take", Take: &t},
			seq:  seq, pos: 1, at: t.CreatedAt,
		})
	}
	if err := takeRows.Err(); err != nil {
		return nil, fmt.Errorf("list sports agent takes: %w", err)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].seq != items[j].seq {
			return items[i].seq < items[j].seq
		}
		if items[i].pos != items[j].pos {
			return items[i].pos < items[j].pos
		}
		return items[i].at.Before(items[j].at)
	})
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}

	out := make([]models.SportsTimelineItem, len(items))
	for i, k := range items {
		out[i] = k.item
	}
	return out, nil
}

// LatestTakes returns the newest takes across all matches, newest first,
// with display names and the author's same-match prediction joined.
func (r *SportsRepo) LatestTakes(ctx context.Context, limit int) ([]models.SportsAgentTake, error) {
	rows, err := r.pool.Query(ctx,
		sportsTakeColumns+` ORDER BY t.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list latest sports agent takes: %w", err)
	}
	defer rows.Close()

	var takes []models.SportsAgentTake
	for rows.Next() {
		var t models.SportsAgentTake
		if err := scanSportsTake(rows, &t); err != nil {
			return nil, fmt.Errorf("scan sports agent take: %w", err)
		}
		takes = append(takes, t)
	}
	return takes, rows.Err()
}

// SetEnrichment links a match to its ESPN event and stores lineups. A nil
// lineups payload keeps whatever is already stored (COALESCE), so the poller
// can refresh the id without clobbering lineups it didn't fetch.
func (r *SportsRepo) SetEnrichment(ctx context.Context, matchID string, espnEventID int64, lineups []byte) error {
	var lp any
	if len(lineups) > 0 {
		lp = lineups
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE sports_matches
		SET espn_event_id = $2, lineups = COALESCE($3, lineups), updated_at = now()
		WHERE id = $1`,
		matchID, espnEventID, lp,
	)
	if err != nil {
		return fmt.Errorf("set sports match enrichment: %w", err)
	}
	return nil
}

// MatchesToEnrich returns matches the ESPN poller should look at: anything
// live, plus matches kicking off within 2 hours or kicked off within the
// last 3 hours (covers full time + a buffer for late final commentary).
func (r *SportsRepo) MatchesToEnrich(ctx context.Context) ([]models.SportsMatch, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+sportsMatchColumns+` FROM sports_matches
		WHERE status IN ('IN_PLAY', 'PAUSED')
		   OR (kickoff_utc BETWEEN now() - interval '3 hours' AND now() + interval '2 hours')
		ORDER BY kickoff_utc`,
	)
	if err != nil {
		return nil, fmt.Errorf("list sports matches to enrich: %w", err)
	}
	defer rows.Close()

	var matches []models.SportsMatch
	for rows.Next() {
		var m models.SportsMatch
		if err := scanSportsMatch(rows, &m); err != nil {
			return nil, fmt.Errorf("scan sports match: %w", err)
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// GroupStandings computes group tables from our own FINISHED group-stage
// results (never from ESPN). Within a group teams order by points DESC,
// goal difference DESC, goals for DESC, then team name; groups order by name.
func (r *SportsRepo) GroupStandings(ctx context.Context) ([]models.SportsStandingRow, error) {
	rows, err := r.pool.Query(ctx, `
		WITH sides AS (
			SELECT group_name, home_team AS team, home_code AS code,
			       home_score AS gf, away_score AS ga
			FROM sports_matches
			WHERE status = 'FINISHED' AND stage = 'GROUP_STAGE'
			  AND home_score IS NOT NULL AND away_score IS NOT NULL
			UNION ALL
			SELECT group_name, away_team, away_code, away_score, home_score
			FROM sports_matches
			WHERE status = 'FINISHED' AND stage = 'GROUP_STAGE'
			  AND home_score IS NOT NULL AND away_score IS NOT NULL
		)
		SELECT group_name, team, code,
		       COUNT(*)::int,
		       COUNT(*) FILTER (WHERE gf > ga)::int,
		       COUNT(*) FILTER (WHERE gf = ga)::int,
		       COUNT(*) FILTER (WHERE gf < ga)::int,
		       COALESCE(SUM(gf), 0)::int AS goals_for,
		       COALESCE(SUM(ga), 0)::int AS goals_against,
		       COALESCE(SUM(gf - ga), 0)::int AS goal_diff,
		       (COUNT(*) FILTER (WHERE gf > ga) * 3 + COUNT(*) FILTER (WHERE gf = ga))::int AS points
		FROM sides
		GROUP BY group_name, team, code
		ORDER BY group_name, points DESC, goal_diff DESC, goals_for DESC, team`,
	)
	if err != nil {
		return nil, fmt.Errorf("compute sports group standings: %w", err)
	}
	defer rows.Close()

	var standings []models.SportsStandingRow
	for rows.Next() {
		var s models.SportsStandingRow
		if err := rows.Scan(
			&s.GroupName, &s.Team, &s.Code,
			&s.Played, &s.Won, &s.Drawn, &s.Lost,
			&s.GF, &s.GA, &s.GD, &s.Points,
		); err != nil {
			return nil, fmt.Errorf("scan sports standing row: %w", err)
		}
		standings = append(standings, s)
	}
	return standings, rows.Err()
}
