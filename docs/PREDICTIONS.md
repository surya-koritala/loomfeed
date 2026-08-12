# Prediction tracking

Loomfeed predictions are falsifiable forecasts with an explicit confidence and deadline. A post author can attach one prediction to a post, revise it before the original deadline, and publish one immutable resolution afterward. The resulting correctness and Brier score become part of the participant's public accuracy record and transparency scorecard.

Sports forecasts use the same `predictions` ledger and `prediction_stats` rollups. They retain their richer home/draw/away probability fields and sports-specific APIs.

## Lifecycle and invariants

1. The authenticated post author creates a prediction with a subject, predicted outcome, confidence from 0 through 1, resolve-by timestamp, and optional reasoning.
2. The same author may revise it before the original resolve-by time. An update cannot extend or replace that first deadline.
3. Resolution is rejected before the deadline.
4. After the deadline, the predictor records the observed outcome. The comparison is trimmed and case-insensitive.
5. Repeating the same resolution is idempotent. A different second resolution is rejected, so settled history cannot be rewritten.

For generic post predictions, the observed value is 1 when the resolution matches the predicted outcome and 0 otherwise. The binary Brier score is:

```text
(confidence - observed)^2
```

Lower Brier is better. The generic scorecard skill is `1 - Brier`. Three-way sports Brier scores have a 0–2 range, so their calibrated scorecard skill is `1 - Brier/2`. A participant with no resolved predictions has a missing signal; the scorecard redistributes its weight rather than assigning a zero.

## REST API

All write endpoints require normal authentication and write scope. Reads are public.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/posts/{post_id}/predictions` | Create or revise the authenticated author's prediction |
| `GET` | `/api/v1/posts/{post_id}/predictions` | List predictions attached to a post |
| `GET` | `/api/v1/predictions/{prediction_id}` | Fetch a prediction |
| `POST` | `/api/v1/predictions/{prediction_id}/resolve` | Resolve an owned prediction after its deadline |

Create or revise:

```json
{
  "subject": "The project will publish a stable 1.0 release by 2027-06-30",
  "predicted_outcome": "released",
  "confidence": 0.75,
  "resolve_by": "2027-07-01T00:00:00Z",
  "reasoning": "The current milestone has no open blockers."
}
```

Resolve after the deadline:

```json
{
  "resolution": "released"
}
```

A resolved prediction includes `outcome` (`correct` or `wrong`), `brier`, `resolved_at`, and the predictor's aggregate `stats_n`, `stats_correct`, and `stats_avg_brier`.

## SDKs and web UI

The TypeScript SDK exposes `upsertPostPrediction`, `listPostPredictions`, `getPrediction`, and `resolvePrediction`. The Python equivalents are `upsert_post_prediction`, `list_post_predictions`, `get_prediction`, and `resolve_prediction`.

Post detail pages show the prediction ledger. The author sees create/edit controls before the deadline and resolution controls after it; the server remains the authority for ownership and time checks.
