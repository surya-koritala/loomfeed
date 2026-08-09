package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/surya-koritala/loomfeed/internal/config"
	mcpgateway "github.com/surya-koritala/loomfeed/internal/gateway/mcp"
	"github.com/surya-koritala/loomfeed/internal/metrics"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"ok"}`)
	})

	// Readiness probe — same shape as cmd/api so Container Apps can
	// distinguish "process started" from "ready to serve". The gateway
	// has no DB connection of its own (it forwards to the API), so
	// readiness is functionally identical to liveness today; the
	// separate endpoint is here so deploy infra has a stable URL when
	// readiness graduates to a real check (e.g. ping the upstream API).
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"status":"ready"}`)
	})

	// Prometheus exposition. Mounted on the mux directly; the metrics
	// middleware below skips /metrics so the scraper does not appear
	// in the counters it reads.
	mux.Handle("GET /metrics", metrics.GuardedHandler(os.Getenv("METRICS_TOKEN")))

	// MCP protocol gateway
	coreAPIURL := os.Getenv("CORE_API_URL")
	if coreAPIURL == "" {
		coreAPIURL = fmt.Sprintf("http://localhost:%s", cfg.API.Port)
	}
	mcpSrv := mcpgateway.NewServer(coreAPIURL)

	// MCP server using Streamable HTTP transport (stateless mode).
	// Same migration as the API server's /mcp endpoint — see
	// internal/api/routes/routes.go for rationale.
	mcpSrvInstance := mcpserver.NewMCPServer("loomfeed", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)
	mcpSrv.RegisterAllTools(mcpSrvInstance)

	streamableServer := mcpserver.NewStreamableHTTPServer(mcpSrvInstance,
		mcpserver.WithStateLess(true),
		mcpserver.WithEndpointPath("/mcp"),
		mcpserver.WithHTTPContextFunc(mcpgateway.APIKeyContextFunc),
	)
	mux.Handle("/mcp", streamableServer)

	// REST wrapper endpoints (backward-compatible)
	mux.HandleFunc("POST /mcp/tools/call", mcpSrv.HandleToolCall)
	mux.HandleFunc("GET /mcp/tools/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mcpgateway.AvailableTools()); err != nil {
			slog.Error("failed to encode tool list", "error", err)
		}
	})

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.Gateway.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      metrics.Middleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("gateway server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
