-- Sports match predictions: store World Cup 2026 matches, per-participant
-- predictions (agent-generated or human), and rolled-up prediction stats.
-- Outcome and Brier score computed post-match for accuracy tracking.
CREATE TABLE sports_matches (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ext_id       bigint UNIQUE NOT NULL,
    competition  text NOT NULL DEFAULT 'wc2026',
    stage        text NOT NULL DEFAULT '',
    group_name   text NOT NULL DEFAULT '',
    home_team    text NOT NULL DEFAULT '',
    home_code    text NOT NULL DEFAULT '',
    home_crest   text NOT NULL DEFAULT '',
    away_team    text NOT NULL DEFAULT '',
    away_code    text NOT NULL DEFAULT '',
    away_crest   text NOT NULL DEFAULT '',
    kickoff_utc  timestamptz NOT NULL,
    status       text NOT NULL DEFAULT 'SCHEDULED',
    home_score   int,
    away_score   int,
    venue        text NOT NULL DEFAULT '',
    settled_at   timestamptz,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_sports_matches_kickoff ON sports_matches (competition, kickoff_utc);
CREATE INDEX idx_sports_matches_status  ON sports_matches (status);

CREATE TABLE sports_predictions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id       uuid NOT NULL REFERENCES sports_matches(id) ON DELETE CASCADE,
    participant_id uuid NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    predictor_kind text NOT NULL CHECK (predictor_kind IN ('agent','human')),
    home_prob      real,
    draw_prob      real,
    away_prob      real,
    pick           text NOT NULL CHECK (pick IN ('home','draw','away')),
    reasoning      text NOT NULL DEFAULT '',
    outcome        text CHECK (outcome IN ('correct','wrong')),
    brier          real,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (match_id, participant_id)
);
CREATE INDEX idx_sports_predictions_participant ON sports_predictions (participant_id);

CREATE TABLE sports_prediction_stats (
    participant_id uuid PRIMARY KEY REFERENCES participants(id) ON DELETE CASCADE,
    predictor_kind text NOT NULL,
    n         int  NOT NULL DEFAULT 0,
    correct   int  NOT NULL DEFAULT 0,
    brier_sum real NOT NULL DEFAULT 0,
    streak    int  NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);
