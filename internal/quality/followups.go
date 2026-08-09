package quality

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GenerateFollowups returns up to 3 one-line question prompts that
// extend a synthesis/article. Empty slice on any failure; callers
// should treat absence as "no suggestions" rather than an error.
func GenerateFollowups(ctx context.Context, title, body string, cfg *LLMConfig) []string {
	if cfg == nil || cfg.APIKey == "" || cfg.Endpoint == "" {
		return nil
	}
	if len(body) < 120 {
		return nil
	}

	truncated := body
	if len(truncated) > 3000 {
		truncated = truncated[:3000] + "..."
	}

	prompt := `Read this post and propose three short follow-up questions a reader might want to explore next. Each question:
- Must be a single sentence ending in "?"
- Must extend — not restate — the post's argument
- Must be specific enough to answer with research (not a vague prompt)
- No numbering, no bullets, no preamble — just three lines

Post title: ` + title + `

Post body:
` + truncated

	reqBody := map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": "You are a research editor. Generate three concrete follow-up questions, one per line, no list markers."},
			{"role": "user", "content": prompt},
		},
		"max_completion_tokens": 200,
		"temperature":           0.5,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	url := fmt.Sprintf("%sopenai/deployments/%s/chat/completions?api-version=2024-02-15-preview",
		cfg.Endpoint, cfg.DeploymentName)

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	if len(result.Choices) == 0 {
		return nil
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	out := []string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		// Strip common list prefixes (1., -, •, *)
		for _, pfx := range []string{"1.", "2.", "3.", "-", "•", "*", "Q:", "—"} {
			if strings.HasPrefix(line, pfx) {
				line = strings.TrimSpace(strings.TrimPrefix(line, pfx))
			}
		}
		if len(line) < 10 || len(line) > 240 {
			continue
		}
		if !strings.HasSuffix(line, "?") {
			continue
		}
		out = append(out, line)
		if len(out) == 3 {
			break
		}
	}
	return out
}
