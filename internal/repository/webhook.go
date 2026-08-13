package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/database"
)

// Webhook represents a registered webhook.
type Webhook struct {
	ID              string     `json:"id"`
	ParticipantID   string     `json:"participant_id"`
	URL             string     `json:"url"`
	Secret          string     `json:"-"`
	Events          []string   `json:"events"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       time.Time  `json:"created_at"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	FailureCount    int        `json:"failure_count"`
}

// WebhookDelivery represents a single webhook delivery attempt.
type WebhookDelivery struct {
	ID            string     `json:"id"`
	EventID       string     `json:"event_id"`
	WebhookID     string     `json:"webhook_id"`
	EventType     string     `json:"event_type"`
	Payload       any        `json:"payload"`
	StatusCode    int        `json:"status_code"`
	ResponseBody  string     `json:"response_body"`
	DeliveredAt   time.Time  `json:"delivered_at"`
	Success       bool       `json:"success"`
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attempt_count"`
	Terminal      bool       `json:"terminal"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
}

// WebhookDeliveryClaim is an exclusively leased delivery job. The event
// fields are immutable and therefore produce the same signed bytes on retry.
type WebhookDeliveryClaim struct {
	JobID         string
	ClaimToken    string
	AttemptNumber int
	EventID       string
	EventType     string
	Payload       map[string]any
	OccurredAt    time.Time
	Webhook       Webhook
}

// WebhookAttempt records the outcome used to finish one claimed job.
type WebhookAttempt struct {
	JobID         string
	ClaimToken    string
	AttemptNumber int
	EventID       string
	WebhookID     string
	EventType     string
	Payload       map[string]any
	StatusCode    int
	ResponseBody  string
	Success       bool
	Retryable     bool
	MaxAttempts   int
	RetryAt       time.Time
}

// WebhookRepo handles database operations for webhooks.
type WebhookRepo struct {
	pool *pgxpool.Pool
}

// NewWebhookRepo creates a new WebhookRepo.
func NewWebhookRepo(pool *pgxpool.Pool) *WebhookRepo {
	return &WebhookRepo{pool: pool}
}

// Enqueue persists an immutable logical event and its current subscriber jobs.
func (r *WebhookRepo) Enqueue(ctx context.Context, eventType string, payload map[string]any) (string, error) {
	var eventID string
	err := database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		var err error
		eventID, err = enqueueWebhookEvent(ctx, tx, eventType, payload)
		return err
	})
	return eventID, err
}

// EnqueueTx persists an event in the caller's originating transaction.
func (r *WebhookRepo) EnqueueTx(ctx context.Context, tx pgx.Tx, eventType string, payload map[string]any) (string, error) {
	return enqueueWebhookEvent(ctx, tx, eventType, payload)
}

// enqueueWebhookEvent is package-visible so source repositories can make the
// outbox write part of their own state transaction without package cycles.
func enqueueWebhookEvent(ctx context.Context, db database.DBTX, eventType string, payload map[string]any) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal webhook outbox payload: %w", err)
	}
	eventID := uuid.NewString()
	occurredAt := time.Now().UTC()
	err = db.QueryRow(ctx, `
		INSERT INTO webhook_outbox_events (id, event_type, payload, occurred_at)
		SELECT $1, $2, $3, $4
		WHERE EXISTS (
			SELECT 1 FROM webhooks WHERE is_active = TRUE AND $2 = ANY(events)
		)
		RETURNING id`, eventID, eventType, payloadBytes, occurredAt).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("insert webhook outbox event: %w", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO webhook_delivery_jobs (event_id, webhook_id)
		SELECT $1, id
		FROM webhooks
		WHERE is_active = TRUE AND $2 = ANY(events)`, eventID, eventType); err != nil {
		return "", fmt.Errorf("insert webhook delivery jobs: %w", err)
	}
	return eventID, nil
}

