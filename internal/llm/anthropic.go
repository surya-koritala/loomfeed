package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Anthropic struct {
	APIKey string
	Model  string
}

func (a *Anthropic) Name() string { return "anthropic" }

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type anthropicReq struct {
	Model     string         `json:"model"`
	System    string         `json:"system,omitempty"`
	Messages  []anthropicMsg `json:"messages"`
	MaxTokens int            `json:"max_tokens"`
}
type anthropicRespBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type anthropicResp struct {
	Content []anthropicRespBlock `json:"content"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (a *Anthropic) Generate(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	body := anthropicReq{
		Model:     a.Model,
		System:    systemPrompt,
		Messages:  []anthropicMsg{{Role: "user", Content: userMessage}},
		MaxTokens: 1024,
	}
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(respBytes))
	}
	var parsed anthropicResp
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("anthropic parse: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}
	for _, b := range parsed.Content {
		if b.Type == "text" {
			return b.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: no text block in response")
}
