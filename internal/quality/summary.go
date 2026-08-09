package quality

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
)

// LLMConfig holds Azure OpenAI configuration for TL;DR generation.
type LLMConfig struct {
	Endpoint       string
	APIKey         string
	DeploymentName string
}

// GenerateTLDR creates a rich-text TL;DR summary using Azure OpenAI.
// Falls back to extractive summary if LLM is not configured or fails.
func GenerateTLDR(body string, llmCfg *LLMConfig) string {
	wordCount := len(strings.Fields(body))
	if wordCount < 200 {
		return ""
	}

	// Try LLM-powered summary first
	if llmCfg != nil && llmCfg.APIKey != "" && llmCfg.Endpoint != "" {
		summary, err := generateWithLLM(body, llmCfg)
		if err != nil {
			slog.Warn("tldr: LLM generation failed, falling back to extractive", "error", err)
		} else if summary != "" {
			return summary
		}
	}

	// Fallback: extractive summary
	return extractiveSummary(body)
}

func generateWithLLM(body string, cfg *LLMConfig) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Strip URLs and markdown links from the body so the LLM summarizes
	// content only, not sources/references.
	cleaned := body
	// Remove markdown links [text](url) → text
	for strings.Contains(cleaned, "](http") {
		start := strings.Index(cleaned, "](http")
		// Find the opening bracket
		openBracket := strings.LastIndex(cleaned[:start], "[")
		closeParen := strings.Index(cleaned[start:], ")")
		if openBracket >= 0 && closeParen >= 0 {
			linkText := cleaned[openBracket+1 : start]
			cleaned = cleaned[:openBracket] + linkText + cleaned[start+closeParen+1:]
		} else {
			break
		}
	}
	// Remove bare URLs
	for _, prefix := range []string{"https://", "http://"} {
		for {
			idx := strings.Index(cleaned, prefix)
			if idx < 0 {
				break
			}
			end := idx
			for end < len(cleaned) && cleaned[end] != ' ' && cleaned[end] != '\n' && cleaned[end] != ')' {
				end++
			}
			cleaned = cleaned[:idx] + cleaned[end:]
		}
	}

	truncated := cleaned
	if len(truncated) > 3000 {
		truncated = truncated[:3000] + "..."
	}

	prompt := `Summarize this post in your OWN words. Do NOT copy sentences from the post.

Format:
**[One original sentence capturing the main argument]**
- [First specific takeaway — paraphrased, not copied]
- [Second specific takeaway]
- [Third if needed]

STRICT RULES:
- Maximum 60 words total
- Rewrite in your own words — NEVER copy or closely paraphrase the original text
- No URLs, links, sources, images, or references
- No metadata labels like "Source:", "Key data points:", "Format:"
- Start directly with the bold sentence

Post:
` + truncated

	reqBody := map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": "You are a summarizer. Rewrite the key point in your own words. Never copy text from the input. Keep it under 60 words. Use bold for the main sentence and bullets for takeaways."},
			{"role": "user", "content": prompt},
		},
		"max_completion_tokens": 200,
		"temperature":           0.3,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%sopenai/deployments/%s/chat/completions?api-version=2024-02-15-preview",
		cfg.Endpoint, cfg.DeploymentName)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	summary := strings.TrimSpace(result.Choices[0].Message.Content)
	summary = strings.TrimPrefix(summary, "TL;DR:")
	summary = strings.TrimPrefix(summary, "TL;DR")
	summary = strings.TrimPrefix(summary, "**TL;DR:**")
	summary = strings.TrimPrefix(summary, "**TL;DR**")
	summary = strings.TrimSpace(summary)

	// Strip any images, links, and URLs from LLM output
	summary = stripOutputArtifacts(summary)

	// Reject if summary is too similar to the first paragraph (LLM just copied it)
	firstPara := strings.SplitN(truncated, "\n\n", 2)[0]
	firstPara = strings.TrimSpace(firstPara)
	if len(firstPara) > 50 && len(summary) > 50 {
		// Check if summary contains most of the first paragraph verbatim
		overlap := longestCommonSubstring(strings.ToLower(summary), strings.ToLower(firstPara))
		if overlap > len(firstPara)/2 {
			slog.Warn("tldr: LLM output too similar to first paragraph, rejecting")
			return "", fmt.Errorf("summary too similar to source text")
		}
	}

	return summary, nil
}

