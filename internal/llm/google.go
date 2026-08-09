package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Google struct {
	APIKey string
	Model  string
}

func (g *Google) Name() string { return "google" }

type googlePart struct {
	Text string `json:"text"`
}
type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}
type googleReq struct {
	SystemInstruction *googleContent  `json:"systemInstruction,omitempty"`
	Contents          []googleContent `json:"contents"`
	GenerationConfig  struct {
		Temperature     float64 `json:"temperature"`
		MaxOutputTokens int     `json:"maxOutputTokens"`
	} `json:"generationConfig"`
}
type googleResp struct {
	Candidates []struct {
		Content googleContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (g *Google) Generate(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	body := googleReq{
		Contents: []googleContent{
			{Role: "user", Parts: []googlePart{{Text: userMessage}}},
		},
	}
	if systemPrompt != "" {
		body.SystemInstruction = &googleContent{Parts: []googlePart{{Text: systemPrompt}}}
	}
	body.GenerationConfig.Temperature = 0.7
	body.GenerationConfig.MaxOutputTokens = 1024

	raw, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		url.PathEscape(g.Model), url.QueryEscape(g.APIKey),
	)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("google request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("google %d: %s", resp.StatusCode, string(respBytes))
	}
	var parsed googleResp
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("google parse: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("google error: %s", parsed.Error.Message)
	}
	for _, c := range parsed.Candidates {
		for _, p := range c.Content.Parts {
			if p.Text != "" {
				return p.Text, nil
			}
		}
	}
	return "", fmt.Errorf("google: no text part in response")
}
