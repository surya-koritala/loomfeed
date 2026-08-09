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

// Embedder produces an embedding vector for one or more chunks of
// text. EmbedBatch is the throughput-oriented path (Azure's endpoint
// natively accepts an input array; latency is dominated by transport
// so batching by 16-32 inputs gives near-linear speedup). Embed is a
// convenience for single-call sites like the post-create hook.
//
// Callers should not assume any particular dimensionality; the table
// column shape is the contract.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error)
}

// AzureEmbedClient calls the Azure OpenAI embeddings endpoint. Same
// URL shape and auth as AzureOpenAIClient — operators with one set
// of credentials have both clients working out of the box.
//
// The Deployment field is the Azure deployment name for an
// embeddings model (e.g. text-embedding-3-large), distinct from the
// chat-completions deployment used by the summon path.
type AzureEmbedClient struct {
	Endpoint   string
	APIKey     string
	Deployment string
	APIVersion string
	HTTP       *http.Client
}

// NewAzureEmbedClient wires the embed client with a 60s default
// timeout (embeddings are typically <500ms; the long timeout is for
// occasional Azure rate-limit waits during backfill).
func NewAzureEmbedClient(endpoint, apiKey, deployment string) *AzureEmbedClient {
	return &AzureEmbedClient{
		Endpoint:   endpoint,
		APIKey:     apiKey,
		Deployment: deployment,
		APIVersion: "2024-02-15-preview",
		HTTP:       &http.Client{Timeout: 60 * time.Second},
	}
}

// embedRequest accepts either a single string OR a string slice for
// the Input field. Azure's embedding endpoint handles both shapes
// natively. We use Input{} (interface{}) so the same struct serves
// both the single-Embed and batched-EmbedBatch paths.
type embedRequest struct {
	Input any `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Embed returns the vector for one input. Convenience wrapper around
// EmbedBatch. Non-200 / parse / missing-embedding paths all surface
// as errors so the caller can retry or skip. No internal retry —
// callers own retry policy.
func (c *AzureEmbedClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("loom embed: input text is empty")
	}
	out, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

// EmbedBatch sends multiple inputs in a single HTTP roundtrip and
// returns their vectors in input order. This is the throughput
// lever for the backfill — Azure's embedding endpoint accepts an
// input array natively, and per-call latency is dominated by
// transport + cold-call overhead rather than per-input compute, so
// batching by 16-32 gets you near-linear throughput speedup.
//
// Length-equality contract: len(result) == len(inputs). The endpoint
// preserves input order via the `index` field on each data entry; we
// reassemble defensively in case Azure ever returns them out of
// order.
func (c *AzureEmbedClient) EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	if c.APIKey == "" || c.Endpoint == "" || c.Deployment == "" {
		return nil, fmt.Errorf("loom embed: endpoint, api key, or deployment not configured")
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("loom embed: empty input list")
	}
	for i, in := range inputs {
		if in == "" {
			return nil, fmt.Errorf("loom embed: input %d is empty", i)
		}
	}

	body := embedRequest{Input: inputs}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	url := fmt.Sprintf("%sopenai/deployments/%s/embeddings?api-version=%s",
		c.Endpoint, c.Deployment, c.APIVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed transport: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure embed %d: %s", resp.StatusCode, string(respBytes))
	}

	var parsed embedResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("parse embed response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("azure embed %s: %s", parsed.Error.Code, parsed.Error.Message)
	}
	if len(parsed.Data) != len(inputs) {
		return nil, fmt.Errorf("azure embed: returned %d vectors for %d inputs",
			len(parsed.Data), len(inputs))
	}

	// Reassemble by index. Azure has historically returned data in
	// input order, but the `index` field is part of the contract and
	// the cost of trusting it is one slice walk.
	out := make([][]float32, len(inputs))
	for i := range parsed.Data {
		idx := parsed.Data[i].Index
		if idx < 0 || idx >= len(inputs) {
			return nil, fmt.Errorf("azure embed: out-of-range index %d", idx)
		}
		if len(parsed.Data[i].Embedding) == 0 {
			return nil, fmt.Errorf("azure embed: empty embedding at index %d", idx)
		}
		out[idx] = parsed.Data[i].Embedding
	}
	for i, v := range out {
		if v == nil {
			return nil, fmt.Errorf("azure embed: missing embedding at index %d", i)
		}
	}
	return out, nil
}
