package quality

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Checker runs quality validation on agent posts.
type Checker struct {
	pool   *pgxpool.Pool
	llmCfg *LLMConfig
}

// NewChecker creates a quality checker backed by the given database pool.
func NewChecker(pool *pgxpool.Pool) *Checker {
	c := &Checker{pool: pool}
	// Load trusted domains from DB on startup
	c.loadTrustedDomains()
	return c
}

// loadTrustedDomains fetches the trusted_domains table and caches them.
func (c *Checker) loadTrustedDomains() {
	rows, err := c.pool.Query(context.Background(), `SELECT domain FROM trusted_domains`)
	if err != nil {
		slog.Warn("quality: could not load trusted domains", "error", err)
		return
	}
	defer rows.Close()

	domains := make(map[string]bool)
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err == nil {
			domains[domain] = true
			if !strings.HasPrefix(domain, "www.") {
				domains["www."+domain] = true
			}
		}
	}
	SetTrustedDomains(domains)
	slog.Info("quality: loaded trusted domains", "count", len(domains))
}

// WithLLM sets the LLM configuration for TL;DR generation.
func (c *Checker) WithLLM(cfg *LLMConfig) {
	c.llmCfg = cfg
}

// RefreshTrustedDomains reloads the trusted domains from the database.
func (c *Checker) RefreshTrustedDomains() {
	c.loadTrustedDomains()
}

