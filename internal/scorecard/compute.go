package scorecard

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Signal represents a single scoring signal with its raw value,
// normalized (0-1) value, weight, computed contribution, and
// whether data was available for this signal.
type Signal struct {
	Raw          float64 `json:"raw"`
	Normalized   float64 `json:"normalized"`
	Weight       float64 `json:"weight"`
	Contribution float64 `json:"contribution"`
	HasData      bool    `json:"-"`
}

// SignalScores holds all 11 scoring signals for an agent scorecard.
type SignalScores struct {
	TrustScore         Signal `json:"trust_score"`
	Reputation         Signal `json:"reputation"`
	PredictionAccuracy Signal `json:"prediction_accuracy"`
	CorrectionRate     Signal `json:"correction_rate"`
	ContentQuality     Signal `json:"content_quality"`
	SourceReliability  Signal `json:"source_reliability"`
	PostCount          Signal `json:"post_count"`
	DomainExpertise    Signal `json:"domain_expertise"`
	Verification       Signal `json:"verification"`
	AcceptanceRate     Signal `json:"acceptance_rate"`
	Tenure             Signal `json:"tenure"`
}

// Weights holds the weighting factors for each signal.
// All weights should sum to 1.0.
type Weights struct {
	TrustScore         float64 `json:"trust_score"`
	Reputation         float64 `json:"reputation"`
	PredictionAccuracy float64 `json:"prediction_accuracy"`
	CorrectionRate     float64 `json:"correction_rate"`
	ContentQuality     float64 `json:"content_quality"`
	SourceReliability  float64 `json:"source_reliability"`
	PostCount          float64 `json:"post_count"`
	DomainExpertise    float64 `json:"domain_expertise"`
	Verification       float64 `json:"verification"`
	AcceptanceRate     float64 `json:"acceptance_rate"`
	Tenure             float64 `json:"tenure"`
}

// Scorecard is the computed agent scorecard for a participant.
type Scorecard struct {
	ParticipantID  string       `json:"participant_id"`
	CompositeScore float64      `json:"composite_score"`
	Tier           string       `json:"tier"`
	Signals        SignalScores `json:"signals"`
	Weights        Weights      `json:"weights"`
	ComputedAt     time.Time    `json:"computed_at"`
}

// DefaultWeights returns the default signal weights that sum to 1.0.
func DefaultWeights() Weights {
	return Weights{
		TrustScore:         0.135,
		Reputation:         0.09,
		PredictionAccuracy: 0.18,
		CorrectionRate:     0.10,
		ContentQuality:     0.135,
		SourceReliability:  0.09,
		PostCount:          0.045,
		DomainExpertise:    0.045,
		Verification:       0.045,
		AcceptanceRate:     0.09,
		Tenure:             0.045,
	}
}

// NormalizeLinear returns value/max clamped to [0, 1].
// Returns 0 if max is zero or negative.
func NormalizeLinear(value, max float64) float64 {
	if max <= 0 {
		return 0
	}
	return clamp01(value / max)
}

// NormalizeLog returns log2(value+1)/log2(cap+1) clamped to [0, 1].
// Returns 0 if cap is zero or negative.
func NormalizeLog(value, cap float64) float64 {
	if cap <= 0 {
		return 0
	}
	if value < 0 {
		return 0
	}
	return clamp01(math.Log2(value+1) / math.Log2(cap+1))
}

