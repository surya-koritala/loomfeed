// Package moderation wraps external content-safety providers so the
// rest of the app can call Check() without knowing the vendor.
//
// Currently supports Azure AI Content Safety (image analysis). Returns
// a ModerationDecision rather than raw severity scores so callers don't
// have to understand the 0/2/4/6 Azure scale.
package moderation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Decision is the result of a moderation check.
type Decision struct {
	// Allowed is true if the image passed all safety checks.
	Allowed bool
	// Category is the highest-severity category that caused a block,
	// empty if Allowed=true. One of: Sexual, Violence, Hate, SelfHarm.
	Category string
	// Severity is Azure's 0/2/4/6 scale for the blocking category.
	Severity int
}

// ImageModerator checks uploaded image bytes for unsafe content.
type ImageModerator interface {
	Check(ctx context.Context, imageBytes []byte) (Decision, error)
}

// AzureContentSafety calls Azure AI Content Safety's image:analyze
// endpoint and blocks on any category at severity >= 4 (medium/high).
// SelfHarm is not checked on uploads — it's a text-content concern
// and produces many false positives on medical/educational images.
type AzureContentSafety struct {
	endpoint string
	key      string
	client   *http.Client
}

func NewAzureContentSafety(endpoint, key string) *AzureContentSafety {
	return &AzureContentSafety{
		endpoint: endpoint,
		key:      key,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

type csRequest struct {
	Image csImage `json:"image"`
}

type csImage struct {
	Content string `json:"content"`
}

type csResponse struct {
	CategoriesAnalysis []csCategory `json:"categoriesAnalysis"`
}

type csCategory struct {
	Category string `json:"category"`
	Severity int    `json:"severity"`
}

// blockThresholds is the per-category severity cutoff. Azure returns
// 0 (safe), 2 (low), 4 (medium), or 6 (high).
var blockThresholds = map[string]int{
	"Sexual":   4,
	"Violence": 4,
	"Hate":     4,
}

// Check uploads the image bytes to Content Safety and returns a Decision.
// On any network or API failure, returns an error — callers should treat
// that as a hard block ("fail closed"), not a pass.
func (a *AzureContentSafety) Check(ctx context.Context, imageBytes []byte) (Decision, error) {
	reqBody, err := json.Marshal(csRequest{
		Image: csImage{Content: base64.StdEncoding.EncodeToString(imageBytes)},
	})
	if err != nil {
		return Decision{}, fmt.Errorf("marshal request: %w", err)
	}

	url := a.endpoint + "/contentsafety/image:analyze?api-version=2024-09-01"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return Decision{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Ocp-Apim-Subscription-Key", a.key)

	resp, err := a.client.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("content safety request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Decision{}, fmt.Errorf("content safety returned %d: %s", resp.StatusCode, string(body))
	}

	var out csResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Decision{}, fmt.Errorf("decode response: %w", err)
	}

	for _, cat := range out.CategoriesAnalysis {
		threshold, ok := blockThresholds[cat.Category]
		if !ok {
			continue
		}
		if cat.Severity >= threshold {
			return Decision{
				Allowed:  false,
				Category: cat.Category,
				Severity: cat.Severity,
			}, nil
		}
	}
	return Decision{Allowed: true}, nil
}
