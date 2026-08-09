package linkpreview

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync"
)

// Matches both `[text](https://...)` markdown links and bare `https://...`
// URLs. We strip trailing punctuation that commonly bleeds in from prose.
var (
	mdLinkRE   = regexp.MustCompile(`\[[^\]]*\]\((https?://[^\s)]+)\)`)
	bareLinkRE = regexp.MustCompile(`(?:^|\s)(https?://[^\s<>"']+)`)
)

// ExtractURLs returns every http(s) URL referenced in the markdown body
// that's NOT an inline image (`![alt](url)`). De-duplicated, order-preserving.
func ExtractURLs(body string) []string {
	// Drop inline images so we don't treat their src URLs as link previews.
	stripped := regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`).ReplaceAllString(body, "")

	seen := map[string]struct{}{}
	out := []string{}
	add := func(raw string) {
		u := strings.TrimRight(raw, ".,;:!?)]}>")
		if u == "" {
			return
		}
		// Skip direct image URLs — they render as <img>, not as preview cards.
		lower := strings.ToLower(u)
		if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
			strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".gif") ||
			strings.HasSuffix(lower, ".webp") || strings.HasSuffix(lower, ".svg") {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	for _, m := range mdLinkRE.FindAllStringSubmatch(stripped, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}
	for _, m := range bareLinkRE.FindAllStringSubmatch(stripped, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}
	return out
}

// FetchMany resolves previews for every URL concurrently, capped at
// `parallelism`. Failures are silently skipped — the caller stores only
// successful results. The context bounds the whole operation.
func FetchMany(ctx context.Context, urls []string, parallelism int) map[string]*Preview {
	if parallelism < 1 {
		parallelism = 3
	}
	if len(urls) == 0 {
		return map[string]*Preview{}
	}

	results := make(map[string]*Preview, len(urls))
	var mu sync.Mutex
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for _, u := range urls {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(url string) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			p, err := Fetch(url)
			if err != nil {
				slog.Debug("body link preview fetch failed", "url", url, "err", err)
				return
			}
			// Only store if we got something useful beyond domain fallback.
			if p == nil || (p.Image == "" && p.Description == "" && (p.Title == "" || p.Title == p.Domain)) {
				return
			}
			mu.Lock()
			results[url] = p
			mu.Unlock()
		}(u)
	}
	wg.Wait()
	return results
}
