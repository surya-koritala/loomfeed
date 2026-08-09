package loom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AzureOpenAIClient is the v1 LLM client. Speaks to Azure OpenAI's
// Chat Completions API (the existing platform LLM, also used by
// internal/quality's followups + summary). Distinct from
// internal/llm.OpenAI because the loom path needs token-usage fields
// (for cost telemetry) that the BYOK Provider interface doesn't
// expose. Same URL shape and headers as quality/followups so an
// operator who has cfg.LLM working already has Loom working.
//
// The deployment name is the "model" Loom records — that's the
// operator-controlled identifier the cost table keys on.
type AzureOpenAIClient struct {
	Endpoint       string // e.g. "https://my-resource.openai.azure.com/"
	APIKey         string
	APIVersion     string // optional; defaults to a stable preview version
	HTTP           *http.Client
}

// NewAzureOpenAIClient wires the v1 LLM client.
func NewAzureOpenAIClient(endpoint, apiKey string) *AzureOpenAIClient {
	return &AzureOpenAIClient{
		Endpoint:   endpoint,
		APIKey:     apiKey,
		APIVersion: "2024-02-15-preview",
		HTTP:       &http.Client{Timeout: 60 * time.Second},
	}
}

type aoMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aoRequest struct {
	Messages            []aoMsg `json:"messages"`
	MaxCompletionTokens int     `json:"max_completion_tokens"`
	Temperature         float64 `json:"temperature"`
}

type aoUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type aoResponse struct {
	Choices []struct {
		Message aoMsg `json:"message"`
	} `json:"choices"`
	Usage aoUsage `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Complete sends one synchronous chat-completion call and returns the
// text + token counts. The model is the Azure deployment name — Azure
// routes by deployment, not raw model id.
//
// Error policy: any non-200, JSON error envelope, or transport
// failure returns a *non-nil* error; the worker maps it to
// MarkErrored. No retries — one summon = one attempt. Clients retry
// by issuing a new summon.
func (c *AzureOpenAIClient) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if c.APIKey == "" || c.Endpoint == "" {
		return nil, fmt.Errorf("loom: Azure OpenAI endpoint or API key not configured")
	}

	body := aoRequest{
		Messages: []aoMsg{
			{Role: "system", Content: req.SystemPrompt},
			{Role: "user", Content: req.UserPrompt},
		},
		MaxCompletionTokens: req.MaxOutputToks,
		Temperature:         0.3, // low temperature for summaries; deterministic enough to cache cleanly
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal azure-openai request: %w", err)
	}

	// req.Model is the deployment name. Azure routes the request based
	// on the path segment, not a "model" body field.
	url := fmt.Sprintf("%sopenai/deployments/%s/chat/completions?api-version=%s",
		c.Endpoint, req.Model, c.APIVersion)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build azure-openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("azure-openai transport: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure-openai %d: %s", resp.StatusCode, string(respBytes))
	}

	var parsed aoResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("parse azure-openai response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("azure-openai %s: %s", parsed.Error.Code, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("azure-openai: no choices in response")
	}

	return &CompletionResponse{
		Text:         parsed.Choices[0].Message.Content,
		InputTokens:  parsed.Usage.PromptTokens,
		OutputTokens: parsed.Usage.CompletionTokens,
	}, nil
}
