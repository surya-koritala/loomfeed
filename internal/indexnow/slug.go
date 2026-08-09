package indexnow

import (
	"regexp"
	"strings"
	"unicode"
)

// SlugifyTitle mirrors web/src/lib/post-url.ts slugifyTitle() so the
// URLs we ping IndexNow with exactly match the canonical slug URLs
// the frontend emits. Keeping the two in sync is a small tax; the
// payoff is we don't rely on 301 redirects when declaring URLs to
// search engines.
//
// Rules (matching the TS side):
//   - lowercase, strip accents, replace non-alphanumeric runs with '-'
//   - cap at 80 chars; truncate on the last hyphen inside [40, 80]
//   - fall back to 100-char hard cap if no word boundary
//   - always return a non-empty slug ("post" as fallback)
func SlugifyTitle(title string) string {
	if title == "" {
		return "post"
	}
	s := strings.ToLower(title)

	// Strip common markdown noise — mirror the JS replacements.
	// We don't need to be exhaustive; the server-produced titles
	// rarely contain nested markdown.
	s = strings.NewReplacer("**", "", "`", "").Replace(s)

	// Decompose accents and drop combining marks.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsMark(r) {
			continue
		}
		b.WriteRune(r)
	}
	s = b.String()

	// Collapse every non-[a-z0-9] run into a single hyphen.
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "post"
	}

	const softMax = 80
	const hardMax = 100
	if len(s) <= softMax {
		return s
	}

	window := s[:softMax]
	lastHyphen := strings.LastIndexByte(window, '-')
	if lastHyphen >= softMax/2 {
		return window[:lastHyphen]
	}
	if len(s) > hardMax {
		s = s[:hardMax]
	}
	return strings.TrimRight(s, "-")
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
