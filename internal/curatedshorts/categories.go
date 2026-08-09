// Package curatedshorts ingests external short-form videos, scores
// them with the existing LLM for loomfeed-fit, and queues them for
// human review before they appear on the /shorts feed.
//
// Day-one source is YouTube Shorts; the package is structured so new
// platforms (Bluesky videos, Mastodon video posts) plug in next to
// youtube.go without rewriting the scoring/curator layers.
package curatedshorts

// Category defines one /shorts lane. Slug is the public-facing id
// (used in URLs, DB rows, UI tabs). DisplayName is what users see.
// SearchQueries are the YouTube search strings we iterate through
// each refresh; tuning live here so the curator layer stays generic.
type Category struct {
	Slug          string
	DisplayName   string
	SearchQueries []string
}

// Day-one categories. Each maps 1:1 to an existing loomfeed
// community so curated shorts feel like an extension, not a
// separate product. Query lists are intentionally short — 2–3
// well-chosen phrases beat 10 mediocre ones on YouTube's search.
var Categories = []Category{
	{
		Slug:        "ai-research",
		DisplayName: "AI research",
		SearchQueries: []string{
			// Aim queries at concepts/architectures. Generic "AI
			// research paper" + "machine learning" wording floods
			// the results with "write your paper with ChatGPT"
			// clickbait. Specific technical terms surface real
			// explainers and demos instead.
			"transformer architecture explained",
			"diffusion model explained",
			"reinforcement learning demo",
		},
	},
	{
		Slug:        "robotics",
		DisplayName: "Robotics",
		SearchQueries: []string{
			"humanoid robot demo",
			"robot dog research",
			"robotic arm project",
		},
	},
	{
		Slug:        "science",
		DisplayName: "Science explainers",
		SearchQueries: []string{
			// Veritasium / Kurzgesagt / Cleo Abram cluster surface
			// here for any of these query shapes.
			"physics paradox explained",
			"biology concept visualized",
			"quantum computing short",
		},
	},
	{
		Slug:        "ml-engineering",
		DisplayName: "ML engineering",
		SearchQueries: []string{
			// Implementation-depth targets — "pytorch from scratch"
			// returns demos; "best AI tools" returns ads.
			"pytorch from scratch",
			"cuda kernel tutorial",
			"llm inference optimization",
		},
	},
	{
		Slug:        "tech-critique",
		DisplayName: "Tech criticism",
		SearchQueries: []string{
			"ai ethics analysis",
			"software industry critique",
			"silicon valley analysis",
		},
	},
}

// CategoryBySlug returns the Category matching slug, or nil. Used by
// handlers that validate query-param category filters.
func CategoryBySlug(slug string) *Category {
	for i, c := range Categories {
		if c.Slug == slug {
			return &Categories[i]
		}
	}
	return nil
}
