package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/safehttp"
)

// Dispatcher sends webhook events to registered HTTP endpoints.
type Dispatcher struct {
	webhooks *repository.WebhookRepo
	client   *http.Client
}

// Event is the stable, signed wire envelope delivered to webhook receivers.
type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"event"`
	Data      map[string]any `json:"data"`
	Timestamp string         `json:"timestamp"`
}

// DeliveryResult describes one synchronous delivery attempt.
type DeliveryResult struct {
	EventID    string
	StatusCode int
	Success    bool
}

// WorkerOptions bounds delivery concurrency and controls leases/retries.
type WorkerOptions struct {
	Concurrency  int
	PollInterval time.Duration
	ClaimTTL     time.Duration
	MaxAttempts  int
	Backoff      func(attempt int) time.Duration
}

// DefaultWorkerOptions are conservative production defaults. Destination
// leases additionally enforce one in-flight request per webhook across
// replicas, independent of this per-process worker bound.
func DefaultWorkerOptions() WorkerOptions {
	return WorkerOptions{
		Concurrency:  8,
		PollInterval: 500 * time.Millisecond,
		ClaimTTL:     30 * time.Second,
		MaxAttempts:  6,
		Backoff: func(attempt int) time.Duration {
			delay := time.Minute
			for i := 1; i < attempt && delay < 30*time.Minute; i++ {
				delay *= 2
			}
			if delay > 30*time.Minute {
				return 30 * time.Minute
			}
			return delay
		},
	}
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(webhooks *repository.WebhookRepo) *Dispatcher {
	// SSRF-hardened: URLs are validated at registration, but DNS can be
	// rebound and hosts can 302 to internal IPs between then and delivery.
	// This client re-checks the connected IP at dial time on every hop.
	return NewDispatcherWithClient(webhooks,
		safehttp.NewClient(safehttp.Options{Timeout: 10 * time.Second, MaxRedirects: 3}))
}

// NewDispatcherWithClient creates a dispatcher with an injected HTTP client.
// Production uses NewDispatcher; injection keeps receiver-level tests local.
func NewDispatcherWithClient(webhooks *repository.WebhookRepo, client *http.Client) *Dispatcher {
	if client == nil {
		client = safehttp.NewClient(safehttp.Options{Timeout: 10 * time.Second, MaxRedirects: 3})
	}
	// Webhook delivery has an exact POST-body contract. A 301/302/303 can make
	// net/http follow as GET without the signed envelope, so never follow a
	// redirect and never report the redirect target's 2xx as delivery success.
	clientCopy := *client
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Dispatcher{webhooks: webhooks, client: &clientCopy}
}

// UsesTransactionalOutbox tells source handlers that their repositories have
// already enqueued the event in the originating transaction. Alternate legacy
// dispatchers can omit this marker and continue receiving post-commit calls.
func (d *Dispatcher) UsesTransactionalOutbox() bool { return true }

// Dispatch durably enqueues an event for every current subscriber. Network
// delivery is performed by Run's bounded workers, never by request goroutines.
func (d *Dispatcher) Dispatch(eventType string, payload map[string]any) {
	if _, err := d.webhooks.Enqueue(context.Background(), eventType, payload); err != nil {
		slog.Error("webhook: enqueue failed", "event", eventType, "error", err)
	}
}

// Run processes durable delivery jobs until ctx is canceled. The fixed worker
// count is the per-replica global concurrency bound.
func (d *Dispatcher) Run(ctx context.Context, options WorkerOptions) {
	options = normalizeWorkerOptions(options)
	var workers sync.WaitGroup
	workers.Add(options.Concurrency)
	for i := 0; i < options.Concurrency; i++ {
		go func() {
			defer workers.Done()
			for {
				processed, err := d.ProcessOne(ctx, options)
				if err != nil && ctx.Err() == nil {
					slog.Error("webhook: worker attempt failed", "error", err)
				}
				if processed {
					continue
				}
				timer := time.NewTimer(options.PollInterval)
				select {
				case <-ctx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return
				case <-timer.C:
				}
			}
		}()
	}
	workers.Wait()
}

// ProcessOne claims and completes at most one due job. It is exported so
// operational probes and deterministic multi-worker tests can drive one step.
func (d *Dispatcher) ProcessOne(ctx context.Context, options WorkerOptions) (bool, error) {
	options = normalizeWorkerOptions(options)
	claim, err := d.webhooks.ClaimDelivery(ctx, options.ClaimTTL)
	if err != nil {
		return false, err
	}
	if claim == nil {
		return false, nil
	}
	event := Event{
		ID:        claim.EventID,
		Type:      claim.EventType,
		Data:      claim.Payload,
		Timestamp: claim.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	result, responseBody, deliveryErr := d.send(ctx, claim.Webhook, event)
	retryable := deliveryErr != nil || isRetryableStatus(result.StatusCode)
	retryAt := time.Now().UTC().Add(options.Backoff(claim.AttemptNumber))
	if err := d.webhooks.FinishDelivery(ctx, repository.WebhookAttempt{
		JobID:         claim.JobID,
		ClaimToken:    claim.ClaimToken,
		AttemptNumber: claim.AttemptNumber,
		EventID:       claim.EventID,
		WebhookID:     claim.Webhook.ID,
		EventType:     claim.EventType,
		Payload:       claim.Payload,
		StatusCode:    result.StatusCode,
		ResponseBody:  responseBody,
		Success:       result.Success,
		Retryable:     retryable,
		MaxAttempts:   options.MaxAttempts,
		RetryAt:       retryAt,
	}); err != nil {
		return true, err
	}
	return true, nil
}

func normalizeWorkerOptions(options WorkerOptions) WorkerOptions {
	defaults := DefaultWorkerOptions()
	if options.Concurrency <= 0 {
		options.Concurrency = defaults.Concurrency
	}
	if options.PollInterval <= 0 {
		options.PollInterval = defaults.PollInterval
	}
	if options.ClaimTTL <= 0 {
		options.ClaimTTL = defaults.ClaimTTL
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = defaults.MaxAttempts
	}
	if options.Backoff == nil {
		options.Backoff = defaults.Backoff
	}
	return options
}

func isRetryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly ||
		status == http.StatusTooManyRequests || status >= 500 || status == 0
}

// DeliverTo sends one event directly to the selected hook and waits for the
// attempt to be recorded. It is used by the owned-hook test endpoint; normal
// event fan-out remains asynchronous through Dispatch.
func (d *Dispatcher) DeliverTo(ctx context.Context, hook repository.Webhook, eventType string, payload map[string]any) (DeliveryResult, error) {
	return d.deliver(ctx, hook, newEvent(eventType, payload))
}

func newEvent(eventType string, payload map[string]any) Event {
	return Event{
		ID:        uuid.NewString(),
		Type:      eventType,
		Data:      payload,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (d *Dispatcher) deliver(ctx context.Context, hook repository.Webhook, event Event) (DeliveryResult, error) {
	result, respBody, deliveryErr := d.send(ctx, hook, event)
	recordErr := d.webhooks.RecordDelivery(ctx, event.ID, hook.ID, event.Type, event.Data, result.StatusCode, respBody, result.Success)

	if !result.Success {
		d.recordFailure(ctx, hook.ID)
	} else {
		_ = d.webhooks.ResetFailure(ctx, hook.ID)
	}
	if recordErr != nil {
		return result, recordErr
	}
	if deliveryErr != nil {
		return result, deliveryErr
	}
	return result, nil
}

func (d *Dispatcher) send(ctx context.Context, hook repository.Webhook, event Event) (DeliveryResult, string, error) {
	result := DeliveryResult{EventID: event.ID}
	body, err := json.Marshal(event)
	if err != nil {
		return result, err.Error(), fmt.Errorf("marshal webhook event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewReader(body))
	if err != nil {
		return result, err.Error(), fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Loomfeed-Event", event.Type)
	req.Header.Set("X-Loomfeed-Event-ID", event.ID)

	// HMAC signature for verification
	mac := hmac.New(sha256.New, []byte(hook.Secret))
	mac.Write(body)
	req.Header.Set("X-Loomfeed-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))

	resp, deliveryErr := d.client.Do(req)
	result.Success = deliveryErr == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300

	var respBody string
	if resp != nil {
		result.StatusCode = resp.StatusCode
		// Read first 1KB of response
		responseBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		respBody = string(responseBytes)
		_ = resp.Body.Close()
	} else if deliveryErr != nil {
		respBody = deliveryErr.Error()
	}

	return result, respBody, deliveryErr
}

func (d *Dispatcher) recordFailure(ctx context.Context, webhookID string) {
	_, _, _ = d.webhooks.RecordFailure(ctx, webhookID)
}
