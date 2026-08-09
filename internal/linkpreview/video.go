package linkpreview

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/RoamXAI/loomfeed/internal/safehttp"
)

// Video is the extracted video reference from a source page.
type Video struct {
	URL    string `json:"video_url"`
	Kind   string `json:"kind"`             // "hls" | "mp4" | "webm" | "mov" | "youtube" | "vimeo" | "iframe"
	Poster string `json:"poster_url,omitempty"`
}

// FetchVideo scrapes the given URL and returns the best inline video
// reference it can find, or nil if no video is present.
//
// Detection order (first match wins):
//  1. <meta property="og:video"> (and og:video:url / :secure_url variants)
//  2. <meta name="twitter:player"> / twitter:player:stream
//  3. The first inline <video src="..."> tag
//  4. The first <source src="..."> nested inside a <video> tag
//  5. The first YouTube/Vimeo <iframe src="...">
//
// Relative paths are resolved against the source URL so the returned
// URL is always absolute.
func FetchVideo(rawURL string) (*Video, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if err := safehttp.ValidateURL(rawURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "loomfeed/1.0 VideoExtract Bot")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("source returned %d", resp.StatusCode)
	}

	// Read up to 1.5MB. Video-heavy pages (newsrooms, news sites) tend
	// to put the <video> tag in the main body, sometimes past the 1MB
	// linkpreview cap.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil {
		return nil, err
	}

	v := extractVideo(string(body), parsed)
	if v == nil || v.URL == "" {
		return nil, nil
	}
	return v, nil
}

func extractVideo(htmlContent string, base *url.URL) *Video {
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))

	var (
		ogVideo       string
		ogVideoType   string
		twitterPlayer string
		twitterStream string
		ogImage       string

		// First <video> tag inputs
		firstVideoSrc string

		// First <source> inside any <video>
		firstSourceSrc string
		insideVideo    bool

		// First useful <iframe> (YouTube/Vimeo only)
		iframeKind string
		iframeURL  string
	)

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := tokenizer.Token()
			switch tok.Data {
			case "meta":
				var prop, name, content string
				for _, a := range tok.Attr {
					switch a.Key {
					case "property":
						prop = a.Val
					case "name":
						name = a.Val
					case "content":
						content = a.Val
					}
				}
				switch prop {
				case "og:video", "og:video:url", "og:video:secure_url":
					if ogVideo == "" {
						ogVideo = content
					}
				case "og:video:type":
					ogVideoType = content
				case "og:image", "og:image:url", "og:image:secure_url":
					if ogImage == "" {
						ogImage = content
					}
				}
				switch name {
				case "twitter:player":
					if twitterPlayer == "" {
						twitterPlayer = content
					}
				case "twitter:player:stream":
					if twitterStream == "" {
						twitterStream = content
					}
				case "twitter:image", "twitter:image:src":
					if ogImage == "" {
						ogImage = content
					}
				}
			case "video":
				insideVideo = true
				if firstVideoSrc == "" {
					for _, a := range tok.Attr {
						if a.Key == "src" {
							firstVideoSrc = a.Val
							break
						}
					}
				}
				if tt == html.SelfClosingTagToken {
					insideVideo = false
				}
			case "source":
				if insideVideo && firstSourceSrc == "" {
					for _, a := range tok.Attr {
						if a.Key == "src" {
							firstSourceSrc = a.Val
							break
						}
					}
				}
			case "iframe":
				if iframeURL == "" {
					var src string
					for _, a := range tok.Attr {
						if a.Key == "src" {
							src = a.Val
							break
						}
					}
					if k := iframeProvider(src); k != "" {
						iframeKind = k
						iframeURL = src
					}
				}
			}
		case html.EndTagToken:
			tok := tokenizer.Token()
			if tok.Data == "video" {
				insideVideo = false
			}
		}
	}

	// Resolve in priority order.
	pick := func(raw, kindHint string) *Video {
		abs := resolveURL(raw, base)
		if abs == "" {
			return nil
		}
		kind := kindHint
		if kind == "" {
			kind = videoKindFromURL(abs)
		}
		return &Video{URL: abs, Kind: kind, Poster: resolveURL(ogImage, base)}
	}

	if twitterStream != "" {
		if v := pick(twitterStream, ""); v != nil {
			return v
		}
	}
	if ogVideo != "" {
		kind := ""
		if strings.Contains(ogVideoType, "mp4") {
			kind = "mp4"
		} else if strings.Contains(ogVideoType, "webm") {
			kind = "webm"
		}
		if v := pick(ogVideo, kind); v != nil {
			return v
		}
	}
	if firstVideoSrc != "" {
		if v := pick(firstVideoSrc, ""); v != nil {
			return v
		}
	}
	if firstSourceSrc != "" {
		if v := pick(firstSourceSrc, ""); v != nil {
			return v
		}
	}
	if twitterPlayer != "" {
		if v := pick(twitterPlayer, "iframe"); v != nil {
			return v
		}
	}
	if iframeURL != "" {
		if v := pick(iframeURL, iframeKind); v != nil {
			return v
		}
	}
	return nil
}

func resolveURL(href string, base *url.URL) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if ref.IsAbs() {
		return ref.String()
	}
	if base == nil {
		return ""
	}
	return base.ResolveReference(ref).String()
}

func videoKindFromURL(absURL string) string {
	low := strings.ToLower(absURL)
	// Strip query string for suffix check.
	if i := strings.Index(low, "?"); i >= 0 {
		low = low[:i]
	}
	switch {
	case strings.HasSuffix(low, ".m3u8"):
		return "hls"
	case strings.HasSuffix(low, ".mp4"):
		return "mp4"
	case strings.HasSuffix(low, ".webm"):
		return "webm"
	case strings.HasSuffix(low, ".mov"):
		return "mov"
	case strings.HasSuffix(low, ".mpd"):
		return "dash"
	default:
		return "unknown"
	}
}

func iframeProvider(src string) string {
	low := strings.ToLower(src)
	switch {
	case strings.Contains(low, "youtube.com/embed/") || strings.Contains(low, "youtube-nocookie.com/embed/"):
		return "youtube"
	case strings.Contains(low, "player.vimeo.com/video/"):
		return "vimeo"
	default:
		return ""
	}
}
