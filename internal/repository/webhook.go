package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	ID           string    `json:"id"`
	WebhookID    string    `json:"webhook_id"`
	EventType    string    `json:"event_type"`
	Payload      any       `json:"payload"`
	StatusCode   int       `json:"status_code"`
	ResponseBody string    `json:"response_body"`
	DeliveredAt  time.Time `json:"delivered_at"`
	Success      bool      `json:"success"`
}

// WebhookRepo handles database operations for webhooks.
type WebhookRepo struct {
	pool *pgxpool.Pool
}

// NewWebhookRepo creates a new WebhookRepo.
func NewWebhookRepo(pool *pgxpool.Pool) *WebhookRepo {
	return &WebhookRepo{pool: pool}
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
func (r *WebhookRepo) RecordDelivery(ctx context.Context, webhookID, eventType string, payload map[string]any, statusCode int, responseBody string, success bool) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO webhook_deliveries (webhook_id, event_type, payload, status_code, response_body, success)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		webhookID, eventType, payloadBytes, statusCode, responseBody, success)
	if err != nil {
		return fmt.Errorf("record delivery: %w", err)
	}
	if success {
		_, _ = r.pool.Exec(ctx,
			`UPDATE webhooks SET last_triggered_at = NOW() WHERE id = $1`, webhookID)
	}
	return nil
}

// IncrementFailure increments the failure count for a webhook.
func (r *WebhookRepo) IncrementFailure(ctx context.Context, webhookID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhooks SET failure_count = failure_count + 1 WHERE id = $1`, webhookID)
	return err
}

// ResetFailure resets the failure count for a webhook to zero.
func (r *WebhookRepo) ResetFailure(ctx context.Context, webhookID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhooks SET failure_count = 0 WHERE id = $1`, webhookID)
	return err
}

// Deactivate sets a webhook as inactive.
func (r *WebhookRepo) Deactivate(ctx context.Context, webhookID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhooks SET is_active = FALSE WHERE id = $1`, webhookID)
	return err
}

// ListDeliveries returns recent delivery logs for a webhook.
func (r *WebhookRepo) ListDeliveries(ctx context.Context, webhookID string, limit, offset int) ([]WebhookDelivery, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, webhook_id, event_type, payload, COALESCE(status_code, 0), COALESCE(response_body, ''), delivered_at, success
         FROM webhook_deliveries
         WHERE webhook_id = $1
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
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.EventType, &payloadBytes, &d.StatusCode, &d.ResponseBody, &d.DeliveredAt, &d.Success); err != nil {
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
