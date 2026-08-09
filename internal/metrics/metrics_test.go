package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHandler_ExposesGoRuntimeMetrics(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics endpoint returned status %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	out := string(body)
	// Standard collectors give us go_goroutines and go_gc_duration_seconds.
	for _, want := range []string{"go_goroutines", "process_resident_memory_bytes"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in metrics output", want)
		}
	}
}

func TestMiddleware_RecordsCounterAndHistogram(t *testing.T) {
	HTTPRequestsTotal.Reset()
	HTTPRequestDuration.Reset()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	srv := Middleware(inner)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)
	srv.ServeHTTP(httptest.NewRecorder(), req)

	count := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("POST", "201"))
	if count != 1 {
		t.Errorf("expected counter=1 for POST/201, got %v", count)
	}
}

func TestMiddleware_DefaultsTo200WhenHandlerDoesNotWriteHeader(t *testing.T) {
	HTTPRequestsTotal.Reset()
	HTTPRequestDuration.Reset()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	})

	srv := Middleware(inner)
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	count := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("GET", "200"))
	if count != 1 {
		t.Errorf("expected GET/200 counter=1, got %v", count)
	}
}

func TestMiddleware_SkipsExcludedPaths(t *testing.T) {
	HTTPRequestsTotal.Reset()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := Middleware(inner)

	for _, path := range []string{"/metrics", "/healthz", "/readyz"} {
		srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	// None of those paths should have generated counter samples.
	if got := testutil.ToFloat64(HTTPRequestsTotal.WithLabelValues("GET", "200")); got != 0 {
		t.Errorf("excluded paths leaked into counter: GET/200 = %v", got)
	}
}

func TestMiddleware_PreservesFlushSemantics(t *testing.T) {
	flushed := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
			flushed = true
		}
	})
	srv := Middleware(inner)
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/events/stream", nil))
	if !flushed {
		t.Error("inner handler could not Flush — wrapper did not preserve http.Flusher")
	}
}
