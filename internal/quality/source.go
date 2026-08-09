package quality

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SourceResult holds the validation result for a single URL.
type SourceResult struct {
	URL           string
	Domain        string
	Status        string // "verified", "unverified", "invalid", "blocked"
	HTTPStatus    int
	ContentType   string
	PageTitle     string
	TitleMatch    bool
	BlockedReason string
}

// trustedDomains is loaded from the database on startup by the Checker.
// It's a package-level cache that gets refreshed.
var (
	trustedDomains   = make(map[string]bool)
	trustedDomainsMu sync.RWMutex
)

// SetTrustedDomains replaces the cached trusted domain list.
func SetTrustedDomains(domains map[string]bool) {
	trustedDomainsMu.Lock()
	trustedDomains = domains
	trustedDomainsMu.Unlock()
}

// isTrustedDomain checks if a domain (or its parent) is in the trusted list.
func isTrustedDomain(domain string) bool {
	trustedDomainsMu.RLock()
	defer trustedDomainsMu.RUnlock()

	if trustedDomains[domain] {
		return true
	}
	// Check parent domain (e.g., "blog.npr.org" → "npr.org")
	parts := strings.Split(domain, ".")
	if len(parts) > 2 {
		parent := strings.Join(parts[len(parts)-2:], ".")
		return trustedDomains[parent]
	}
	return false
}

var titleRe = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
var ogTitleRe = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`)
var ogDescRe = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']+)["']`)

// ValidateSource checks a single URL for validity.
// postBody is used for title/content matching.
func ValidateSource(ctx context.Context, rawURL string, postBody string) SourceResult {
	result := SourceResult{URL: rawURL}

	// Parse URL
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		result.Status = "invalid"
		result.BlockedReason = "malformed URL"
		return result
	}

	result.Domain = strings.ToLower(parsed.Hostname())

	// Check blocklist
	if blocked, reason := IsBlocked(result.Domain); blocked {
		result.Status = "blocked"
		result.BlockedReason = reason
		return result
	}

	// Check if domain is a well-known trusted source — if so, mark as verified
	// even if we can't access it (many major sites block datacenter IPs).
	if isTrustedDomain(result.Domain) {
		result.Status = "verified"
		result.TitleMatch = true
		return result
	}

	// Go straight to limited GET for better compatibility.
	// Many sites block HEAD requests or return 403 for bot User-Agents.
	result = fetchAndMatch(ctx, rawURL, postBody, result)

	return result
}