func webhookExcerpt(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit || limit <= 0 {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

// ClaimDelivery leases one due job and its destination across all replicas.
// The destination-row lease serializes different events for the same hook.
func (r *WebhookRepo) ClaimDelivery(ctx context.Context, lease time.Duration) (*WebhookDeliveryClaim, error) {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin webhook delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE app_service"); err != nil {
		return nil, fmt.Errorf("assume webhook service role for claim: %w", err)
	}

	var claim WebhookDeliveryClaim
	var payloadBytes []byte
	var previousAttempts int
	err = tx.QueryRow(ctx, `
		SELECT j.id, j.event_id, e.event_type, e.payload, e.occurred_at,
		       j.attempt_count,
		       w.id, w.participant_id, w.url, w.secret, w.events,
		       w.is_active, w.created_at, w.last_triggered_at, w.failure_count
		FROM webhook_delivery_jobs j
		JOIN webhook_outbox_events e ON e.id = j.event_id
		JOIN webhooks w ON w.id = j.webhook_id
		WHERE w.is_active = TRUE
		  AND (w.delivery_claim_expires_at IS NULL OR w.delivery_claim_expires_at <= NOW())
		  AND (
		    (j.status IN ('pending', 'retry') AND j.next_attempt_at <= NOW())
		    OR (j.status = 'processing' AND j.claim_expires_at <= NOW())
		  )
		ORDER BY j.next_attempt_at, j.created_at, j.id
		FOR UPDATE OF j SKIP LOCKED
		LIMIT 1`).Scan(
		&claim.JobID, &claim.EventID, &claim.EventType, &payloadBytes, &claim.OccurredAt,
		&previousAttempts,
		&claim.Webhook.ID, &claim.Webhook.ParticipantID, &claim.Webhook.URL,
		&claim.Webhook.Secret, &claim.Webhook.Events, &claim.Webhook.IsActive,
		&claim.Webhook.CreatedAt, &claim.Webhook.LastTriggeredAt, &claim.Webhook.FailureCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select webhook delivery claim: %w", err)
	}
	if err := json.Unmarshal(payloadBytes, &claim.Payload); err != nil {
		return nil, fmt.Errorf("decode webhook outbox payload: %w", err)
	}

	claim.ClaimToken = uuid.NewString()
	claim.AttemptNumber = previousAttempts + 1
	leaseUntil := time.Now().UTC().Add(lease)
	result, err := tx.Exec(ctx, `
		UPDATE webhooks
		SET delivery_claim_token = $2, delivery_claim_expires_at = $3
		WHERE id = $1
		  AND is_active = TRUE
		  AND (delivery_claim_expires_at IS NULL OR delivery_claim_expires_at <= NOW())`,
		claim.Webhook.ID, claim.ClaimToken, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("lease webhook destination: %w", err)
	}
	if result.RowsAffected() != 1 {
		return nil, nil
	}
	result, err = tx.Exec(ctx, `
		UPDATE webhook_delivery_jobs
		SET status = 'processing', attempt_count = $2,
		    claim_token = $3, claim_expires_at = $4,
		    last_attempt_at = NOW(), updated_at = NOW()
		WHERE id = $1`, claim.JobID, claim.AttemptNumber, claim.ClaimToken, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("claim webhook delivery job: %w", err)
	}
	if result.RowsAffected() != 1 {
		return nil, fmt.Errorf("webhook delivery job %s disappeared during claim", claim.JobID)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit webhook delivery claim: %w", err)
	}
	return &claim, nil
}

// FinishDelivery atomically records an attempt, schedules retry/terminal
// state, and releases the destination lease owned by this claim.
func (r *WebhookRepo) FinishDelivery(ctx context.Context, attempt WebhookAttempt) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin webhook delivery finish: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE app_service"); err != nil {
		return fmt.Errorf("assume webhook service role for finish: %w", err)
	}

	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT TRUE FROM webhook_delivery_jobs
		WHERE id = $1 AND status = 'processing' AND claim_token = $2
		FOR UPDATE`, attempt.JobID, attempt.ClaimToken).Scan(&exists); err != nil {
		return fmt.Errorf("lock webhook delivery claim: %w", err)
	}

	terminal := false
	status := "succeeded"
	var nextAttempt any
	if attempt.Success {
		_, err = tx.Exec(ctx, `
			UPDATE webhooks
			SET failure_count = 0, is_active = TRUE, last_triggered_at = NOW(),
			    delivery_claim_token = NULL, delivery_claim_expires_at = NULL
			WHERE id = $1 AND delivery_claim_token = $2`, attempt.WebhookID, attempt.ClaimToken)
	} else {
		var active bool
		err = tx.QueryRow(ctx, `
			UPDATE webhooks
			SET failure_count = failure_count + 1,
			    is_active = is_active AND failure_count + 1 < 10,
			    delivery_claim_token = NULL, delivery_claim_expires_at = NULL
			WHERE id = $1 AND delivery_claim_token = $2
			RETURNING is_active`, attempt.WebhookID, attempt.ClaimToken).Scan(&active)
		terminal = !attempt.Retryable || attempt.AttemptNumber >= attempt.MaxAttempts || !active
		if terminal {
			status = "dead"
		} else {
			status = "retry"
			nextAttempt = attempt.RetryAt
		}
		if !active {
			if _, cancelErr := tx.Exec(ctx, `
				UPDATE webhook_delivery_jobs
				SET status = 'canceled', completed_at = NOW(), updated_at = NOW(),
				    claim_token = NULL, claim_expires_at = NULL
				WHERE webhook_id = $1 AND id <> $2
				  AND status IN ('pending', 'retry', 'processing')`, attempt.WebhookID, attempt.JobID); cancelErr != nil {
				return fmt.Errorf("cancel jobs for inactive webhook: %w", cancelErr)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("update webhook delivery health: %w", err)
	}

	payloadBytes, err := json.Marshal(attempt.Payload)
	if err != nil {
		return fmt.Errorf("marshal webhook attempt payload: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO webhook_deliveries (
			event_id, webhook_id, event_type, payload, status_code,
			response_body, success, job_id, attempt_number, terminal
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		attempt.EventID, attempt.WebhookID, attempt.EventType, payloadBytes,
		attempt.StatusCode, attempt.ResponseBody, attempt.Success,
		attempt.JobID, attempt.AttemptNumber, terminal); err != nil {
		return fmt.Errorf("record webhook delivery attempt: %w", err)
	}

	if status == "retry" {
		_, err = tx.Exec(ctx, `
			UPDATE webhook_delivery_jobs
			SET status = 'retry', next_attempt_at = $3,
			    claim_token = NULL, claim_expires_at = NULL, updated_at = NOW()
			WHERE id = $1 AND claim_token = $2`, attempt.JobID, attempt.ClaimToken, nextAttempt)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE webhook_delivery_jobs
			SET status = $3, completed_at = NOW(),
			    claim_token = NULL, claim_expires_at = NULL, updated_at = NOW()
			WHERE id = $1 AND claim_token = $2`, attempt.JobID, attempt.ClaimToken, status)
	}
	if err != nil {
		return fmt.Errorf("finish webhook delivery job: %w", err)
	}
	return tx.Commit(ctx)
}

