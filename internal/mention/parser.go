// Package mention parses and resolves @mentions inside post and
// comment bodies. The parser is deliberately minimal — match the
// same shape the frontend already renders (one word after an @
// symbol, escaped if the @ comes mid-word like in an email) — and
// the resolver hits ParticipantRepo to confirm each candidate.
//
// The split lets handlers do "parse" cheaply on the request goroutine
// and "resolve + notify" once per author, asynchronously.
package mention

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// mentionRe captures the leading boundary as group 1 so the parser
// can avoid matching the @ inside an email address. The handle
// itself is group 2.
//
// Constraints on the handle:
//   - must start with a letter (so "@1" or "@_x" don't match)
//   - subsequent chars: letters, digits, underscore, hyphen
//   - 1–50 chars total — long enough for any reasonable display name,
//     short enough to bound the resolver's work per body.
var mentionRe = regexp.MustCompile(`(^|[^A-Za-z0-9_.\-])@([A-Za-z][A-Za-z0-9_-]{0,49})`)

// Parse extracts unique mention handles from a body of text. Order
// is preserved (first occurrence wins). Case is preserved as-typed —
// the resolver does the case-insensitive lookup.
//
// Email addresses are skipped: "ping support@example.com" parses
// as no mentions, because the @ is preceded by 't' which fails the
// boundary check.
//
// Code fences and inline code are NOT special-cased. If we ship
// markdown rendering that highlights @x inside code blocks (we
// don't, currently), we'll need to strip those before parsing.
func Parse(body string) []string {
	if body == "" {
		return nil
	}
	matches := mentionRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		// m[1] is the boundary char (or empty for line-start),
		// m[2] is the handle. Lowercase for de-dup so "@Polaris"
		// and "@polaris" don't both notify.
		h := m[2]
		key := strings.ToLower(h)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, h)
	}
	return out
}

// Resolver looks up handles in the participants table. Defined as
// an interface so tests can stub it without touching pgx.
type Resolver interface {
	GetByDisplayName(ctx context.Context, name string) (Participant, error)
}

// Participant is the shape Resolve needs from the participant repo.
// Defined locally to keep this package's import surface small.
type Participant struct {
	ID          string
	DisplayName string
	Type        string
}

// Resolve maps each handle to a participant ID. Unknown handles are
// silently dropped. Returns participants in handle order, deduped by
// ID (so two handles pointing at the same participant only fire one
// notification).
func Resolve(ctx context.Context, r Resolver, handles []string) []Participant {
	if len(handles) == 0 || r == nil {
		return nil
	}
	out := make([]Participant, 0, len(handles))
	seen := make(map[string]struct{}, len(handles))
	for _, h := range handles {
		// Try the handle as-typed first, then a Title-case variant
		// (matches a community convention where display names are
		// capitalized "Polaris" but users type "@polaris").
		p, err := r.GetByDisplayName(ctx, h)
		if err != nil || p.ID == "" {
			alt := titleCase(h)
			if alt != h {
				p, err = r.GetByDisplayName(ctx, alt)
			}
		}
		if err != nil || p.ID == "" {
			continue
		}
		if _, dup := seen[p.ID]; dup {
			continue
		}
		seen[p.ID] = struct{}{}
		out = append(out, p)
	}
	return out
}

// titleCase upper-cases the first rune of s, leaves the rest alone.
// Avoids strings.Title (deprecated) and the unicode-table cost of
// cases.Title for ASCII handles.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	if c := s[0]; c >= 'a' && c <= 'z' {
		return string(c-32) + s[1:]
	}
	return s
}

// FormatMessage builds the notification copy a recipient sees.
// Single source of truth so the message format stays consistent
// across post and comment surfaces.
func FormatMessage(actorName, contentType, postTitle string) string {
	if postTitle == "" {
		return fmt.Sprintf("%s mentioned you in a %s", actorName, contentType)
	}
	return fmt.Sprintf("%s mentioned you in a %s on %q", actorName, contentType, postTitle)
}
