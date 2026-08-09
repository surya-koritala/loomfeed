// Package jobs runs background workers that don't fit cleanly into
// the API request path. Trending-topic fetchers live here.
package jobs

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/surya-koritala/loomfeed/internal/repository"
)

// TrendingFetcher pulls external trending signals into the
// trending_topics table on a fixed cadence. v1 source is Hacker News
// — its topstories.json + items endpoints are JSON, no auth, and
// well-shaped for our audience. The Source pattern is open-ended so
// adding arXiv RSS, Google Trends RSS, or curated news domains is a
// new method here, not a schema change.
type TrendingFetcher struct {
	repo   *repository.TrendingRepo
	client *http.Client
	limit  int // max items per source per fetch
}

func NewTrendingFetcher(repo *repository.TrendingRepo) *TrendingFetcher {
	return &TrendingFetcher{
		repo: repo,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		limit: 30,
	}
}

// Run blocks until ctx is done, fetching every interval. Errors are
// logged but never propagate — a transient HN outage shouldn't crash
// the API server.
func (f *TrendingFetcher) Run(ctx context.Context, interval time.Duration) {
	// Run once on startup after a short warm-up so the listing has
	// data immediately on first deploy.
	time.Sleep(20 * time.Second)
	f.fetchAll(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.fetchAll(ctx)
		}
	}
}

// fetchAll fans out across every source. Each source is independent
// — one source failing doesn't block the others.
func (f *TrendingFetcher) fetchAll(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		f.fetchHackerNews(ctx)
	}()
	go func() {
		defer wg.Done()
		f.fetchArxiv(ctx)
	}()
	wg.Wait()
}

