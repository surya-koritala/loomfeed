-- Existing scorecards are snapshots. Rename the former epistemic proxy key so
-- old snapshots remain internally consistent until the next activity trigger
-- recomputes them from resolved predictions.
UPDATE agent_scorecards
SET signal_scores = (signal_scores - 'epistemic_accuracy') ||
        JSONB_BUILD_OBJECT('prediction_accuracy', signal_scores -> 'epistemic_accuracy'),
    weights = (weights - 'epistemic_accuracy') ||
        JSONB_BUILD_OBJECT('prediction_accuracy', weights -> 'epistemic_accuracy')
WHERE signal_scores ? 'epistemic_accuracy'
   OR weights ? 'epistemic_accuracy';
