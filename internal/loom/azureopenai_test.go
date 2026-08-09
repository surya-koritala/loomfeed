package loom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubAzureOpenAI stands in for the Azure OpenAI Chat Completions API.
// Asserts URL shape, request headers, and returns a canned response.
func stubAzureOpenAI(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if got := r.Header.Get("api-key"); got != "test-key" {
			t.Errorf("api-key: want test-key, got %q", got)
		}
		if !strings.Contains(r.URL.Path, "/openai/deployments/") {
			t.Errorf("path should contain /openai/deployments/, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("api-version") == "" {
			t.Errorf("api-version query param required")
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func TestAzureOpenAIHappyPath(t *testing.T) {
	srv := stubAzureOpenAI(t, 200, `{
		"choices": [{"message": {"role": "assistant", "content": "Here is the summary. Loom can make mistakes — verify before relying."}}],
		"usage": {"prompt_tokens": 123, "completion_tokens": 45, "total_tokens": 168}
	}`)
	defer srv.Close()

	c := &AzureOpenAIClient{
		Endpoint:   srv.URL + "/", // trailing slash matches Azure resource URLs
		APIKey:     "test-key",
		APIVersion: "2024-02-15-preview",
		HTTP:       srv.Client(),
	}
	resp, err := c.Complete(context.Background(), CompletionRequest{
		Model: "test-deploy", SystemPrompt: "sys", UserPrompt: "hi", MaxOutputToks: 100,
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if !strings.HasPrefix(resp.Text, "Here is the summary") {
		t.Errorf("text not parsed: %q", resp.Text)
	}
	if resp.InputTokens != 123 || resp.OutputTokens != 45 {
		t.Errorf("usage parsed wrong: in=%d out=%d", resp.InputTokens, resp.OutputTokens)
	}
}

func TestAzureOpenAINon200IsError(t *testing.T) {
	srv := stubAzureOpenAI(t, 429, `{"error": {"code": "RateLimit", "message": "too fast"}}`)
	defer srv.Close()

	c := &AzureOpenAIClient{
		Endpoint:   srv.URL + "/",
		APIKey:     "test-key",
		APIVersion: "2024-02-15-preview",
		HTTP:       srv.Client(),
	}
	_, err := c.Complete(context.Background(), CompletionRequest{
		Model: "test-deploy", SystemPrompt: "sys", UserPrompt: "hi", MaxOutputToks: 100,
	})
	if err == nil {
		t.Fatal("Complete on 429 should error, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestAzureOpenAINoChoicesIsError(t *testing.T) {
	srv := stubAzureOpenAI(t, 200, `{"choices": [], "usage": {"prompt_tokens": 1, "completion_tokens": 0}}`)
	defer srv.Close()

	c := &AzureOpenAIClient{
		Endpoint:   srv.URL + "/",
		APIKey:     "test-key",
		APIVersion: "2024-02-15-preview",
		HTTP:       srv.Client(),
	}
	_, err := c.Complete(context.Background(), CompletionRequest{
		Model: "test-deploy", SystemPrompt: "sys", UserPrompt: "hi", MaxOutputToks: 100,
	})
	if err == nil {
		t.Fatal("empty choices should error, got nil")
	}
}

func TestAzureOpenAIMissingCredsErrors(t *testing.T) {
	c := &AzureOpenAIClient{Endpoint: "", APIKey: ""}
	_, err := c.Complete(context.Background(), CompletionRequest{Model: "x", UserPrompt: "y"})
	if err == nil {
		t.Error("missing creds should error, got nil")
	}
}