// stripTrackingParams removes common tracking query parameters (utm_*, fbclid, etc.)
// and returns the cleaned URL.
func stripTrackingParams(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	trackingPrefixes := []string{"utm_", "fbclid", "gclid", "mc_", "ref", "source"}
	for key := range q {
		keyLower := strings.ToLower(key)
		for _, prefix := range trackingPrefixes {
			if strings.HasPrefix(keyLower, prefix) {
				q.Del(key)
				break
			}
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// fetchAndMatch does a limited GET to extract page title and compare with post body.
func fetchAndMatch(ctx context.Context, rawURL string, postBody string, result SourceResult) SourceResult {
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		result.Status = "unverified"
		return result
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		// Distinguish DNS/connection failures from other errors
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "no such host") ||
			strings.Contains(errStr, "connection refused") ||
			strings.Contains(errStr, "dial tcp") {
			result.Status = "invalid"
			result.BlockedReason = "request failed: " + err.Error()
		} else {
			result.Status = "unverified"
			result.BlockedReason = "request failed"
		}
		return result
	}
	defer resp.Body.Close()

	result.HTTPStatus = resp.StatusCode
	result.ContentType = resp.Header.Get("Content-Type")

	// Handle 403: retry once with stripped tracking params
	if resp.StatusCode == 403 {
		resp.Body.Close()
		cleanURL := stripTrackingParams(rawURL)
		retryReq, err := http.NewRequestWithContext(ctx, "GET", cleanURL, nil)
		if err == nil {
			retryReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			retryReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
			retryReq.Header.Set("Accept-Language", "en-US,en;q=0.5")

			retryResp, retryErr := client.Do(retryReq)
			if retryErr == nil {
				defer retryResp.Body.Close()
				if retryResp.StatusCode < 400 {
					// Retry succeeded, continue with the retry response
					resp = retryResp
					result.HTTPStatus = resp.StatusCode
					result.ContentType = resp.Header.Get("Content-Type")
				} else {
					retryResp.Body.Close()
				}
			}
		}

		// Still 403 after retry: mark as "unverified" (site blocks bots, URL may be valid)
		if result.HTTPStatus == 403 {
			result.Status = "unverified"
			result.BlockedReason = "site blocks automated access (403)"
			return result
		}
	}

	if resp.StatusCode >= 400 {
		// Only mark as "invalid" for definitive failures: 404 and 410
		if resp.StatusCode == 404 || resp.StatusCode == 410 {
			result.Status = "invalid"
			result.BlockedReason = fmt.Sprintf("HTTP %d", resp.StatusCode)
		} else {
			result.Status = "unverified"
			result.BlockedReason = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return result
	}

	// Non-HTML content (PDF, image, etc.) — URL resolves, mark as unverified
	if !strings.Contains(result.ContentType, "text/html") && result.ContentType != "" {
		result.Status = "unverified"
		return result
	}

	// Read first 64KB only
	limited := io.LimitReader(resp.Body, 64*1024)
	body, err := io.ReadAll(limited)
	if err != nil {
		result.Status = "unverified"
		return result
	}

	html := string(body)

	// Extract title
	if m := titleRe.FindStringSubmatch(html); len(m) > 1 {
		result.PageTitle = strings.TrimSpace(m[1])
	}
	if result.PageTitle == "" {
		if m := ogTitleRe.FindStringSubmatch(html); len(m) > 1 {
			result.PageTitle = strings.TrimSpace(m[1])
		}
	}

	// Extract OG description for matching
	var ogDesc string
	if m := ogDescRe.FindStringSubmatch(html); len(m) > 1 {
		ogDesc = strings.TrimSpace(m[1])
	}

	// Match: check if page title or OG description terms overlap with post body
	pageText := strings.ToLower(result.PageTitle + " " + ogDesc)
	postLower := strings.ToLower(postBody)

	if pageText != "" && termOverlap(pageText, postLower) > 0.15 {
		result.Status = "verified"
		result.TitleMatch = true
	} else if result.PageTitle != "" {
		result.Status = "unverified"
		result.TitleMatch = false
	} else {
		result.Status = "unverified"
	}

	return result
}

// termOverlap computes Jaccard-like overlap between significant words in two texts.
func termOverlap(a, b string) float64 {
	wordsA := significantWords(a)
	wordsB := significantWords(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		setB[w] = true
	}

	matches := 0
	for _, w := range wordsA {
		if setB[w] {
			matches++
		}
	}

	return float64(matches) / float64(len(wordsA))
}

// significantWords extracts words >= 4 chars, excluding common stop words.
func significantWords(text string) []string {
	stopWords := map[string]bool{
		"the": true, "and": true, "for": true, "are": true, "but": true,
		"not": true, "you": true, "all": true, "can": true, "had": true,
		"her": true, "was": true, "one": true, "our": true, "out": true,
		"this": true, "that": true, "with": true, "have": true, "from": true,
		"they": true, "been": true, "said": true, "each": true, "which": true,
		"their": true, "will": true, "other": true, "about": true, "many": true,
		"then": true, "them": true, "these": true, "some": true, "would": true,
		"make": true, "like": true, "into": true, "more": true, "also": true,
	}

	words := strings.Fields(text)
	var result []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}—-")
		if len(w) >= 4 && !stopWords[w] {
			result = append(result, w)
		}
	}
	return result
}
