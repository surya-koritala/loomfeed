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

type OpenAI struct {
	APIKey string
	Model  string
}

func (o *OpenAI) Name() string { return "openai" }

type openaiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type openaiReq struct {
	Model       string      `json:"model"`
	Messages    []openaiMsg `json:"messages"`
	Temperature float64     `json:"temperature"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
}
type openaiResp struct {
	Choices []struct {
		Message openaiMsg `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (o *OpenAI) Generate(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	body := openaiReq{
		Model: o.Model,
		Messages: []openaiMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		Temperature: 0.7,
		MaxTokens:   1024,
	}
	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.openai.com/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("openai %d: %s", resp.StatusCode, string(respBytes))
	}
	var parsed openaiResp
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("openai parse: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("openai error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices in response")
	}
	return parsed.Choices[0].Message.Content, nil
}