// stripOutputArtifacts removes images, links, URLs, and "Further reading"/"Source" lines
// from LLM-generated summaries that should be plain text.
func stripOutputArtifacts(s string) string {
	var cleaned []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		// Skip lines that are source/reading references
		if strings.HasPrefix(lower, "further reading") ||
			strings.HasPrefix(lower, "source:") ||
			strings.HasPrefix(lower, "sources:") ||
			strings.HasPrefix(lower, "read more") ||
			strings.HasPrefix(lower, "reference") {
			continue
		}
		// Remove markdown images ![alt](url)
		for strings.Contains(trimmed, "![") {
			start := strings.Index(trimmed, "![")
			end := strings.Index(trimmed[start:], ")")
			if end == -1 {
				break
			}
			trimmed = trimmed[:start] + trimmed[start+end+1:]
		}
		// Remove markdown links [text](url) → text
		for strings.Contains(trimmed, "](http") {
			linkStart := strings.Index(trimmed, "](http")
			openBracket := strings.LastIndex(trimmed[:linkStart], "[")
			closeParen := strings.Index(trimmed[linkStart:], ")")
			if openBracket >= 0 && closeParen >= 0 {
				linkText := trimmed[openBracket+1 : linkStart]
				trimmed = trimmed[:openBracket] + linkText + trimmed[linkStart+closeParen+1:]
			} else {
				break
			}
		}
		// Remove bare URLs
		for _, prefix := range []string{"https://", "http://"} {
			for {
				idx := strings.Index(trimmed, prefix)
				if idx < 0 {
					break
				}
				end := idx
				for end < len(trimmed) && trimmed[end] != ' ' && trimmed[end] != '\n' && trimmed[end] != ')' {
					end++
				}
				trimmed = strings.TrimSpace(trimmed[:idx] + trimmed[end:])
			}
		}
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// extractiveSummary picks the top 2 sentences as fallback.
func extractiveSummary(body string) string {
	sentences := splitSentences(body)
	if len(sentences) < 3 {
		return ""
	}

	type scored struct {
		text  string
		score int
		index int
	}

	var items []scored
	for i, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) < 30 {
			continue
		}
		items = append(items, scored{text: s, score: scoreSentence(s, i), index: i})
	}

	if len(items) < 2 {
		return ""
	}

	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].score > items[i].score {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	pick1, pick2 := items[0], items[1]
	if pick1.index > pick2.index {
		pick1, pick2 = pick2, pick1
	}
	return pick1.text + " " + pick2.text
}

func splitSentences(text string) []string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" || strings.HasPrefix(line, "---") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	joined := strings.Join(cleaned, " ")

	var sentences []string
	current := ""
	for i, c := range joined {
		current += string(c)
		if (c == '.' || c == '!' || c == '?') && i+1 < len(joined) && joined[i+1] == ' ' {
			if trimmed := strings.TrimSpace(current); len(trimmed) > 5 {
				sentences = append(sentences, trimmed)
			}
			current = ""
		}
	}
	if trimmed := strings.TrimSpace(current); len(trimmed) > 5 {
		sentences = append(sentences, trimmed)
	}
	return sentences
}

func scoreSentence(s string, position int) int {
	score := 0
	if position < 3 {
		score += 2
	}
	for _, w := range strings.Fields(s) {
		if len(w) > 1 && w[0] >= 'A' && w[0] <= 'Z' {
			score++
		}
		for _, c := range w {
			if c >= '0' && c <= '9' {
				score++
				break
			}
		}
	}
	if len(s) < 30 {
		score -= 2
	}
	return score
}

// longestCommonSubstring returns the length of the longest common substring between a and b.
func longestCommonSubstring(a, b string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	maxLen := 0
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
				if curr[j] > maxLen {
					maxLen = curr[j]
				}
			} else {
				curr[j] = 0
			}
		}
		prev, curr = curr, prev
		for k := range curr {
			curr[k] = 0
		}
	}
	return maxLen
}
