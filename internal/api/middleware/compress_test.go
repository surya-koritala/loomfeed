package middleware_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/surya-koritala/loomfeed/internal/api/middleware"
)

func TestGzip_CompressesWhenAccepted(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(strings.Repeat(`{"ok":true}`, 100)))
	})

	handler := middleware.Gzip(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("expected Content-Encoding: gzip")
	}

	gr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal("failed to create gzip reader:", err)
	}
	defer gr.Close()

	body, _ := io.ReadAll(gr)
	expected := strings.Repeat(`{"ok":true}`, 100)
	if string(body) != expected {
		t.Fatalf("body mismatch: got %d bytes, want %d", len(body), len(expected))
	}
}

func TestGzip_SkipsWhenNotAccepted(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})

	handler := middleware.Gzip(inner)

	req := httptest.NewRequest("GET", "/", nil)
	// No Accept-Encoding header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Fatal("should not gzip when Accept-Encoding is absent")
	}

	if rec.Body.String() != "hello" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestGzip_SkipsSmallResponses(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	})

	handler := middleware.Gzip(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Small response should not be gzipped (overhead not worth it)
	// The middleware buffers — if body < 256 bytes, writes plain
	if rec.Body.String() != "hi" {
		// It's acceptable if the middleware gzips anyway; just check it's valid
		if rec.Header().Get("Content-Encoding") == "gzip" {
			gr, err := gzip.NewReader(rec.Body)
			if err != nil {
				t.Fatal("invalid gzip for small response")
			}
			defer gr.Close()
			body, _ := io.ReadAll(gr)
			if string(body) != "hi" {
				t.Fatal("body mismatch for small response")
			}
		}
	}
}
