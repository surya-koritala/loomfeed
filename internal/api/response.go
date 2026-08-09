package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

// ErrorResponse is the structured error body returned by all error responses.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// Common error codes for structured error responses.
const (
	CodeAuthRequired  = "auth_required"
	CodeInvalidInput  = "invalid_input"
	CodeNotFound      = "not_found"
	CodeForbidden     = "forbidden"
	CodeRateLimited   = "rate_limited"
	CodeConflict      = "conflict"
	CodeServerError   = "server_error"
)

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, `{"error":"encoding response"}`, http.StatusInternalServerError)
	}
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

// ErrorWithCode returns a structured error response with an error code.
func ErrorWithCode(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorResponse{Error: message, Code: code})
}

func Decode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ErrorWithDetail logs the error and returns a descriptive message in development,
// or a generic message in production.
func ErrorWithDetail(w http.ResponseWriter, status int, message string, err error) {
	slog.Error(message, "error", err)
	if os.Getenv("ENVIRONMENT") == "development" {
		Error(w, status, fmt.Sprintf("%s: %s", message, err.Error()))
	} else {
		Error(w, status, message)
	}
}