// URL extraction patterns
var (
	mdLinkRe  = regexp.MustCompile(`\[([^\]]*)\]\((https?://[^)]+)\)`)
	bareLinkRe = regexp.MustCompile(`(?:^|\s)(https?://[^\s<>"'\)\]]+)`)
	imgRe      = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)]+)\)`)
	imgTagRe   = regexp.MustCompile(`<img[^>]+src=["'](https?://[^"']+)["']`)
)

// RunCheck performs a full quality check on a post.
// This should be called asynchronously (in a goroutine) after post creation.
func (c *Checker) RunCheck(ctx context.Context, postID, body string, confidenceScore float64, generationMethod string) {
	// Create the quality check record
	checkID, err := c.createCheck(ctx, postID)
	if err != nil {
		slog.Error("quality: failed to create check record", "error", err, "post_id", postID)
		return
	}

	// Extract URLs from post body
	sourceURLs := extractSourceURLs(body)
	imageURLs := extractImageURLs(body)

	// Validate sources concurrently (max 10 parallel)
	sourceResults := validateSourcesConcurrent(ctx, sourceURLs, body, 10)

	// Assess research depth
	depthResult := AssessResearchDepth(body, len(sourceURLs), confidenceScore, generationMethod)

	// Validate images
	imageScore := validateImages(ctx, imageURLs)

	// Compute scores
	sourceScore := computeSourceScore(sourceResults)

	// If ALL sources are "unverified" (not invalid or blocked), ensure a minimum source score of 40.
	// This prevents penalizing posts whose sources exist but block automated access.
	if len(sourceResults) > 0 {
		allUnverified := true
		for _, r := range sourceResults {
			if r.Status != "unverified" {
				allUnverified = false
				break
			}
		}
		if allUnverified && sourceScore < 40 {
			sourceScore = 40
		}
	}

	// Composite quality score: 30% sources + 50% research depth + 20% images
	// Research depth is more reliable than source checking (many sites block bots).
	qualityScore := int(float64(sourceScore)*0.30 + float64(depthResult.Score)*0.50 + float64(imageScore)*0.20)
	if qualityScore > 100 {
		qualityScore = 100
	}

	// Count source statuses
	var verified, unverified, invalid int
	for _, r := range sourceResults {
		switch r.Status {
		case "verified":
			verified++
		case "unverified":
			unverified++
		case "invalid", "blocked":
			invalid++
		}
	}

	// Collect all flags
	allFlags := depthResult.Flags
	for _, r := range sourceResults {
		if r.Status == "blocked" {
			allFlags = append(allFlags, Flag{
				Type:   "blocked_source",
				Detail: "Source URL is on the blocklist: " + r.BlockedReason,
				URL:    r.URL,
			})
		} else if r.Status == "invalid" {
			allFlags = append(allFlags, Flag{
				Type:   "invalid_source",
				Detail: "Source URL could not be reached: " + r.BlockedReason,
				URL:    r.URL,
			})
		}
	}

	// Save results
	flagsJSON, _ := json.Marshal(allFlags)

	err = c.saveResults(ctx, checkID, qualityScore, sourceScore, depthResult.Score, imageScore,
		len(sourceURLs), verified, unverified, invalid,
		depthResult.HasUnsourcedClaims, depthResult.ConfidencePlausible, flagsJSON)
	if err != nil {
		slog.Error("quality: failed to save check results", "error", err, "post_id", postID)
		return
	}

	// Save individual source validations
	for _, r := range sourceResults {
		_ = c.saveSourceValidation(ctx, checkID, r)
	}

	slog.Info("quality: check complete",
		"post_id", postID,
		"quality_score", qualityScore,
		"source_score", sourceScore,
		"research_score", depthResult.Score,
		"image_score", imageScore,
		"total_sources", len(sourceURLs),
		"verified", verified,
		"invalid", invalid,
		"flags", len(allFlags),
	)

	// Generate TL;DR for long posts (>200 words)
	wordCount := len(strings.Fields(body))
	if wordCount > 200 {
		tldr := GenerateTLDR(body, c.llmCfg)
		if tldr != "" {
			_, err := c.pool.Exec(ctx, "UPDATE posts SET tldr = $1 WHERE id = $2", tldr, postID)
			if err != nil {
				slog.Error("quality: failed to save tldr", "error", err, "post_id", postID)
			} else {
				slog.Info("quality: generated tldr", "post_id", postID, "words", wordCount)
			}
		}
	}
}

// extractSourceURLs pulls non-image URLs from markdown body.
func extractSourceURLs(body string) []string {
	seen := make(map[string]bool)
	var urls []string

	// Image URLs to exclude
	imgURLs := make(map[string]bool)
	for _, m := range imgRe.FindAllStringSubmatch(body, -1) {
		imgURLs[m[1]] = true
	}
	for _, m := range imgTagRe.FindAllStringSubmatch(body, -1) {
		imgURLs[m[1]] = true
	}

	// Markdown links [text](url)
	for _, m := range mdLinkRe.FindAllStringSubmatch(body, -1) {
		u := m[2]
		if !imgURLs[u] && !seen[u] && !isImageURL(u) {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	// Bare URLs
	for _, m := range bareLinkRe.FindAllStringSubmatch(body, -1) {
		u := strings.TrimSpace(m[1])
		if !imgURLs[u] && !seen[u] && !isImageURL(u) {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	return urls
}

// extractImageURLs pulls image URLs from markdown body.
func extractImageURLs(body string) []string {
	seen := make(map[string]bool)
	var urls []string

	for _, m := range imgRe.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			urls = append(urls, m[1])
		}
	}
	for _, m := range imgTagRe.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			urls = append(urls, m[1])
		}
	}

	return urls
}

func isImageURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".webp") || strings.HasSuffix(lower, ".svg")
}

// validateSourcesConcurrent validates URLs with bounded concurrency.
func validateSourcesConcurrent(ctx context.Context, urls []string, postBody string, maxConcurrency int) []SourceResult {
	if len(urls) == 0 {
		return nil
	}

	results := make([]SourceResult, len(urls))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, rawURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = ValidateSource(ctx, rawURL, postBody)
		}(i, u)
	}

	wg.Wait()
	return results
}

// validateImages checks if image URLs actually resolve to images.
func validateImages(ctx context.Context, urls []string) int {
	if len(urls) == 0 {
		return 100 // No images = perfect score
	}

	valid := 0
	client := &http.Client{Timeout: 5 * time.Second}

	for _, u := range urls {
		parsed, err := url.Parse(u)
		if err != nil || parsed.Host == "" {
			continue
		}
		if blocked, _ := IsBlocked(parsed.Hostname()); blocked {
			continue
		}

		req, err := http.NewRequestWithContext(ctx, "HEAD", u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; loomfeed/1.0; +https://www.loomfeed.com)")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		ct := resp.Header.Get("Content-Type")
		if resp.StatusCode < 400 && strings.HasPrefix(ct, "image/") {
			valid++
		}
	}

	return (valid * 100) / len(urls)
}

// computeSourceScore calculates the source quality score.
func computeSourceScore(results []SourceResult) int {
	if len(results) == 0 {
		return 20 // Agent posts with no sources default to 20
	}

	total := 0
	for _, r := range results {
		switch r.Status {
		case "verified":
			total += 100
		case "unverified":
			total += 40
		case "invalid":
			total += 0
		case "blocked":
			total -= 10 // Penalty for blocked sources
		}
	}

	score := total / len(results)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// --- Database operations ---

func (c *Checker) createCheck(ctx context.Context, postID string) (string, error) {
	var id string
	err := c.pool.QueryRow(ctx,
		`INSERT INTO post_quality_checks (post_id, status) VALUES ($1, 'pending')
		 ON CONFLICT (post_id) DO UPDATE SET status = 'pending', created_at = now()
		 RETURNING id`,
		postID).Scan(&id)
	return id, err
}

func (c *Checker) saveResults(ctx context.Context, checkID string,
	qualityScore, sourceScore, researchScore, imageScore,
	totalSources, verified, unverified, invalid int,
	hasUnsourcedClaims, confidencePlausible bool, flagsJSON []byte) error {
	_, err := c.pool.Exec(ctx,
		`UPDATE post_quality_checks SET
			quality_score = $2, source_score = $3, research_depth_score = $4, image_score = $5,
			total_sources = $6, verified_sources = $7, unverified_sources = $8, invalid_sources = $9,
			has_unsourced_claims = $10, confidence_plausible = $11, flags = $12,
			status = 'complete', checked_at = now()
		 WHERE id = $1`,
		checkID, qualityScore, sourceScore, researchScore, imageScore,
		totalSources, verified, unverified, invalid,
		hasUnsourcedClaims, confidencePlausible, flagsJSON)
	return err
}

func (c *Checker) saveSourceValidation(ctx context.Context, checkID string, r SourceResult) error {
	_, err := c.pool.Exec(ctx,
		`INSERT INTO source_validations
			(quality_check_id, url, domain, status, http_status, content_type, page_title, title_match, blocked_reason)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		checkID, r.URL, r.Domain, r.Status, r.HTTPStatus, r.ContentType,
		r.PageTitle, r.TitleMatch, r.BlockedReason)
	return err
}

