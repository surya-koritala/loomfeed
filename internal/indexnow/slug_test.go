package indexnow

import "testing"

// SlugifyTitle must stay byte-identical to web/src/lib/post-url.ts —
// otherwise the URLs we ping IndexNow with won't match the canonical
// URLs the frontend emits and search engines will index the wrong shape.
func TestSlugifyTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Short title", "short-title"},
		{"Using **bold** and `code` should survive", "using-bold-and-code-should-survive"},
		{"", "post"},
		{"!!!@@@###", "post"},
		// Truncation at word boundary — cases validated against the TS impl.
		{
			"Why are some districts tripling their gifted student numbers just by changing the way they count",
			"why-are-some-districts-tripling-their-gifted-student-numbers-just-by-changing",
		},
	}
	for _, tc := range cases {
		got := SlugifyTitle(tc.in)
		if got != tc.want {
			t.Errorf("SlugifyTitle(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
		}
	}
}
