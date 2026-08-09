package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/safehttp"
)

// Dispatcher sends webhook events to registered HTTP endpoints.
type Dispatcher struct {
	webhooks *repository.WebhookRepo
	client   *http.Client
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(webhooks *repository.WebhookRepo) *Dispatcher {
	return &Dispatcher{
		webhooks: webhooks,
		// SSRF-hardened: URLs are validated at registration, but DNS can be
		// rebound and hosts can 302 to internal IPs between then and delivery.
		// This client re-checks the connected IP at dial time on every hop.
		client: safehttp.NewClient(safehttp.Options{Timeout: 10 * time.Second, MaxRedirects: 3}),
	}
}

// Dispatch sends an event to all subscribed webhooks (async, non-blocking).
func (d *Dispatcher) Dispatch(eventType string, payload map[string]any) {
	go func() {
		ctx := context.Background()
		hooks, err := d.webhooks.ListByEvent(ctx, eventType)
		if err != nil {
			return
		}

		for _, hook := range hooks {
			go d.deliver(ctx, hook, eventType, payload)
		}
	}()
}

func (d *Dispatcher) deliver(ctx context.Context, hook repository.Webhook, eventType string, payload map[string]any) {
	body, _ := json.Marshal(map[string]any{
		"event":     eventType,
		"data":      payload,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})

	req, err := http.NewRequestWithContext(ctx, "POST", hook.URL, bytes.NewReader(body))
	if err != nil {
		_ = d.webhooks.IncrementFailure(ctx, hook.ID)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Loomfeed-Event", eventType)

	// HMAC signature for verification
	mac := hmac.New(sha256.New, []byte(hook.Secret))
	mac.Write(body)
	req.Header.Set("X-Loomfeed-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))

	resp, err := d.client.Do(req)
	success := err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300

	var statusCode int
	var respBody string
	if resp != nil {
		statusCode = resp.StatusCode
		// Read first 1KB of response
		buf := make([]byte, 1024)
		n, _ := resp.Body.Read(buf)
		respBody = string(buf[:n])
		_ = resp.Body.Close()
	}

	_ = d.webhooks.RecordDelivery(ctx, hook.ID, eventType, payload, statusCode, respBody, success)

	if !success {
		_ = d.webhooks.IncrementFailure(ctx, hook.ID)
		// Disable webhook after 10 consecutive failures
		if hook.FailureCount >= 9 {
			_ = d.webhooks.Deactivate(ctx, hook.ID)
		}
	} else {
		_ = d.webhooks.ResetFailure(ctx, hook.ID)
	}
}
