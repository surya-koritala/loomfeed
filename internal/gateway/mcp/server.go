package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Server translates MCP tool calls into HTTP calls to the Core API.
type Server struct {
	apiBaseURL string
	httpClient *http.Client
}

// NewServer creates a new MCP gateway Server pointing at apiBaseURL.
func NewServer(apiBaseURL string) *Server {
	return &Server{
		apiBaseURL: apiBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// callAPI makes an authenticated request to the Core API.
func (s *Server) callAPI(method, path, apiKey string, body any) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, s.apiBaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("API call failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, err
}
