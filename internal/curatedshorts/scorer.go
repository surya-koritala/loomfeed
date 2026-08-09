package curatedshorts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/surya-koritala/loomfeed/internal/quality"
)

// Scorer rates a Video for loomfeed-fit using the same Azure OpenAI
// deployment the rest of the app calls. Returns a score in [0, 1]
// plus a one-sentence rationale the admin queue can surface.
//
// Pattern borrowed from internal/quality/followups.go — same endpoint
// URL shape, same headers, same response parsing. Prompt is the only
// thing that changes.
type Scorer struct {
	cfg  *quality.LLMConfig
	http *http.Client
}

func NewScorer(cfg *quality.LLMConfig) *Scorer {
	return &Scorer{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}}
}

// Enabled mirrors the other LLM callers — empty endpoint or key
// means the whole pipeline silently no-ops (curator ships videos at
// score 0.0 which get filtered by the min-score threshold, so
// nothing reaches the /shorts feed without a real LLM decision).
func (s *Scorer) Enabled() bool {
	return s.cfg != nil && s.cfg.APIKey != "" && s.cfg.Endpoint != ""
}

// Score returns (score, rationale). Always returns nil error — all
// failures degrade to score=0 + rationale="". The admin queue will
// naturally hide those below the min-score threshold.
type Decision struct {
	Score     float64
	Rationale string
}

// LLM response shape we ask for.
type jsonDecision struct {
	Score     float64 `json:"score"`
	Rationale string  `json:"rationale"`
}

func (s *Scorer) Score(ctx context.Context, v Video, categoryDisplayName string) Decision {
	if !s.Enabled() {
		return Decision{}
	}

	// Clamp description so the prompt stays under budget on long
	// uploads. The scorer cares about whether the content is
	// substantive, not whether it reads every link in the box.
	desc := v.Description
	if len(desc) > 1200 {
		desc = desc[:1200] + "..."
	}

	prompt := buildPrompt(v, desc, categoryDisplayName)

	reqBody := map[string]any{
		"messages": []map[string]string{
			{
				"role": "system",
				"content": `You are a strict content curator for loomfeed — a platform for AI researchers, ML engineers, and technically literate readers who value sourced research and substantive debate.

You aggressively reject:
- Tool listicles ("Top 5 AI tools for X", "Best AI for Y")
- "How to use [ChatGPT|Claude|AI] to write your research paper"
- SEO-driven clickbait titles
- Reaction commentary without original analysis
- Tool ads disguised as tutorials
- Generic "AI is changing everything" hot takes
- Surface-level explainer content for absolute beginners
- AI-generated voiceover content with stock footage

You approve:
- Research paper walkthroughs with technical depth
- Engineering/code demos that show real implementation
- Original analysis or arguments with sources
- Authentic creator voice (not stock-footage compilations)
- Working demonstrations of robots, models, or systems

Reply ONLY with a JSON object: {"score": 0.0-1.0, "rationale": "one sentence"}. No surrounding text, no code fences.`,
			},
			{"role": "user", "content": prompt},
		},
		"max_completion_tokens": 200,
		"temperature":           0.2,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return Decision{}
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	url := fmt.Sprintf("%sopenai/deployments/%s/chat/completions?api-version=2024-02-15-preview",
		s.cfg.Endpoint, s.cfg.DeploymentName)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return Decision{}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", s.cfg.APIKey)

	resp, err := s.http.Do(req)
	if err != nil {
		slog.Warn("curatedshorts: LLM http error", "video", v.PlatformID, "err", err)
		return Decision{}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		slog.Warn("curatedshorts: LLM non-2xx",
			"video", v.PlatformID, "status", resp.StatusCode,
			"body", truncate(string(body), 300))
		return Decision{}
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		slog.Warn("curatedshorts: LLM decode failed", "video", v.PlatformID, "err", err)
		return Decision{}
	}
	if len(result.Choices) == 0 {
		slog.Warn("curatedshorts: LLM empty choices", "video", v.PlatformID)
		return Decision{}
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	// Some models wrap JSON in code fences despite the system prompt
	// asking them not to. Strip the fences before parsing.
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Some models still emit prose around the JSON. Fall back to
	// pulling the first {...} block out of the reply before we give
	// up — much cheaper than re-prompting.
	dec, ok := parseDecision(content)
	if !ok {
		slog.Warn("curatedshorts: could not parse LLM reply",
			"video", v.PlatformID, "title", v.Title,
			"raw_reply", truncate(content, 400))
		return Decision{}
	}
	if dec.Score < 0 {
		dec.Score = 0
	}
	if dec.Score > 1 {
		dec.Score = 1
	}
	if len(dec.Rationale) > 280 {
		dec.Rationale = dec.Rationale[:277] + "..."
	}
	slog.Debug("curatedshorts: scored",
		"video", v.PlatformID, "score", dec.Score, "rationale", dec.Rationale)
	return Decision{Score: dec.Score, Rationale: dec.Rationale}
}

// parseDecision is lenient. Tries direct JSON unmarshal first; if
// that fails, hunts for the first {...} block in the reply and tries
// again. Handles replies like `Here is my rating: {"score":0.7,...}`
// that some models still produce despite the system prompt.
func parseDecision(content string) (jsonDecision, bool) {
	var dec jsonDecision
	if err := json.Unmarshal([]byte(content), &dec); err == nil {
		return dec, true
	}
	// Find the outermost {…} and try that.
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		sub := content[start : end+1]
		if err := json.Unmarshal([]byte(sub), &dec); err == nil {
			return dec, true
		}
	}
	return dec, false
}

// truncate is a tiny helper for log lines so we don't dump 4KB
// garbage into slog. Returns at most n chars + "…" marker.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func buildPrompt(v Video, description, category string) string {
	return fmt.Sprintf(`Score this YouTube Short for fit with the %s category on loomfeed.

Apply the system-prompt's reject criteria strictly. If the title or description suggests a tool listicle, "how to write your paper with AI", or generic clickbait — score below 0.3 even if the topic is nominally on-category.

Score distribution:
- 0.9+  Landmark content (rare). Original research demo, paper walkthrough by author, or definitive explainer.
- 0.7   Solid technical or analytical content from a credible voice. Worth a swipe stop.
- 0.5   Decent but unremarkable. Educational without depth.
- 0.3   Tangentially relevant clickbait. Usually a tool ad or surface skim.
- 0.0   Tool listicle, "Best AI for X" content, AI-slop, off-topic.

Title: %s
Channel: %s
Duration: %ds
Views: %d
Description:
%s

JSON only: {"score": 0.0-1.0, "rationale": "one sentence"}`,
		category,
		v.Title,
		v.CreatorName,
		v.DurationSec,
		v.ViewCount,
		description,
	)
}
