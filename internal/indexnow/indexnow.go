// Package indexnow pings the IndexNow protocol endpoint so search
// engines (Bing, Yandex, Naver, Seznam, Yep) learn about content
// changes within seconds instead of waiting for organic sitemap
// crawls. Google doesn't honor IndexNow directly, but the Bing
// signal percolates through parts of Google's graph so it never
// hurts to ping.
//
// Docs: https://www.indexnow.org/documentation
package indexnow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Config holds the site identity IndexNow uses to verify ownership.
// Key is also served at keyLocation (a static file on the site);
// IndexNow fetches that file to confirm the submitter owns the host.
type Config struct {
	// Host is the bare hostname (no scheme, no trailing slash).
	// Example: "www.loomfeed.com".
	Host string
	// Key is the random secret string that must match the content of
	// the file at KeyLocation.
	Key string
	// KeyLocation is the absolute URL of the key file.
	// Example: "https://www.loomfeed.com/<key>.txt".
	KeyLocation string
}

// Default endpoint — the aggregator that fans out to every
// participating engine. Individual engines (bing, yandex) also
// accept direct pings, but using api.indexnow.org is the
// documented one-stop path.
const endpoint = "https://api.indexnow.org/IndexNow"

// Enabled reports whether the config has everything we need. A
// half-configured Sender silently no-ops rather than returning
// errors — pings are advisory and shouldn't break user-facing flows.
func (c Config) Enabled() bool {
	return c.Host != "" && c.Key != "" && c.KeyLocation != ""
}

// Sender ships URL notifications. Safe for concurrent use.
type Sender struct {
	cfg    Config
	client *http.Client
}

func NewSender(cfg Config) *Sender {
	return &Sender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

// Ping submits one or more URLs. IndexNow accepts up to 10,000
// URLs per request; we're well below that in practice. Non-2xx
// responses are logged at debug level and otherwise ignored —
// pinging is fire-and-forget by design.
func (s *Sender) Ping(ctx context.Context, urls []string) error {
	if !s.cfg.Enabled() || len(urls) == 0 {
		return nil
	}
	// Filter to same-host URLs only. IndexNow rejects (422) a
	// batch if any URL's host doesn't match config.Host.
	filtered := make([]string, 0, len(urls))
	for _, u := range urls {
		if strings.Contains(u, "://"+s.cfg.Host) {
			filtered = append(filtered, u)
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	body, err := json.Marshal(map[string]any{
		"host":        s.cfg.Host,
		"key":         s.cfg.Key,
		"keyLocation": s.cfg.KeyLocation,
		"urlList":     filtered,
	})
	if err != nil {
		return fmt.Errorf("marshal indexnow body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build indexnow request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "loomfeed-indexnow/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("indexnow ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		slog.Warn("indexnow ping non-2xx", "status", resp.StatusCode, "count", len(filtered))
		return fmt.Errorf("indexnow status %d", resp.StatusCode)
	}
	slog.Debug("indexnow pinged", "count", len(filtered), "status", resp.StatusCode)
	return nil
}

// Fire launches a Ping on a background goroutine that's detached
// from the request context. Handlers use this so slow or failing
// pings never block the user-facing response.
func (s *Sender) Fire(urls []string) {
	if !s.cfg.Enabled() || len(urls) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.Ping(ctx, urls); err != nil {
			// Already logged inside Ping; no-op here.
			_ = err
		}
	}()
}

// maxURLsPerBatch is the IndexNow-documented cap per POST body.
// Spec allows 10,000; we go a hair below so we never tip over.
const maxURLsPerBatch = 9500

// PingBatched submits an arbitrary-sized URL list by splitting into
// 9500-URL chunks. Returns (submitted, rejected) counts; rejected
// captures URLs in batches that failed (best-effort — a batch
// failure doesn't abort the rest). Use for backfills and sitemap
// regenerations, not per-post hot paths.
func (s *Sender) PingBatched(ctx context.Context, urls []string) (submitted, rejected int, err error) {
	if !s.cfg.Enabled() || len(urls) == 0 {
		return 0, 0, nil
	}
	for i := 0; i < len(urls); i += maxURLsPerBatch {
		end := min(i+maxURLsPerBatch, len(urls))
		batch := urls[i:end]
		if batchErr := s.Ping(ctx, batch); batchErr != nil {
			rejected += len(batch)
			// Log via Ping; continue on. Save the first error so the
			// caller knows *something* went wrong, without masking
			// successful later batches.
			if err == nil {
				err = batchErr
			}
			continue
		}
		submitted += len(batch)
		// Brief pause between batches to avoid IndexNow 429s when
		// backfilling tens of thousands of URLs.
		if end < len(urls) {
			select {
			case <-ctx.Done():
				return submitted, rejected, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	return submitted, rejected, err
}
