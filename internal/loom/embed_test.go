package loom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func stubAzureEmbed(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if got := r.Header.Get("api-key"); got != "test-key" {
			t.Errorf("api-key: want test-key, got %q", got)
		}
		if !strings.Contains(r.URL.Path, "/openai/deployments/test-deploy/embeddings") {
			t.Errorf("URL should target /openai/deployments/{deploy}/embeddings, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("api-version") == "" {
			t.Errorf("api-version query param required")
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func TestEmbedHappyPath(t *testing.T) {
	srv := stubAzureEmbed(t, 200, `{
		"data": [{"embedding": [0.1, -0.2, 0.3, 0.4]}],
		"usage": {"prompt_tokens": 5, "total_tokens": 5}
	}`)
	defer srv.Close()

	c := &AzureEmbedClient{
		Endpoint:   srv.URL + "/",
		APIKey:     "test-key",
		Deployment: "test-deploy",
		APIVersion: "2024-02-15-preview",
		HTTP:       srv.Client(),
	}
	vec, err := c.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 4 {
		t.Errorf("want 4 dims, got %d", len(vec))
	}
	if vec[0] != 0.1 || vec[1] != -0.2 {
		t.Errorf("unexpected vec values: %v", vec)
	}
}

func TestEmbedNon200IsError(t *testing.T) {
	srv := stubAzureEmbed(t, 429, `{"error":{"code":"RateLimit","message":"slow down"}}`)
	defer srv.Close()

	c := &AzureEmbedClient{
		Endpoint: srv.URL + "/", APIKey: "test-key",
		Deployment: "test-deploy", APIVersion: "2024-02-15-preview", HTTP: srv.Client(),
	}
	_, err := c.Embed(context.Background(), "hi")
	if err == nil {
		t.Fatal("429 should error")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestEmbedEmptyResponseIsError(t *testing.T) {
	srv := stubAzureEmbed(t, 200, `{"data":[],"usage":{"prompt_tokens":1,"total_tokens":1}}`)
	defer srv.Close()

	c := &AzureEmbedClient{
		Endpoint: srv.URL + "/", APIKey: "test-key",
		Deployment: "test-deploy", APIVersion: "2024-02-15-preview", HTTP: srv.Client(),
	}
	_, err := c.Embed(context.Background(), "hi")
	if err == nil {
		t.Fatal("empty data array should error")
	}
}

func TestEmbedMissingCredsErrors(t *testing.T) {
	c := &AzureEmbedClient{}
	_, err := c.Embed(context.Background(), "hi")
	if err == nil {
		t.Error("missing creds should error")
	}
}

func TestEmbedEmptyInputErrors(t *testing.T) {
	c := &AzureEmbedClient{
		Endpoint: "https://x/", APIKey: "k", Deployment: "d", APIVersion: "v",
		HTTP: http.DefaultClient,
	}
	_, err := c.Embed(context.Background(), "")
	if err == nil {
		t.Error("empty input should error before transport")
	}
}
