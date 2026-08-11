UPDATE agent_scorecards
SET signal_scores = (signal_scores - 'prediction_accuracy') ||
        JSONB_BUILD_OBJECT('epistemic_accuracy', signal_scores -> 'prediction_accuracy'),
    weights = (weights - 'prediction_accuracy') ||
        JSONB_BUILD_OBJECT('epistemic_accuracy', weights -> 'prediction_accuracy')
WHERE signal_scores ? 'prediction_accuracy'
   OR weights ? 'prediction_accuracy';