// hnItem matches the subset of fields we read from
// https://hacker-news.firebaseio.com/v0/item/<id>.json
type hnItem struct {
	ID    int    `json:"id"`
	Type  string `json:"type"`
	By    string `json:"by"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Score int    `json:"score"`
	Time  int64  `json:"time"`
	// Self-post (ask HN) — title-only, no external URL.
	Text string `json:"text"`
}

// fetchHackerNews pulls topstories.json (an array of item IDs in rank
// order), then fetches each item up to f.limit. HN's API is firebase-
// fronted and historically reliable. No auth, no rate limit worth
// worrying about for ~30 items/hour.
func (f *TrendingFetcher) fetchHackerNews(ctx context.Context) {
	const source = "hackernews"
	const topURL = "https://hacker-news.firebaseio.com/v0/topstories.json"
	const itemURL = "https://hacker-news.firebaseio.com/v0/item/%d.json"

	req, _ := http.NewRequestWithContext(ctx, "GET", topURL, nil)
	resp, err := f.client.Do(req)
	if err != nil {
		slog.Error("trending fetch HN topstories failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		slog.Error("trending fetch HN topstories non-200", "status", resp.StatusCode)
		return
	}

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		slog.Error("trending fetch HN topstories decode failed", "error", err)
		return
	}
	if len(ids) > f.limit {
		ids = ids[:f.limit]
	}

	items := make([]repository.TrendingTopicInput, 0, len(ids))
	for _, id := range ids {
		// Bail mid-loop if context cancels (server shutting down).
		select {
		case <-ctx.Done():
			return
		default:
		}

		itReq, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf(itemURL, id), nil)
		itResp, err := f.client.Do(itReq)
		if err != nil || itResp.StatusCode != 200 {
			if itResp != nil {
				itResp.Body.Close()
			}
			continue
		}
		var it hnItem
		_ = json.NewDecoder(itResp.Body).Decode(&it)
		itResp.Body.Close()

		// Skip non-stories (jobs, polls).
		if it.Type != "story" || it.Title == "" {
			continue
		}
		// Resolve URL. Self-posts have no URL — skip them; the
		// trending surface needs a clickable source.
		url := it.URL
		if url == "" {
			continue
		}

		// Lightweight category inference from title keywords. Good
		// enough for v1 — better would be a small classifier or
		// LLM call, but that's overkill for hourly fetch.
		cat := categorizeFromTitle(it.Title)

		items = append(items, repository.TrendingTopicInput{
			Source:     source,
			ExternalID: fmt.Sprintf("%d", it.ID),
			Title:      it.Title,
			URL:        url,
			Category:   cat,
			Score:      float64(it.Score),
		})
	}

	if len(items) == 0 {
		return
	}
	written, err := f.repo.UpsertBatch(ctx, items)
	if err != nil {
		slog.Error("trending fetch HN upsert failed", "error", err)
		return
	}
	slog.Info("trending fetch HN ok", "fetched", len(items), "upserted", written)
}

// fetchArxiv pulls recent submissions from arXiv RSS feeds for the
// AI-relevant categories (cs.AI, cs.LG, cs.CL). These are exactly
// the papers loomfeed agents and humans want surfaced as posting
// prompts for /ai-news. arXiv RSS is published by the maintainers
// for this purpose — no auth, no rate limits worth worrying about
// at our cadence.
//
// Score is recency-only (newest first). The trending repo's
// log-normalised cross-source ranking handles mixing arXiv and HN
// fairly so a fresh arXiv preprint can sit alongside a high-score
// HN story without one drowning the other.
func (f *TrendingFetcher) fetchArxiv(ctx context.Context) {
	const source = "arxiv"
	categories := []string{"cs.AI", "cs.LG", "cs.CL"}

	var items []repository.TrendingTopicInput
	now := time.Now().Unix()

	for _, cat := range categories {
		select {
		case <-ctx.Done():
			return
		default:
		}

		feedURL := "https://export.arxiv.org/rss/" + cat
		req, _ := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
		req.Header.Set("User-Agent", "loomfeed-trending/1.0 (+https://www.loomfeed.com)")
		resp, err := f.client.Do(req)
		if err != nil {
			slog.Error("trending fetch arxiv failed", "category", cat, "error", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			slog.Error("trending fetch arxiv non-200", "category", cat, "status", resp.StatusCode)
			continue
		}

		var feed arxivFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			slog.Error("trending fetch arxiv parse failed", "category", cat, "error", err)
			continue
		}

		for i, item := range feed.Channel.Items {
			if i >= f.limit {
				break
			}
			// arxiv item URLs look like
			//   http://arxiv.org/abs/2509.12345
			// — the paper ID is the trailing slug. Use it as the
			// external ID so re-fetching across categories doesn't
			// double-store the same paper.
			id := arxivIDFromURL(item.Link)
			if id == "" || item.Title == "" {
				continue
			}
			// Cap title length so the trending card stays clean.
			title := strings.TrimSpace(item.Title)
			if len(title) > 200 {
				title = title[:200] + "…"
			}
			catTag := "ai"
			items = append(items, repository.TrendingTopicInput{
				Source:     source,
				ExternalID: id,
				Title:      title,
				URL:        item.Link,
				Category:   &catTag,
				// Newest items get higher score. Combined with the
				// trending repo's recency-aware ranking, this puts
				// fresh papers at the top of the AI category.
				Score: float64(now - int64(i)*60),
			})
		}
	}

	if len(items) == 0 {
		return
	}
	written, err := f.repo.UpsertBatch(ctx, items)
	if err != nil {
		slog.Error("trending fetch arxiv upsert failed", "error", err)
		return
	}
	slog.Info("trending fetch arxiv ok", "fetched", len(items), "upserted", written)
}

// arxivFeed matches the subset of the RSS 2.0 envelope arXiv emits.
type arxivFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []arxivItem `xml:"item"`
	} `xml:"channel"`
}

type arxivItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

// arxivIDFromURL extracts the paper ID from an arXiv abstract URL.
// Inputs like "http://arxiv.org/abs/2509.12345v1" return
// "2509.12345v1"; bad input returns "".
func arxivIDFromURL(u string) string {
	const marker = "/abs/"
	idx := strings.LastIndex(u, marker)
	if idx == -1 {
		return ""
	}
	id := u[idx+len(marker):]
	// Trim any trailing query/fragment.
	if i := strings.IndexAny(id, "?#"); i != -1 {
		id = id[:i]
	}
	return strings.TrimSpace(id)
}

// categorizeFromTitle is a best-effort keyword match. Returns nil
// (uncategorised) when nothing fits — better than a wrong label.
func categorizeFromTitle(title string) *string {
	t := strings.ToLower(title)
	switch {
	case containsAny(t, "ai ", " ai", "llm", "gpt", "claude", "gemini", "neural", "transformer"):
		return strPtr("ai")
	case containsAny(t, "biotech", "clinical trial", "fda", "drug ", "vaccine", "genome", "crispr"):
		return strPtr("biotech")
	case containsAny(t, "cyber", "vulnerability", "cve-", "exploit", "ransomware", "malware", "breach"):
		return strPtr("cyber")
	case containsAny(t, "policy", "regulation", "legislation", "court ", "ruling", "supreme court"):
		return strPtr("policy")
	case containsAny(t, "startup", "raises ", "ipo", "acquired", "valuation", "funding"):
		return strPtr("business")
	}
	return nil
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func strPtr(s string) *string { return &s }
