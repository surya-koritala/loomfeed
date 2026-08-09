// Package push wraps Web Push delivery. Exposes Send(), which encodes
// a Notification payload, signs a VAPID JWT, and ships it to the
// browser's push endpoint. Gone subscriptions (410/404) should be
// purged by the caller; Send surfaces the HTTP status so repo can.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/RoamXAI/loomfeed/internal/config"
)

// Notification is the JSON we serialize and hand the browser. The
// service worker's `push` handler parses exactly these fields.
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// Subscription is what the browser handed us via /api/v1/push/subscribe.
type Subscription struct {
	Endpoint  string
	P256dhKey string
	AuthKey   string
}

// Sender is usable concurrently. One per process is plenty.
type Sender struct {
	cfg    *config.Config
	client *http.Client
}

func NewSender(cfg *config.Config) *Sender {
	return &Sender{cfg: cfg, client: &http.Client{}}
}

// Enabled returns true if both VAPID keys are configured. Call before
// Send to avoid issuing no-op deliveries.
func (s *Sender) Enabled() bool {
	return s.cfg.Push.PublicKey != "" && s.cfg.Push.PrivateKey != ""
}

// ErrGone signals the subscription endpoint is dead and should be
// deleted. Callers should treat this as "purge, don't retry."
var ErrGone = errors.New("push subscription gone")

// Send ships one notification to one subscription.
func (s *Sender) Send(ctx context.Context, sub Subscription, n Notification) error {
	if !s.Enabled() {
		return nil // silently no-op when unconfigured
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	wpSub := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dhKey,
			Auth:   sub.AuthKey,
		},
	}

	resp, err := webpush.SendNotificationWithContext(ctx, payload, wpSub, &webpush.Options{
		TTL:             30,
		Subscriber:      s.cfg.Push.Subject,
		VAPIDPublicKey:  s.cfg.Push.PublicKey,
		VAPIDPrivateKey: s.cfg.Push.PrivateKey,
		Urgency:         webpush.UrgencyNormal,
		HTTPClient:      s.client,
	})
	if err != nil {
		return fmt.Errorf("webpush send: %w", err)
	}
	defer resp.Body.Close()
	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, bytes.NewReader(nil))
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		return ErrGone
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webpush status %d", resp.StatusCode)
	}
	return nil
}
