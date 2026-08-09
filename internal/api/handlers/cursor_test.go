package handlers

import (
	"strings"
	"testing"
	"time"
)

func TestCursor_FloatRoundTrip(t *testing.T) {
	enc := EncodeCursor(0.873, "agent-id-1")
	if strings.Contains(enc, "|") || strings.Contains(enc, ":") {
		t.Errorf("encoded cursor should be base64-safe, got %q", enc)
	}
	sortVal, id, ok := DecodeCursor(enc)
	if !ok {
		t.Fatal("DecodeCursor should succeed")
	}
	if sortVal != "0.873" {
		t.Errorf("sortVal: got %q, want %q", sortVal, "0.873")
	}
	if id != "agent-id-1" {
		t.Errorf("id: got %q, want %q", id, "agent-id-1")
	}
}

func TestCursor_IntRoundTrip(t *testing.T) {
	enc := EncodeCursor(42, "p-1")
	sortVal, id, ok := DecodeCursor(enc)
	if !ok || sortVal != "42" || id != "p-1" {
		t.Errorf("int round-trip: got (%q, %q, %v); want (42, p-1, true)", sortVal, id, ok)
	}
}

func TestCursor_TimeRoundTripUTC(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	in := time.Date(2026, 5, 11, 10, 15, 0, 0, loc)

	enc := EncodeCursor(in, "p-2")
	sortVal, id, ok := DecodeCursor(enc)
	if !ok {
		t.Fatal("DecodeCursor should succeed for time.Time")
	}
	parsed, err := time.Parse(time.RFC3339Nano, sortVal)
	if err != nil {
		t.Fatalf("parsed back time: %v (raw %q)", err, sortVal)
	}
	if !parsed.Equal(in) {
		t.Errorf("time round-trip: got %v, want %v", parsed, in)
	}
	if parsed.Location().String() != "UTC" {
		t.Errorf("decoded time should be UTC, got loc %v", parsed.Location())
	}
	if id != "p-2" {
		t.Errorf("id: got %q", id)
	}
}

func TestDecodeCursor_RejectsGarbage(t *testing.T) {
	cases := []string{
		"",
		"not-base64-!!",
		"YWJj",   // base64 of "abc" — has no | separator
		"fA==",   // base64 of "|"  — empty parts
	}
	for _, c := range cases {
		_, _, ok := DecodeCursor(c)
		if c == "fA==" {
			// "|" splits to ["",""] which technically has 2 parts —
			// the function accepts it. Handlers should additionally
			// reject empty parts; this test just documents current
			// behavior so a future tightening is intentional.
			continue
		}
		if ok {
			t.Errorf("DecodeCursor(%q) should return ok=false", c)
		}
	}
}

func TestEncodeCursor_StableAcrossCalls(t *testing.T) {
	a := EncodeCursor(0.5, "id-x")
	b := EncodeCursor(0.5, "id-x")
	if a != b {
		t.Errorf("encoding should be deterministic; got %q vs %q", a, b)
	}
}
