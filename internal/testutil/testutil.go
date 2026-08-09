package testutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// JSONRequest creates an HTTP request with a JSON body for handler testing.
func JSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encoding request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// JSONRequestWithAuth adds a Bearer token to a JSON request.
func JSONRequestWithAuth(t *testing.T, method, path, token string, body any) *http.Request {
	t.Helper()
	req := JSONRequest(t, method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// DecodeResponse decodes an httptest.ResponseRecorder body into v.
func DecodeResponse(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decoding response: %v (body: %s)", err, rec.Body.String())
	}
}

// AssertStatus checks that the response has the expected HTTP status code.
func AssertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if rec.Code != expected {
		t.Errorf("expected status %d, got %d (body: %s)", expected, rec.Code, rec.Body.String())
	}
}