// Create inserts a new webhook registration.
func (r *WebhookRepo) Create(ctx context.Context, participantID, url, secret string, events []string) (*Webhook, error) {
	var w Webhook
	err := r.pool.QueryRow(ctx,
		`INSERT INTO webhooks (participant_id, url, secret, events)
         VALUES ($1, $2, $3, $4)
         RETURNING id, participant_id, url, secret, events, is_active, created_at, last_triggered_at, failure_count`,
		participantID, url, secret, events,
	).Scan(&w.ID, &w.ParticipantID, &w.URL, &w.Secret, &w.Events, &w.IsActive, &w.CreatedAt, &w.LastTriggeredAt, &w.FailureCount)
	if err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}
	return &w, nil
}

// ListByParticipant returns all webhooks for a participant.
func (r *WebhookRepo) ListByParticipant(ctx context.Context, participantID string) ([]Webhook, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, participant_id, url, secret, events, is_active, created_at, last_triggered_at, failure_count
         FROM webhooks WHERE participant_id = $1 ORDER BY created_at DESC`,
		participantID)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()

	var hooks []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.ParticipantID, &w.URL, &w.Secret, &w.Events, &w.IsActive, &w.CreatedAt, &w.LastTriggeredAt, &w.FailureCount); err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}
		hooks = append(hooks, w)
	}
	return hooks, rows.Err()
}

// ListByEvent returns all active webhooks subscribed to a specific event type.
func (r *WebhookRepo) ListByEvent(ctx context.Context, eventType string) ([]Webhook, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, participant_id, url, secret, events, is_active, created_at, last_triggered_at, failure_count
         FROM webhooks
         WHERE is_active = TRUE AND $1 = ANY(events)`,
		eventType)
	if err != nil {
		return nil, fmt.Errorf("list webhooks by event: %w", err)
	}
	defer rows.Close()

	var hooks []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.ParticipantID, &w.URL, &w.Secret, &w.Events, &w.IsActive, &w.CreatedAt, &w.LastTriggeredAt, &w.FailureCount); err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}
		hooks = append(hooks, w)
	}
	return hooks, rows.Err()
}

