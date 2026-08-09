// Package metrics exposes Prometheus instrumentation primitives shared
// by the loomfeed services (cmd/api, cmd/gateway). It deliberately
// keeps the label set tight — only `method` and `status` for HTTP
// counters/histograms — to avoid cardinality explosion from path-level
// labels (post IDs, agent IDs, etc.). Per-route breakdowns can be added
// later via a dedicated middleware that maps request to a small set of
// canonical route patterns.
package metrics

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is the package-private registry. Each cmd/ binary should
// expose it via `Handler()` mounted at `/metrics`.
var registry = prometheus.NewRegistry()

var (
	// HTTPRequestsTotal counts every HTTP request the service handled,
	// labeled by method and status code class. We label with the
	// numeric status (200, 404, 500) rather than a class so error
	// rates can be computed precisely.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests processed, partitioned by method and status code.",
		},
		[]string{"method", "status"},
	)

	// HTTPRequestDuration tracks request latency. Buckets are tuned for
	// a typical web API (sub-millisecond → multi-second) without going
	// out to several seconds, since anything slower than ~5s is almost
	// certainly a timeout we want to alert on separately.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration distribution, partitioned by method and status code.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "status"},
	)

	// LoomSummonsTotal counts every Loom summon dispatched. Labelled
	// by intent (summarize, fact_check, ...), model (haiku/sonnet),
	// terminal state (done/error), and cached (true when served from
	// the Redis layer). Cardinality is bounded by the dispatch table
	// + a 2-state cached label, so this is cheap.
	LoomSummonsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "loom_summons_total",
			Help: "Total Loom summons, by intent, model, terminal state, and cache outcome.",
		},
		[]string{"intent", "model", "state", "cached"},
	)

	// LoomInferenceCostUSD totals the dollar cost charged for Loom
	// inference per model. Cache hits contribute 0. This is the metric
	// that gets piped into the Grafana cost dashboard and is the
	// load-bearing signal for "free-for-long" affordability.
	LoomInferenceCostUSD = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "loom_inference_cost_usd_total",
			Help: "Cumulative USD spent on Loom inference, partitioned by model.",
		},
		[]string{"model"},
	)

	// LoomSummonLatency captures end-to-end latency from summon insert
	// to either reply-posted or marked-errored. Bucketing favors the
	// 0.1s–10s band where LLM calls land; finer resolution below 0.1s
	// would mostly capture cache hits (already labelled).
	LoomSummonLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "loom_summon_latency_seconds",
			Help:    "Loom summon end-to-end latency, by intent and cache outcome.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"intent", "cached"},
	)

	// LoomCacheHits / LoomCacheMisses drive the hit-rate panel. A low
	// hit rate on summarize is a signal that posts churn faster than
	// the TTL (or that the cache key is too narrow). Tracked
	// separately rather than as a labelled counter so the rate ratio
	// is a one-liner in PromQL.
	LoomCacheHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "loom_cache_hits_total",
			Help: "Total Loom cache hits.",
		},
	)
	LoomCacheMisses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "loom_cache_misses_total",
			Help: "Total Loom cache misses (forced a fresh LLM call).",
		},
	)
)

func init() {
	registry.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		LoomSummonsTotal,
		LoomInferenceCostUSD,
		LoomSummonLatency,
		LoomCacheHits,
		LoomCacheMisses,
		// Standard process + Go runtime collectors — free metrics for
		// goroutine count, GC pauses, RSS, fd count, etc.
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
	)
}

// Handler returns the http.Handler that serves /metrics in Prometheus
// exposition format. Mount it explicitly in each service's main.go.
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		Registry:          registry,
		EnableOpenMetrics: true,
	})
}

// GuardedHandler wraps Handler with a bearer-token check. The /metrics
// endpoint is otherwise publicly reachable (the services sit behind a public
// ingress) and leaks per-route latencies, error rates, and runtime internals
// useful for timing/capacity probing. When token is non-empty, callers must
// send `Authorization: Bearer <token>`; when empty, the handler is open and a
// warning is logged at startup so the exposure is deliberate, not accidental.
func GuardedHandler(token string) http.Handler {
	h := Handler()
	if token == "" {
		slog.Warn("metrics endpoint is unauthenticated; set METRICS_TOKEN to require a bearer token")
		return h
	}
	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// excludedPaths are not instrumented. /metrics would otherwise count
// the Prometheus scraper itself, inflating request_total and skewing
// duration percentiles. /healthz and /readyz are high-frequency probes
// from container orchestration; we don't want their volume drowning
// out actual user traffic in the request counter.
var excludedPaths = map[string]struct{}{
	"/metrics": {},
	"/healthz": {},
	"/readyz":  {},
}

// Middleware wraps next to record per-request counter and histogram
// observations. Place it innermost (closest to the mux) so the status
// code captured reflects what the handler wrote rather than anything
// upstream middleware mutated.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, skip := excludedPaths[r.URL.Path]; skip {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		status := strconv.Itoa(sw.status)
		HTTPRequestsTotal.WithLabelValues(r.Method, status).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, status).Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

// Flush + Unwrap mirror the existing middleware/statusWriter so
// upstream middlewares (Gzip, MCP SSE) keep working when this wrapper
// is in the chain.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
