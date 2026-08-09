package linkpreview

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/surya-koritala/loomfeed/internal/safehttp"
)

type Preview struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image,omitempty"`
	Domain      string `json:"domain"`
	URL         string `json:"url"`
}

// client is SSRF-hardened: it refuses to connect to private/internal/
// metadata addresses (validated at dial time, so DNS-rebinding can't bypass
// it) and re-applies the guard across redirect hops. These fetchers run on
// fully attacker-controlled URLs (link-preview is an unauthenticated
// endpoint), so the guard is the primary defense.
var client = safehttp.NewClient(safehttp.Options{Timeout: 10 * time.Second, MaxRedirects: 5})

func Fetch(rawURL string) (*Preview, error) {
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
	req.Header.Set("User-Agent", "loomfeed/1.0 LinkPreview Bot")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return &Preview{URL: rawURL, Domain: parsed.Host, Title: parsed.Host}, nil
	}

	// Read up to 1MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &Preview{URL: rawURL, Domain: parsed.Host, Title: parsed.Host}, nil
	}

	preview := extractMeta(string(body))
	preview.URL = rawURL
	preview.Domain = parsed.Host

	if preview.Title == "" {
		preview.Title = parsed.Host
	}

	// Resolve relative image URLs (e.g. "/images/hero.jpg") against the
	// page URL. Many sites set og:image to a path instead of an
	// absolute URL, which renders broken when shown from a different
	// origin.
	if preview.Image != "" {
		if imgURL, err := url.Parse(preview.Image); err == nil && !imgURL.IsAbs() {
			preview.Image = parsed.ResolveReference(imgURL).String()
		}
	}

	return preview, nil
}

func extractMeta(htmlContent string) *Preview {
	p := &Preview{}
	tokenizer := html.NewTokenizer(strings.NewReader(htmlContent))

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			token := tokenizer.Token()

			if token.Data == "title" && p.Title == "" {
				// Read the title content
				tokenizer.Next()
				p.Title = strings.TrimSpace(tokenizer.Token().Data)
				continue
			}

			if token.Data == "meta" {
				var property, content string
				for _, attr := range token.Attr {
					switch attr.Key {
					case "property", "name":
						property = attr.Val
					case "content":
						content = attr.Val
					}
				}
				switch property {
				case "og:title":
					if content != "" {
						p.Title = content
					}
				case "og:description", "description":
					if content != "" && p.Description == "" {
						p.Description = content
					}
				case "og:image", "og:image:url", "og:image:secure_url":
					if content != "" {
						p.Image = content
					}
				case "twitter:image", "twitter:image:src":
					// Only use twitter:image if og:image wasn't already
					// set — og is preferred when both are present.
					if content != "" && p.Image == "" {
						p.Image = content
					}
				}
			}
		}
	}
	return p
}
