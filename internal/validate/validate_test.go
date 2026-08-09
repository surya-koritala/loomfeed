package validate

import (
	"errors"
	"strings"
	"testing"
)

func TestStringLen(t *testing.T) {
	cases := []struct {
		name, value string
		min, max    int
		wantErr     bool
	}{
		{"in-range", "abcdef", 3, 10, false},
		{"too short", "ab", 3, 10, true},
		{"too long", "abcdefghijklmnop", 3, 10, true},
		{"min-only, ok", "abcdef", 3, 0, false},
		{"min-only, fail", "ab", 3, 0, true},
		{"max-only, ok", "ab", 0, 10, false},
		{"max-only, fail", "abcdefghijklmnop", 0, 10, true},
		// Each "🦀" is 1 rune (2 UTF-16 code units, 4 bytes). Five
		// crabs = 5 runes = passes min=1, max=5. Bytes-counting would
		// give 20 and fail max=5; this asserts we count runes.
		{"runes not bytes", strings.Repeat("🦀", 5), 1, 5, false},
		{"empty allowed when min=0", "", 0, 5, false},
		{"empty rejected when min>=1", "", 1, 5, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := New().StringLen("f", c.value, c.min, c.max).Err()
			if (err != nil) != c.wantErr {
				t.Errorf("StringLen(%q, min=%d, max=%d): err=%v, wantErr=%v", c.value, c.min, c.max, err, c.wantErr)
			}
		})
	}
}

func TestNotEmpty(t *testing.T) {
	if err := New().NotEmpty("f", "abc").Err(); err != nil {
		t.Errorf("non-empty should pass, got %v", err)
	}
	if err := New().NotEmpty("f", "").Err(); err == nil {
		t.Error("empty should fail")
	}
	if err := New().NotEmpty("f", "   \t").Err(); err == nil {
		t.Error("whitespace-only should fail")
	}
}

func TestEmail(t *testing.T) {
	good := []string{
		"x@y.com",
		"user.name+tag@sub.example.co.uk",
	}
	bad := []string{
		"",
		"not-an-email",
		"no-at-sign.com",
		"@missing-local.com",
	}
	for _, e := range good {
		if err := New().Email("email", e).Err(); err != nil {
			t.Errorf("good email %q rejected: %v", e, err)
		}
	}
	for _, e := range bad {
		if err := New().Email("email", e).Err(); err == nil {
			t.Errorf("bad email %q accepted", e)
		}
	}
}

func TestURL(t *testing.T) {
	if err := New().URL("u", "https://example.com").Err(); err != nil {
		t.Errorf("https should pass: %v", err)
	}
	if err := New().URL("u", "https://example.com", "https").Err(); err != nil {
		t.Errorf("https with allowlist should pass: %v", err)
	}
	if err := New().URL("u", "http://example.com", "https").Err(); err == nil {
		t.Error("http with https-only allowlist should fail")
	}
	if err := New().URL("u", "javascript:alert(1)", "http", "https").Err(); err == nil {
		t.Error("javascript: scheme should fail")
	}
	if err := New().URL("u", "not a url").Err(); err == nil {
		t.Error("non-URL should fail")
	}
	if err := New().URL("u", "").Err(); err == nil {
		t.Error("empty should fail")
	}
	if err := New().URL("u", "https://").Err(); err == nil {
		t.Error("URL without host should fail")
	}
}

func TestSliceLen(t *testing.T) {
	if err := New().SliceLen("opts", 3, 2, 10).Err(); err != nil {
		t.Errorf("3 in [2,10] should pass: %v", err)
	}
	if err := New().SliceLen("opts", 1, 2, 10).Err(); err == nil {
		t.Error("1 not in [2,10] should fail (below min)")
	}
	if err := New().SliceLen("opts", 11, 2, 10).Err(); err == nil {
		t.Error("11 not in [2,10] should fail (above max)")
	}
}

func TestIntRange(t *testing.T) {
	if err := New().IntRange("n", 5, 1, 10).Err(); err != nil {
		t.Errorf("5 in [1,10] should pass: %v", err)
	}
	if err := New().IntRange("n", 0, 1, 10).Err(); err == nil {
		t.Error("0 below 1 should fail")
	}
	if err := New().IntRange("n", 11, 1, 10).Err(); err == nil {
		t.Error("11 above 10 should fail")
	}
}

func TestOneOf(t *testing.T) {
	if err := New().OneOf("sort", "hot", "hot", "new", "top").Err(); err != nil {
		t.Errorf("'hot' in allowlist should pass: %v", err)
	}
	if err := New().OneOf("sort", "bogus", "hot", "new", "top").Err(); err == nil {
		t.Error("'bogus' should fail")
	}
}

func TestChainCollectsMultipleErrors(t *testing.T) {
	err := New().
		StringLen("password", "abc", 8, 128).
		Email("email", "not-an-email").
		URL("avatar_url", "javascript:alert(1)", "http", "https").
		Err()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var verr *Error
	if !errors.As(err, &verr) {
		t.Fatalf("want *Error, got %T", err)
	}
	if len(verr.Fields) != 3 {
		t.Errorf("want 3 field errors, got %d: %+v", len(verr.Fields), verr.Fields)
	}
	// Sanity: message identifies the failing field.
	gotFields := make(map[string]string, len(verr.Fields))
	for _, f := range verr.Fields {
		gotFields[f.Field] = f.Message
	}
	for _, f := range []string{"password", "email", "avatar_url"} {
		if _, ok := gotFields[f]; !ok {
			t.Errorf("expected field %q in errors: %+v", f, verr.Fields)
		}
	}
}

func TestErrIsNilWhenAllPass(t *testing.T) {
	err := New().
		Email("e", "x@y.com").
		StringLen("p", "abcdefgh", 8, 128).
		NotEmpty("name", "Alice").
		Err()
	if err != nil {
		t.Errorf("all checks pass should give nil err, got %v", err)
	}
}
