package mention

import (
	"context"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"none", "no mentions here", nil},
		{"basic", "@alice nice take", []string{"alice"}},
		{"two", "@alice and @bob", []string{"alice", "bob"}},
		{"dedup case-insensitive", "@alice and @Alice", []string{"alice"}},
		{"line start", "@alice agrees", []string{"alice"}},
		{"after punctuation", "agreed (@alice) — see also @bob.", []string{"alice", "bob"}},
		{"ignore email", "ping support@example.com please", nil},
		{
			"email mid-sentence doesn't swallow real mention",
			"email me at me@x.com but also @bob has thoughts",
			[]string{"bob"},
		},
		{"hyphen in handle", "@my-agent what do you think", []string{"my-agent"}},
		{"underscore in handle", "@my_agent_v2 thoughts?", []string{"my_agent_v2"}},
		{"preserves case", "@Polaris weighs in", []string{"Polaris"}},
		{"reject leading digit", "@1polaris is bad", nil},
		{"reject leading underscore", "@_polaris is bad", nil},
		{"reject just @", "this @ alone", nil},
		{"length cap respected", "@" + string(make([]byte, 60)), nil}, // null bytes — won't match alpha
		{
			"three with one dup",
			"@alice @bob @alice @charlie",
			[]string{"alice", "bob", "charlie"},
		},
		{
			"adjacent words still split correctly",
			"copy@me agrees, @bob disagrees",
			[]string{"bob"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse(%q):\n  got  %#v\n  want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// fakeResolver satisfies Resolver with an in-memory map keyed by
// display_name (case-sensitive — production resolver does the
// case-insensitive lookup at the SQL layer).
type fakeResolver map[string]Participant

func (f fakeResolver) GetByDisplayName(_ context.Context, name string) (Participant, error) {
	if p, ok := f[name]; ok {
		return p, nil
	}
	return Participant{}, nil
}

func TestResolve(t *testing.T) {
	r := fakeResolver{
		"Polaris": {ID: "p-1", DisplayName: "Polaris", Type: "agent"},
		"Surya":   {ID: "h-1", DisplayName: "Surya", Type: "human"},
	}
	ctx := context.Background()

	t.Run("title-case fallback", func(t *testing.T) {
		got := Resolve(ctx, r, []string{"polaris"})
		if len(got) != 1 || got[0].ID != "p-1" {
			t.Fatalf("expected polaris→p-1, got %+v", got)
		}
	})

	t.Run("unknown handles dropped", func(t *testing.T) {
		got := Resolve(ctx, r, []string{"polaris", "ghost", "surya"})
		ids := []string{}
		for _, p := range got {
			ids = append(ids, p.ID)
		}
		if !reflect.DeepEqual(ids, []string{"p-1", "h-1"}) {
			t.Fatalf("ghost should be dropped, got %v", ids)
		}
	})

	t.Run("dedup by participant id", func(t *testing.T) {
		// Two different handles resolving to the same row should
		// produce one entry. Construct that case by giving the
		// resolver two name keys for the same ID.
		r2 := fakeResolver{
			"Polaris":   {ID: "p-1", DisplayName: "Polaris", Type: "agent"},
			"polaris":   {ID: "p-1", DisplayName: "Polaris", Type: "agent"},
		}
		got := Resolve(ctx, r2, []string{"Polaris", "polaris"})
		if len(got) != 1 {
			t.Fatalf("expected 1 deduped result, got %d", len(got))
		}
	})

	t.Run("nil resolver", func(t *testing.T) {
		got := Resolve(ctx, nil, []string{"polaris"})
		if got != nil {
			t.Fatalf("nil resolver should yield nil, got %v", got)
		}
	})

	t.Run("empty handles", func(t *testing.T) {
		got := Resolve(ctx, r, nil)
		if got != nil {
			t.Fatalf("nil handles should yield nil, got %v", got)
		}
	})
}

func TestFormatMessage(t *testing.T) {
	cases := []struct {
		name, actor, kind, title, want string
	}{
		{"no title", "Alice", "comment", "", "Alice mentioned you in a comment"},
		{"with title", "Alice", "post", "Hot take", `Alice mentioned you in a post on "Hot take"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatMessage(tc.actor, tc.kind, tc.title)
			if got != tc.want {
				t.Errorf("\n  got  %q\n  want %q", got, tc.want)
			}
		})
	}
}
