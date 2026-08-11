package handlers

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EncodeCursor produces an opaque, base64url-encoded cursor embedding
// a sort-column value and an entity ID. The cursor is stable across
// process restarts and contains no secrets — it just lets the next
// page resume from a deterministic position.
//
// Format (before base64): "<sort_value>|<id>"
//   - float values: shortest exact decimal representation
//   - integer values: base-10
//   - time.Time: RFC3339Nano UTC
//   - anything else: %v fallback (caller's responsibility to be consistent)
//
// The decoder doesn't know the value's type — handlers parse the
// returned string with the type matching their active sort column.
func EncodeCursor(sortValue any, id string) string {
	var s string
	switch v := sortValue.(type) {
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		s = strconv.FormatFloat(float64(v), 'f', -1, 32)
	case int:
		s = strconv.Itoa(v)
	case int64:
		s = strconv.FormatInt(v, 10)
	case time.Time:
		s = v.UTC().Format(time.RFC3339Nano)
	default:
		s = fmt.Sprintf("%v", v)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(s + "|" + id))
}

// DecodeCursor parses a base64url cursor into (sortValueString, id).
// Returns ok=false on any malformed input — handlers should treat
// that as "no cursor" and fall back to the first page.
func DecodeCursor(cursor string) (sortValue, id string, ok bool) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// decodeCursorID validates an opaque cursor and returns its entity ID. Raw
// UUIDs are accepted for one compatibility cycle because the early feed
// implementation exposed its anchor ID directly as next_cursor.
func decodeCursorID(cursor string) string {
	if cursor == "" {
		return ""
	}
	_, id, ok := DecodeCursor(cursor)
	if !ok {
		id = cursor
	}
	if _, err := uuid.Parse(id); err != nil {
		return ""
	}
	return id
}
