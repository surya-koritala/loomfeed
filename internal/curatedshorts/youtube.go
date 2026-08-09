package curatedshorts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Video is the normalized shape the rest of this package works with.
// Whatever platform we ingest from, we map into this before scoring.
type Video struct {
	Platform       string
	PlatformID     string
	Title          string
	Description    string
	CreatorName    string
	CreatorURL     string
	WatchURL       string
	EmbedURL       string
	ThumbnailURL   string
	DurationSec    int
	ViewCount      int64
	PublishedAt    time.Time
}

// YouTubeClient pulls shorts via YouTube Data API v3. The free-tier
// quota is 10,000 units/day; each search.list costs 100 units and
// each videos.list costs 1 unit per video. A full refresh across
// five categories × three queries × 25 results is well inside that.
type YouTubeClient struct {
	apiKey string
	http   *http.Client
}

func NewYouTubeClient(apiKey string) *YouTubeClient {
	return &YouTubeClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled reports whether the client has an API key. An unconfigured
// client silently returns empty results so the whole pipeline
// degrades to a no-op rather than erroring out.
func (c *YouTubeClient) Enabled() bool { return c.apiKey != "" }

type searchListResponse struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
	} `json:"items"`
}

type videosListResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title        string    `json:"title"`
			Description  string    `json:"description"`
			PublishedAt  time.Time `json:"publishedAt"`
			ChannelID    string    `json:"channelId"`
			ChannelTitle string    `json:"channelTitle"`
			Thumbnails   struct {
				High struct {
					URL string `json:"url"`
				} `json:"high"`
				Medium struct {
					URL string `json:"url"`
				} `json:"medium"`
				Default struct {
					URL string `json:"url"`
				} `json:"default"`
			} `json:"thumbnails"`
		} `json:"snippet"`
		ContentDetails struct {
			// ISO 8601 duration, e.g. "PT32S".
			Duration string `json:"duration"`
		} `json:"contentDetails"`
		Statistics struct {
			ViewCount string `json:"viewCount"`
		} `json:"statistics"`
	} `json:"items"`
}

// Search runs a single YouTube search and returns fully-hydrated
// Video objects. Uses the two-step pattern YouTube Data API requires:
//   1. search.list → video IDs (videoDuration=short filters to <4min,
//      which is the strongest duration filter the API exposes;
//      we post-filter to <=60s via contentDetails.duration below).
//   2. videos.list → full metadata + statistics + durations.
//
// "Short" in YouTube's API means <4min; true Shorts (<60s vertical)
// aren't a distinct search filter, so we fetch the short-ish pool
// and drop anything longer than 90s in our own filter. 90 gives us
// a little slack for borderline videos without becoming "any video".
func (c *YouTubeClient) Search(ctx context.Context, query string, maxResults int) ([]Video, error) {
	if !c.Enabled() || query == "" {
		return nil, nil
	}
	if maxResults <= 0 || maxResults > 50 {
		maxResults = 25
	}

	// Step 1: search.list
	sq := url.Values{}
	sq.Set("part", "id")
	sq.Set("q", query)
	sq.Set("type", "video")
	sq.Set("videoDuration", "short")
	sq.Set("order", "relevance")
	sq.Set("safeSearch", "moderate")
	sq.Set("maxResults", strconv.Itoa(maxResults))
	sq.Set("key", c.apiKey)
	searchURL := "https://www.googleapis.com/youtube/v3/search?" + sq.Encode()

	ids, err := c.searchIDs(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Step 2: videos.list for full detail
	vq := url.Values{}
	vq.Set("part", "snippet,contentDetails,statistics")
	vq.Set("id", strings.Join(ids, ","))
	vq.Set("key", c.apiKey)
	videosURL := "https://www.googleapis.com/youtube/v3/videos?" + vq.Encode()

	videos, err := c.fetchVideos(ctx, videosURL)
	if err != nil {
		return nil, fmt.Errorf("videos list: %w", err)
	}
	return videos, nil
}

func (c *YouTubeClient) searchIDs(ctx context.Context, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out searchListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Items))
	for _, it := range out.Items {
		if it.ID.VideoID != "" {
			ids = append(ids, it.ID.VideoID)
		}
	}
	return ids, nil
}

func (c *YouTubeClient) fetchVideos(ctx context.Context, url string) ([]Video, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out videosListResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	videos := make([]Video, 0, len(out.Items))
	for _, it := range out.Items {
		dur := parseISO8601Duration(it.ContentDetails.Duration)
		// Drop anything meaningfully longer than a true Short. 90s
		// gives slack for creators who upload 75-80s shorts that
		// still play in the Shorts player.
		if dur > 90 {
			continue
		}
		views, _ := strconv.ParseInt(it.Statistics.ViewCount, 10, 64)
		thumb := it.Snippet.Thumbnails.High.URL
		if thumb == "" {
			thumb = it.Snippet.Thumbnails.Medium.URL
		}
		if thumb == "" {
			thumb = it.Snippet.Thumbnails.Default.URL
		}
		videos = append(videos, Video{
			Platform:     "youtube",
			PlatformID:   it.ID,
			Title:        it.Snippet.Title,
			Description:  it.Snippet.Description,
			CreatorName:  it.Snippet.ChannelTitle,
			CreatorURL:   "https://www.youtube.com/channel/" + it.Snippet.ChannelID,
			WatchURL:     "https://www.youtube.com/shorts/" + it.ID,
			// youtube-nocookie.com is Google's privacy-enhanced embed
			// host. Behaves identically to youtube.com/embed but avoids
			// dropping tracking cookies until the user actually hits
			// play, which makes it markedly more reliable behind
			// privacy-leaning ad-blockers and tracker-blocker
			// extensions that selectively break the standard embed.
			EmbedURL:     "https://www.youtube-nocookie.com/embed/" + it.ID + "?modestbranding=1&rel=0",
			ThumbnailURL: thumb,
			DurationSec:  dur,
			ViewCount:    views,
			PublishedAt:  it.Snippet.PublishedAt,
		})
	}
	return videos, nil
}

// parseISO8601Duration parses the subset of ISO 8601 that YouTube
// actually emits for Shorts (PT#H#M#S, always starting with PT). For
// videos this short, "PT52S" or "PT1M12S" are the common shapes.
// Returns seconds. Malformed input returns 0, which is treated as
// "unknown" and filtered out downstream.
func parseISO8601Duration(s string) int {
	if !strings.HasPrefix(s, "PT") {
		return 0
	}
	s = s[2:]
	var total int
	var cur int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			cur = cur*10 + int(r-'0')
			continue
		}
		switch r {
		case 'H':
			total += cur * 3600
		case 'M':
			total += cur * 60
		case 'S':
			total += cur
		}
		cur = 0
	}
	return total
}
