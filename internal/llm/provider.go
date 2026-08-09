// Package llm is a thin abstraction over hosted LLM APIs (OpenAI, Anthropic,
// Google) so the rest of the codebase can call a uniform Generate(ctx, ...)
// regardless of which provider a BYOK agent is configured with.
//
// Each provider has its own quirks (JSON shapes, auth headers, "system"
// handling) that are hidden behind the Provider interface. Adding a new
// provider = implementing one method + registering in New().
package llm

import (
	"context"
	"fmt"
	"strings"
)

// Provider speaks to one hosted LLM API.
type Provider interface {
	// Generate returns the model's text response. systemPrompt sets persona
	// / guardrails; userMessage is what the caller wants answered.
	Generate(ctx context.Context, systemPrompt, userMessage string) (string, error)

	// Name returns the canonical lowercase provider slug ("openai", etc.)
	// used in the byok_agents.provider column.
	Name() string
}

// Config identifies which provider to construct and with what credentials.
type Config struct {
	Provider string // "openai" | "anthropic" | "google"
	Model    string
	APIKey   string
}

// New builds a Provider for the given config. Unknown providers return an
// error so callers fail loudly instead of silently no-oping.
func New(cfg Config) (Provider, error) {
	switch strings.ToLower(cfg.Provider) {
	case "openai":
		return &OpenAI{APIKey: cfg.APIKey, Model: defaultIfEmpty(cfg.Model, "gpt-4o-mini")}, nil
	case "anthropic":
		return &Anthropic{APIKey: cfg.APIKey, Model: defaultIfEmpty(cfg.Model, "claude-haiku-4-5-20251001")}, nil
	case "google":
		return &Google{APIKey: cfg.APIKey, Model: defaultIfEmpty(cfg.Model, "gemini-2.0-flash")}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}

// SupportedProviders returns the canonical slugs the UI should offer in the
// dropdown. Kept in a single place so the API handler can validate against it.
func SupportedProviders() []string {
	return []string{"openai", "anthropic", "google"}
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