// Delete removes a webhook owned by the participant.
func (r *WebhookRepo) Delete(ctx context.Context, webhookID, participantID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM webhooks WHERE id = $1 AND participant_id = $2`,
		webhookID, participantID)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("webhook not found or not owned by participant")
	}
	return nil
}

// RecordDelivery logs a webhook delivery attempt.
func (r *WebhookRepo) RecordDelivery(ctx context.Context, eventID, webhookID, eventType string, payload map[string]any, statusCode int, responseBody string, success bool) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (event_id, webhook_id, event_type, payload, status_code, response_body, success)
         VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		eventID, webhookID, eventType, payloadBytes, statusCode, responseBody, success)
	if err != nil {
		return fmt.Errorf("record delivery: %w", err)
	}
	if success {
		_, _ = r.pool.Exec(ctx,
			`UPDATE webhooks SET last_triggered_at = NOW() WHERE id = $1`, webhookID)
	}
	return nil
}

// RecordFailure atomically increments the consecutive-failure count and
// deactivates the webhook at the threshold. Keeping both changes in one row
// update prevents a concurrent successful delivery from being overwritten by
// a stale threshold decision.
func (r *WebhookRepo) RecordFailure(ctx context.Context, webhookID string) (int, bool, error) {
	var count int
	var active bool
	err := r.pool.QueryRow(ctx,
		`UPDATE webhooks
		 SET failure_count = failure_count + 1,
		     is_active = is_active AND failure_count + 1 < 10
		 WHERE id = $1
		 RETURNING failure_count, is_active`,
		webhookID).Scan(&count, &active)
	return count, active, err
}

// ResetFailure records a successful in-flight delivery. It clears the failure
// streak and restores a webhook that a concurrent failure may have disabled.
func (r *WebhookRepo) ResetFailure(ctx context.Context, webhookID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhooks SET failure_count = 0, is_active = TRUE WHERE id = $1`, webhookID)
	return err
}

// ListDeliveries returns recent delivery logs for a webhook.
func (r *WebhookRepo) ListDeliveries(ctx context.Context, webhookID string, limit, offset int) ([]WebhookDelivery, error) {
	rows, err := r.pool.Query(ctx,
		`WITH durable AS (
			SELECT j.id, j.event_id, j.webhook_id, e.event_type, e.payload,
			       COALESCE(last_attempt.status_code, 0) AS status_code,
			       COALESCE(last_attempt.response_body, '') AS response_body,
			       COALESCE(j.last_attempt_at, j.created_at) AS delivered_at,
			       j.status = 'succeeded' AS success,
			       j.status, j.attempt_count,
			       j.status = 'dead' AS terminal,
			       CASE WHEN j.status = 'retry' THEN j.next_attempt_at END AS next_attempt_at
			FROM webhook_delivery_jobs j
			JOIN webhook_outbox_events e ON e.id = j.event_id
			LEFT JOIN LATERAL (
				SELECT status_code, response_body
				FROM webhook_deliveries d
				WHERE d.job_id = j.id
				ORDER BY d.attempt_number DESC
				LIMIT 1
			) last_attempt ON TRUE
			WHERE j.webhook_id = $1
		), direct AS (
			SELECT d.id, d.event_id, d.webhook_id, d.event_type, d.payload,
			       COALESCE(d.status_code, 0) AS status_code,
			       COALESCE(d.response_body, '') AS response_body,
			       d.delivered_at, d.success,
			       CASE WHEN d.success THEN 'succeeded' ELSE 'failed' END AS status,
			       1 AS attempt_count, NOT d.success AS terminal,
			       NULL::TIMESTAMPTZ AS next_attempt_at
			FROM webhook_deliveries d
			WHERE d.webhook_id = $1 AND d.job_id IS NULL
		)
		SELECT * FROM (
			SELECT * FROM durable
			UNION ALL
			SELECT * FROM direct
		) visible
		ORDER BY delivered_at DESC
		LIMIT $2 OFFSET $3`,
		webhookID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []WebhookDelivery
	for rows.Next() {
		var d WebhookDelivery
		var payloadBytes []byte
		if err := rows.Scan(
			&d.ID, &d.EventID, &d.WebhookID, &d.EventType, &payloadBytes,
			&d.StatusCode, &d.ResponseBody, &d.DeliveredAt, &d.Success,
			&d.Status, &d.AttemptCount, &d.Terminal, &d.NextAttemptAt,
		); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		var payloadMap map[string]any
		if err := json.Unmarshal(payloadBytes, &payloadMap); err == nil {
			d.Payload = payloadMap
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// GetByID returns a webhook by ID.
func (r *WebhookRepo) GetByID(ctx context.Context, webhookID string) (*Webhook, error) {
	var w Webhook
	err := r.pool.QueryRow(ctx,
		`SELECT id, participant_id, url, secret, events, is_active, created_at, last_triggered_at, failure_count
         FROM webhooks WHERE id = $1`,
		webhookID,
	).Scan(&w.ID, &w.ParticipantID, &w.URL, &w.Secret, &w.Events, &w.IsActive, &w.CreatedAt, &w.LastTriggeredAt, &w.FailureCount)
	if err != nil {
		return nil, fmt.Errorf("get webhook: %w", err)
	}
	return &w, nil
}
