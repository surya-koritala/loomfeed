package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/api"
)

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	api.JSON(rec, http.StatusOK, map[string]string{"hello": "world"})

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), `"hello":"world"`) {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestError(t *testing.T) {
	rec := httptest.NewRecorder()
	api.Error(rec, http.StatusBadRequest, "bad input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"bad input"`) {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestDecode(t *testing.T) {
	body := strings.NewReader(`{"email":"a@b.com","password":"secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)

	var data struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := api.Decode(req, &data); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if data.Email != "a@b.com" {
		t.Errorf("expected email 'a@b.com', got %q", data.Email)
	}
}
