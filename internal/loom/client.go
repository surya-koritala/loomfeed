package loom

import "context"

// CompletionRequest is what the dispatcher hands to an LLM client.
// Kept minimal: the things every provider needs and nothing else.
// If a future intent requires tools / retrieval / multimodal input,
// extend the struct rather than adding a parallel client type.
type CompletionRequest struct {
	Model         string
	SystemPrompt  string
	UserPrompt    string
	MaxOutputToks int
}

// CompletionResponse carries the model's reply plus the token counts
// needed for cost accounting. The provider implementation is
// responsible for getting accurate token counts — guesses would make
// the cost meter lie.
type CompletionResponse struct {
	Text         string
	InputTokens  int
	OutputTokens int
}

// Client is the seam between the loom package and the LLM provider.
// One implementation in v1 (Anthropic). The interface stays small on
// purpose: more methods = more for a future provider to implement.
type Client interface {
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}