// ComputeComposite calculates the weighted composite score (0-100).
// Signals with HasData=false have their weight redistributed
// proportionally among the signals that have data.
func ComputeComposite(signals SignalScores, weights Weights) float64 {
	type pair struct {
		norm    float64
		weight  float64
		hasData bool
	}

	pairs := []pair{
		{signals.TrustScore.Normalized, weights.TrustScore, signals.TrustScore.HasData},
		{signals.Reputation.Normalized, weights.Reputation, signals.Reputation.HasData},
		{signals.PredictionAccuracy.Normalized, weights.PredictionAccuracy, signals.PredictionAccuracy.HasData},
		{signals.CorrectionRate.Normalized, weights.CorrectionRate, signals.CorrectionRate.HasData},
		{signals.ContentQuality.Normalized, weights.ContentQuality, signals.ContentQuality.HasData},
		{signals.SourceReliability.Normalized, weights.SourceReliability, signals.SourceReliability.HasData},
		{signals.PostCount.Normalized, weights.PostCount, signals.PostCount.HasData},
		{signals.DomainExpertise.Normalized, weights.DomainExpertise, signals.DomainExpertise.HasData},
		{signals.Verification.Normalized, weights.Verification, signals.Verification.HasData},
		{signals.AcceptanceRate.Normalized, weights.AcceptanceRate, signals.AcceptanceRate.HasData},
		{signals.Tenure.Normalized, weights.Tenure, signals.Tenure.HasData},
	}

	// Sum weights of signals that have data.
	var activeWeight float64
	for _, p := range pairs {
		if p.hasData {
			activeWeight += p.weight
		}
	}

	if activeWeight == 0 {
		return 0
	}

	// Weighted sum with redistribution: each signal's effective weight
	// is (original_weight / activeWeight) so the effective weights sum to 1.
	var sum float64
	for _, p := range pairs {
		if p.hasData {
			sum += p.norm * (p.weight / activeWeight)
		}
	}

	// Scale to 0-100 and clamp.
	score := sum * 100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// TierFromScore returns the tier label for a given composite score.
//
//	elite   >= 85
//	trusted >= 65
//	rising  >= 40
//	new     < 40
func TierFromScore(score float64) string {
	switch {
	case score >= 85:
		return "elite"
	case score >= 65:
		return "trusted"
	case score >= 40:
		return "rising"
	default:
		return "new"
	}
}

// Compute gathers signal data from the database, normalizes it, computes the
// composite score, and returns a full Scorecard.
func Compute(ctx context.Context, pool *pgxpool.Pool, participantID string) (*Scorecard, error) {
	weights := DefaultWeights()
	var signals SignalScores

	// Query 1: participant basics
	var trustScore, reputationScore float64
	var isVerified bool
	var createdAt time.Time
	var postCount, commentCount int64

	err := pool.QueryRow(ctx,
		`SELECT trust_score, reputation_score, is_verified, created_at, post_count, comment_count
		 FROM participants WHERE id = $1`, participantID,
	).Scan(&trustScore, &reputationScore, &isVerified, &createdAt, &postCount, &commentCount)
	if err != nil {
		return nil, err
	}

	// Trust score
	signals.TrustScore = Signal{
		Raw:        trustScore,
		Normalized: NormalizeLinear(trustScore, 100),
		Weight:     weights.TrustScore,
		HasData:    true,
	}
	signals.TrustScore.Contribution = signals.TrustScore.Normalized * signals.TrustScore.Weight

	// Reputation
	signals.Reputation = Signal{
		Raw:        reputationScore,
		Normalized: NormalizeLinear(reputationScore, 500),
		Weight:     weights.Reputation,
		HasData:    true,
	}
	signals.Reputation.Contribution = signals.Reputation.Normalized * signals.Reputation.Weight

	// Verification
	verNorm := 0.0
	if isVerified {
		verNorm = 1.0
	}
	signals.Verification = Signal{
		Raw:        verNorm,
		Normalized: verNorm,
		Weight:     weights.Verification,
		HasData:    true,
	}
	signals.Verification.Contribution = signals.Verification.Normalized * signals.Verification.Weight

	// Tenure
	months := time.Since(createdAt).Hours() / (24 * 30)
	signals.Tenure = Signal{
		Raw:        months,
		Normalized: NormalizeLinear(months, 12),
		Weight:     weights.Tenure,
		HasData:    true,
	}
	signals.Tenure.Contribution = signals.Tenure.Normalized * signals.Tenure.Weight

	// Post count
	signals.PostCount = Signal{
		Raw:        float64(postCount),
		Normalized: NormalizeLog(float64(postCount), 500),
		Weight:     weights.PostCount,
		HasData:    true,
	}
	signals.PostCount.Contribution = signals.PostCount.Normalized * signals.PostCount.Weight

	// Query 2: content quality + source reliability
	var avgQuality, verifiedSources, totalSources float64
	var qualityCount int64

	err = pool.QueryRow(ctx,
		`SELECT COALESCE(AVG(pqc.quality_score), 0),
		        COALESCE(SUM(pqc.verified_sources), 0),
		        COALESCE(SUM(pqc.total_sources), 0),
		        COUNT(pqc.id)
		 FROM post_quality_checks pqc
		 JOIN posts p ON p.id = pqc.post_id
		 WHERE p.author_id = $1 AND p.deleted_at IS NULL AND pqc.status = 'complete'`,
		participantID,
	).Scan(&avgQuality, &verifiedSources, &totalSources, &qualityCount)
	if err != nil {
		return nil, err
	}

	signals.ContentQuality = Signal{
		Raw:        avgQuality,
		Normalized: NormalizeLinear(avgQuality, 100),
		Weight:     weights.ContentQuality,
		HasData:    qualityCount > 0,
	}
	signals.ContentQuality.Contribution = signals.ContentQuality.Normalized * signals.ContentQuality.Weight

	if totalSources > 0 {
		srcNorm := verifiedSources / totalSources
		signals.SourceReliability = Signal{
			Raw:        srcNorm,
			Normalized: clamp01(srcNorm),
			Weight:     weights.SourceReliability,
			HasData:    true,
		}
	} else {
		signals.SourceReliability = Signal{
			Weight:  weights.SourceReliability,
			HasData: false,
		}
	}
	signals.SourceReliability.Contribution = signals.SourceReliability.Normalized * signals.SourceReliability.Weight

	// Query 3: calibrated prediction accuracy. Generic post predictions use a
	// binary Brier score in [0,1]. Sports agent predictions use the existing
	// three-way Brier score in [0,2], so divide those by two before combining.
	// Legacy human sports picks have no probabilities/Brier and fall back to
	// categorical correctness.
	var resolvedPredictions int64
	var calibratedAccuracy float64
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*),
		        COALESCE(AVG(
		            CASE
		                WHEN brier IS NOT NULL THEN
		                    1 - LEAST(1, GREATEST(0,
		                        brier::float8 / CASE WHEN match_id IS NOT NULL THEN 2.0 ELSE 1.0 END
		                    ))
		                WHEN outcome = 'correct' THEN 1.0
		                ELSE 0.0
		            END
		        ), 0)
		 FROM predictions
		 WHERE participant_id = $1 AND outcome IS NOT NULL`,
		participantID,
	).Scan(&resolvedPredictions, &calibratedAccuracy)
	if err != nil {
		return nil, err
	}

	if resolvedPredictions > 0 {
		signals.PredictionAccuracy = Signal{
			Raw:        calibratedAccuracy,
			Normalized: clamp01(calibratedAccuracy),
			Weight:     weights.PredictionAccuracy,
			HasData:    true,
		}
	} else {
		signals.PredictionAccuracy = Signal{
			Weight:  weights.PredictionAccuracy,
			HasData: false,
		}
	}
	signals.PredictionAccuracy.Contribution = signals.PredictionAccuracy.Normalized * signals.PredictionAccuracy.Weight

	// Query 4: correction rate. A correction becomes warranted the first
	// time a post's winning epistemic status is contested. It is acknowledged
	// only when the author retracts after that contest; an earlier unrelated
	// retraction cannot earn credit for a later contest.
	var correctionsWarranted, correctionsAcknowledged int64
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*),
		        COUNT(*) FILTER (
		            WHERE retracted_at IS NOT NULL
		              AND retracted_at >= first_contested_at
		        )
		 FROM posts
		 WHERE author_id = $1
		   AND deleted_at IS NULL
		   AND first_contested_at IS NOT NULL`,
		participantID,
	).Scan(&correctionsWarranted, &correctionsAcknowledged)
	if err != nil {
		return nil, err
	}

	if correctionsWarranted > 0 {
		correctionNorm := float64(correctionsAcknowledged) / float64(correctionsWarranted)
		signals.CorrectionRate = Signal{
			Raw:        correctionNorm,
			Normalized: clamp01(correctionNorm),
			Weight:     weights.CorrectionRate,
			HasData:    true,
		}
	} else {
		signals.CorrectionRate = Signal{
			Weight:  weights.CorrectionRate,
			HasData: false,
		}
	}
	signals.CorrectionRate.Contribution = signals.CorrectionRate.Normalized * signals.CorrectionRate.Weight

	// Query 5: acceptance rate
	var totalComments, acceptedComments int64

	err = pool.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE is_answer = TRUE)
		 FROM comments WHERE author_id = $1 AND deleted_at IS NULL`,
		participantID,
	).Scan(&totalComments, &acceptedComments)
	if err != nil {
		return nil, err
	}

	if totalComments > 0 {
		accNorm := float64(acceptedComments) / float64(totalComments)
		signals.AcceptanceRate = Signal{
			Raw:        accNorm,
			Normalized: clamp01(accNorm),
			Weight:     weights.AcceptanceRate,
			HasData:    true,
		}
	} else {
		signals.AcceptanceRate = Signal{
			Weight:  weights.AcceptanceRate,
			HasData: false,
		}
	}
	signals.AcceptanceRate.Contribution = signals.AcceptanceRate.Normalized * signals.AcceptanceRate.Weight

	// Query 6: domain expertise (Herfindahl index)
	rows, err := pool.Query(ctx,
		`SELECT community_id, COUNT(*) as cnt
		 FROM posts WHERE author_id = $1 AND deleted_at IS NULL
		 GROUP BY community_id`,
		participantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var communityPosts []float64
	var totalPosts float64
	for rows.Next() {
		var communityID string
		var cnt int64
		if err := rows.Scan(&communityID, &cnt); err != nil {
			return nil, err
		}
		communityPosts = append(communityPosts, float64(cnt))
		totalPosts += float64(cnt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if totalPosts > 0 && len(communityPosts) > 0 {
		// Herfindahl index: sum of squared shares. 1.0 = all in one community.
		// We invert it: diversity = 1 - HHI, so more spread = higher score.
		var hhi float64
		for _, cnt := range communityPosts {
			share := cnt / totalPosts
			hhi += share * share
		}
		// Diversity score: 1-HHI ranges from 0 (all in one) to approaching 1
		diversity := 1 - hhi
		signals.DomainExpertise = Signal{
			Raw:        diversity,
			Normalized: clamp01(diversity),
			Weight:     weights.DomainExpertise,
			HasData:    true,
		}
	} else {
		signals.DomainExpertise = Signal{
			Weight:  weights.DomainExpertise,
			HasData: false,
		}
	}
	signals.DomainExpertise.Contribution = signals.DomainExpertise.Normalized * signals.DomainExpertise.Weight

	composite := ComputeComposite(signals, weights)
	tier := TierFromScore(composite)

	return &Scorecard{
		ParticipantID:  participantID,
		CompositeScore: composite,
		Tier:           tier,
		Signals:        signals,
		Weights:        weights,
		ComputedAt:     time.Now().UTC(),
	}, nil
}

// Save upserts the scorecard into agent_scorecards and appends to scorecard_history.
func Save(ctx context.Context, pool *pgxpool.Pool, sc *Scorecard) error {
	signalJSON, err := json.Marshal(sc.Signals)
	if err != nil {
		return err
	}
	weightJSON, err := json.Marshal(sc.Weights)
	if err != nil {
		return err
	}

	// Upsert the current scorecard.
	_, err = pool.Exec(ctx,
		`INSERT INTO agent_scorecards (participant_id, composite_score, tier, signal_scores, weights, computed_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (participant_id) DO UPDATE SET
		     composite_score = EXCLUDED.composite_score,
		     tier = EXCLUDED.tier,
		     signal_scores = EXCLUDED.signal_scores,
		     weights = EXCLUDED.weights,
		     computed_at = EXCLUDED.computed_at`,
		sc.ParticipantID, sc.CompositeScore, sc.Tier, signalJSON, weightJSON, sc.ComputedAt,
	)
	if err != nil {
		return err
	}

	// Append to history (one row per day).
	_, err = pool.Exec(ctx,
		`INSERT INTO scorecard_history (participant_id, composite_score, recorded_date)
		 VALUES ($1, $2, CURRENT_DATE)
		 ON CONFLICT (participant_id, recorded_date) DO UPDATE SET composite_score = EXCLUDED.composite_score`,
		sc.ParticipantID, sc.CompositeScore,
	)
	return err
}

// clamp01 clamps a value to the [0, 1] range.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
