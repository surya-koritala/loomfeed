-- Promote the World Cup prediction ledger into a subject-agnostic ledger.
-- Sports keeps using match_id and its three-way probability columns; generic
-- predictions use post_id plus subject/outcome/confidence/resolution.
ALTER TABLE sports_predictions RENAME TO predictions;
ALTER INDEX idx_sports_predictions_participant RENAME TO idx_predictions_participant;
ALTER TABLE sports_prediction_stats RENAME TO prediction_stats;

ALTER TABLE predictions
    ADD COLUMN post_id UUID REFERENCES posts(id) ON DELETE CASCADE,
    ADD COLUMN subject TEXT,
    ADD COLUMN predicted_outcome TEXT,
    ADD COLUMN confidence REAL,
    ADD COLUMN resolve_by TIMESTAMPTZ,
    ADD COLUMN resolution TEXT,
    ADD COLUMN resolved_at TIMESTAMPTZ;

UPDATE predictions p
SET subject = CONCAT_WS(' vs ', m.home_team, m.away_team),
    predicted_outcome = p.pick,
    confidence = COALESCE(GREATEST(p.home_prob, p.draw_prob, p.away_prob), 1.0 / 3.0),
    resolve_by = m.kickoff_utc,
    resolution = CASE
        WHEN p.outcome IS NULL THEN NULL
        WHEN m.home_score > m.away_score THEN 'home'
        WHEN m.away_score > m.home_score THEN 'away'
        ELSE 'draw'
    END,
    resolved_at = CASE
        WHEN p.outcome IS NULL THEN NULL
        ELSE COALESCE(m.settled_at, p.updated_at)
    END
FROM sports_matches m
WHERE m.id = p.match_id;

ALTER TABLE predictions
    ALTER COLUMN match_id DROP NOT NULL,
    ALTER COLUMN pick DROP NOT NULL,
    ALTER COLUMN subject SET NOT NULL,
    ALTER COLUMN predicted_outcome SET NOT NULL,
    ALTER COLUMN confidence SET NOT NULL,
    ALTER COLUMN resolve_by SET NOT NULL,
    DROP CONSTRAINT sports_predictions_pick_check,
    ADD CONSTRAINT predictions_subject_not_blank
        CHECK (LENGTH(BTRIM(subject)) > 0),
    ADD CONSTRAINT predictions_outcome_not_blank
        CHECK (LENGTH(BTRIM(predicted_outcome)) > 0),
    ADD CONSTRAINT predictions_confidence_range
        CHECK (confidence >= 0 AND confidence <= 1),
    ADD CONSTRAINT predictions_resolution_not_blank
        CHECK (resolution IS NULL OR LENGTH(BTRIM(resolution)) > 0),
    ADD CONSTRAINT predictions_one_subject_source
        CHECK (NUM_NONNULLS(match_id, post_id) = 1),
    ADD CONSTRAINT predictions_sports_pick_check
        CHECK (match_id IS NULL OR pick IN ('home', 'draw', 'away')),
    ADD CONSTRAINT predictions_resolution_state
        CHECK (
            (resolution IS NULL AND resolved_at IS NULL AND outcome IS NULL)
            OR
            (resolution IS NOT NULL AND resolved_at IS NOT NULL AND outcome IS NOT NULL)
        ),
    ADD CONSTRAINT predictions_post_participant_unique
        UNIQUE (post_id, participant_id);

CREATE INDEX idx_predictions_post_created
    ON predictions (post_id, created_at DESC)
    WHERE post_id IS NOT NULL;

CREATE INDEX idx_predictions_participant_resolved
    ON predictions (participant_id, resolved_at DESC)
    WHERE outcome IS NOT NULL;
