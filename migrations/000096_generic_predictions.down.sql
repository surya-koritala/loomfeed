-- Generic post predictions cannot be represented by the old sports-only
-- schema, so remove them before restoring match_id/pick NOT NULL.
DELETE FROM predictions WHERE post_id IS NOT NULL;

-- Remove generic contributions from the shared rollup before restoring its
-- sports-only name. Historical streak direction is intentionally reset; the
-- authoritative prediction rows and accuracy/Brier totals are preserved.
TRUNCATE prediction_stats;
INSERT INTO prediction_stats
    (participant_id, predictor_kind, n, correct, brier_sum, streak)
SELECT participant_id,
       predictor_kind,
       COUNT(*)::int,
       COUNT(*) FILTER (WHERE outcome = 'correct')::int,
       COALESCE(SUM(brier), 0),
       0
FROM predictions
WHERE outcome IS NOT NULL
GROUP BY participant_id, predictor_kind;

DROP INDEX IF EXISTS idx_predictions_participant_resolved;
DROP INDEX IF EXISTS idx_predictions_post_created;

ALTER TABLE predictions
    DROP CONSTRAINT IF EXISTS predictions_post_participant_unique,
    DROP CONSTRAINT IF EXISTS predictions_resolution_state,
    DROP CONSTRAINT IF EXISTS predictions_sports_pick_check,
    DROP CONSTRAINT IF EXISTS predictions_one_subject_source,
    DROP CONSTRAINT IF EXISTS predictions_resolution_not_blank,
    DROP CONSTRAINT IF EXISTS predictions_confidence_range,
    DROP CONSTRAINT IF EXISTS predictions_outcome_not_blank,
    DROP CONSTRAINT IF EXISTS predictions_subject_not_blank,
    ALTER COLUMN match_id SET NOT NULL,
    ALTER COLUMN pick SET NOT NULL,
    ADD CONSTRAINT sports_predictions_pick_check
        CHECK (pick IN ('home', 'draw', 'away')),
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS resolution,
    DROP COLUMN IF EXISTS resolve_by,
    DROP COLUMN IF EXISTS confidence,
    DROP COLUMN IF EXISTS predicted_outcome,
    DROP COLUMN IF EXISTS subject,
    DROP COLUMN IF EXISTS post_id;

ALTER TABLE prediction_stats RENAME TO sports_prediction_stats;
ALTER TABLE predictions RENAME TO sports_predictions;
ALTER INDEX idx_predictions_participant RENAME TO idx_sports_predictions_participant;
