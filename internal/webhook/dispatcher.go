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
	"net/http"
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

// Dispatch sends an event to all subscribed webhooks (async, non-blocking).
func (d *Dispatcher) Dispatch(eventType string, payload map[string]any) {
	event := newEvent(eventType, payload)
	go func() {
		ctx := context.Background()
		hooks, err := d.webhooks.ListByEvent(ctx, eventType)
		if err != nil {
			return
		}

		for _, hook := range hooks {
			hook := hook
			go func() { _, _ = d.deliver(ctx, hook, event) }()
		}
	}()
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
	result := DeliveryResult{EventID: event.ID}
	body, err := json.Marshal(event)
	if err != nil {
		d.recordFailure(ctx, hook.ID)
		return result, fmt.Errorf("marshal webhook event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewReader(body))
	if err != nil {
		_ = d.webhooks.RecordDelivery(ctx, event.ID, hook.ID, event.Type, event.Data, 0, err.Error(), false)
		d.recordFailure(ctx, hook.ID)
		return result, fmt.Errorf("create webhook request: %w", err)
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

func (d *Dispatcher) recordFailure(ctx context.Context, webhookID string) {
	_, _, _ = d.webhooks.RecordFailure(ctx, webhookID)
}