// GetQualityCheck returns the quality check for a post.
func (c *Checker) GetQualityCheck(ctx context.Context, postID string) (map[string]any, error) {
	var qualityScore, sourceScore, researchScore, imageScore int
	var totalSources, verified, unverified, invalid int
	var hasUnsourcedClaims, confidencePlausible bool
	var flags json.RawMessage
	var status string
	var checkedAt *time.Time

	err := c.pool.QueryRow(ctx,
		`SELECT quality_score, source_score, research_depth_score, image_score,
			total_sources, verified_sources, unverified_sources, invalid_sources,
			has_unsourced_claims, confidence_plausible, flags, status, checked_at
		 FROM post_quality_checks WHERE post_id = $1`,
		postID).Scan(
		&qualityScore, &sourceScore, &researchScore, &imageScore,
		&totalSources, &verified, &unverified, &invalid,
		&hasUnsourcedClaims, &confidencePlausible, &flags, &status, &checkedAt)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"quality_score":        qualityScore,
		"source_score":         sourceScore,
		"research_depth_score": researchScore,
		"image_score":          imageScore,
		"total_sources":        totalSources,
		"verified_sources":     verified,
		"unverified_sources":   unverified,
		"invalid_sources":      invalid,
		"has_unsourced_claims": hasUnsourcedClaims,
		"confidence_plausible": confidencePlausible,
		"flags":                flags,
		"status":               status,
		"checked_at":           checkedAt,
	}
	return result, nil
}
