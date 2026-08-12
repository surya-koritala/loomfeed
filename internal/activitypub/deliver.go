package activitypub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/surya-koritala/loomfeed/internal/safehttp"
)

var deliveryClient = safehttp.NewClient(safehttp.Options{Timeout: 15 * time.Second, MaxRedirects: 3})

// Deliver POSTs a signed ActivityPub activity to a single inbox URL.
// Returns an error on any non-2xx status; caller decides whether to
// retry or purge. keyID is the sender's full publicKey id (e.g.
// https://loomfeed.com/users/alice#main-key), privateKeyPEM is the
// sender's RSA key.
func Deliver(ctx context.Context, inboxURL, keyID, privateKeyPEM string, activity map[string]any) error {
	if err := safehttp.ValidateURL(inboxURL); err != nil {
		return fmt.Errorf("unsafe inbox URL: %w", err)
	}
	body, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("marshal activity: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inboxURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("Accept", "application/activity+json")
	req.Header.Set("User-Agent", "loomfeed/1.0 (+https://www.loomfeed.com)")

	if err := SignRequest(req, keyID, privateKeyPEM, body); err != nil {
		return fmt.Errorf("sign: %w", err)
	}

	resp, err := deliveryClient.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("delivery %d: %s", resp.StatusCode, string(bodySnippet))
	}
	// Drain body so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// FanoutCreate delivers a Create activity for a new post to every
// follower's inbox concurrently (bounded). Logs per-target failures
// but doesn't propagate — one bad follower shouldn't kill the fanout.
func FanoutCreate(
	ctx context.Context,
	followers *FollowersRepo,
	localActorID, keyID, privateKeyPEM string,
	activity map[string]any,
) {
	targets, err := followers.ListForDelivery(ctx, localActorID)
	if err != nil {
		slog.Warn("ap: list followers for delivery failed", "err", err)
		return
	}
	if len(targets) == 0 {
		return
	}
	// Concurrency cap so a single post doesn't open hundreds of
	// connections at once against mismatched servers.
	const maxConcurrent = 8
	sem := make(chan struct{}, maxConcurrent)
	for _, inbox := range targets {
		sem <- struct{}{}
		go func(u string) {
			defer func() { <-sem }()
			deliverCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := Deliver(deliverCtx, u, keyID, privateKeyPEM, activity); err != nil {
				slog.Warn("ap: delivery failed", "inbox", u, "err", err)
			}
		}(inbox)
	}
	// Drain
	for range maxConcurrent {
		sem <- struct{}{}
	}
}
