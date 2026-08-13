package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/api/routes"
	"github.com/surya-koritala/loomfeed/internal/cache"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/digest"
	"github.com/surya-koritala/loomfeed/internal/email"
	"github.com/surya-koritala/loomfeed/internal/events"
	"github.com/surya-koritala/loomfeed/internal/jobs"
	"github.com/surya-koritala/loomfeed/internal/loom"
	"github.com/surya-koritala/loomfeed/internal/metrics"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/sports"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "error", err)
		os.Exit(1)
	}

	pool, err := database.ConnectWithRLS(ctx, cfg.DB.URL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Initialize Redis client (used for rate limiting and caching)
	var redisClient *redis.Client
	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		slog.Warn("redis not available, rate limiting and caching disabled", "error", err)
	} else {
		opt.DialTimeout = 2 * time.Second
		opt.ReadTimeout = 1 * time.Second
		opt.WriteTimeout = 1 * time.Second
		opt.PoolTimeout = 2 * time.Second
		opt.MaxRetries = 1
		// Keep ~4 idle connections per replica warm so requests don't
		// pay TLS handshake to Azure Cache for Redis on every cache
		// hit after quiet periods. Without this, observed cache HITs
		// ranged 120ms (warm conn) → 835ms (cold conn) because the
		// rediss:// handshake adds ~300-500ms. PoolSize caps total
		// per-replica connections — well under the C0 SKU's 1000-conn
		// limit even at maxReplicas=10.
		opt.MinIdleConns = 4
		opt.PoolSize = 20
		redisClient = redis.NewClient(opt)
		// Test connection
		if err := redisClient.Ping(ctx).Err(); err != nil {
			slog.Warn("redis ping failed, disabling", "error", err)
			redisClient = nil
		} else {
			slog.Info("redis connected")
		}
	}

	// Create Redis cache (nil-safe — handlers skip caching if nil)
	redisCache := cache.NewRedisCache(redisClient)

	mux := http.NewServeMux()

	// Health check — verifies DB connectivity
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, `{"status":"unhealthy","db":"down"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// Readiness check
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Prometheus exposition. Mounted directly on the mux (not behind
	// any middleware) so a Prometheus scrape is not counted as a
	// request in the metrics it is reading — the scraper would
	// otherwise inflate request_total and skew duration percentiles.
	mux.Handle("GET /metrics", metrics.GuardedHandler(os.Getenv("METRICS_TOKEN")))

	hub := events.NewHub()
	routes.Register(mux, pool, cfg, redisCache, hub)

	// metrics.Middleware is innermost (closest to the mux) so the
	// captured status code is what the handler wrote, not anything
	// mutated upstream by Gzip. The middleware excludes /metrics
	// (so scrape requests don't inflate the counters they read)
	// and /healthz + /readyz (so probe volume doesn't drown out
	// actual traffic in request_total).
	//
	// CSRF sits between CORS and metrics: blocked requests are still
	// counted in request_total (we want to alert on CSRF-rejected
	// volume), but their handlers never run. The middleware is a
	// no-op for safe methods and Bearer/API-key auth, so SDKs see
	// zero change in behavior.
	handler := http.Handler(
		middleware.Gzip(
			middleware.SecurityHeaders(
				middleware.Logger(
					middleware.CORS(cfg.API.AllowedOrigins)(
						middleware.CSRF(cfg.API.AllowedOrigins)(
							metrics.Middleware(mux),
						),
					),
				),
			),
		),
	)

	// Limit request body to 10MB
	handler = http.MaxBytesHandler(handler, 10<<20)

	if redisClient != nil {
		rl := middleware.NewRateLimiter(redisClient, 300, time.Minute)
		handler = rl.Middleware(handler)
	}

	addr := fmt.Sprintf("%s:%s", cfg.API.Host, cfg.API.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("api server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Background goroutine: mark agents offline if no heartbeat in last 15 minutes.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		hbRepo := repository.NewHeartbeatRepo(pool)
		for range ticker.C {
			count, err := hbRepo.MarkOffline(context.Background(), 15*time.Minute)
			if err != nil {
				slog.Error("heartbeat offline sweep failed", "error", err)
			} else if count > 0 {
				slog.Info("marked agents offline", "count", count)
			}
		}
	}()

	// Background goroutine: fetch external trending topics every hour.
	// First fetch happens 20s after startup so the listing has data on
	// fresh deploys without blocking the API server's readiness.
	go func() {
		fetcher := jobs.NewTrendingFetcher(repository.NewTrendingRepo(pool))
		fetcher.Run(ctx, time.Hour)
	}()

	// Background goroutine: keep posts.ranked_score current. The feed
	// handler reads this column instead of computing the multi-table
	// score expression per request — the difference between sub-ms
	// index-only top-K and a 3.5s sort over 46k rows.
	go func() {
		w := jobs.NewRankedScoreWorker(pool)
		w.Run(ctx, time.Minute)
	}()

	// Background goroutine: refresh the platform_stats snapshot every
	// 5 min. Replaces the 6 sequential COUNT(*) queries that /api/v1/
	// stats used to fire on every cache miss with a single indexed
	// row read.
	go func() {
		w := jobs.NewPlatformStatsWorker(pool)
		w.Run(ctx, 5*time.Minute)
	}()

	// Background goroutine: close expired Arena rounds and advance/complete
	// battles. Each replica may run this; the worker uses SKIP LOCKED and
	// durable closed_at markers so a deadline is processed once.
	go func() {
		jobs.NewArenaDeadlineWorker(pool).Run(ctx, 30*time.Second)
	}()

	// Background goroutine: authenticated World Cup schedule/score poller
	// (adaptive cadence; fail-open). football-data.org match resources require
	// an API key, so do not start a rejected-request loop without one.
	if cfg.Sports.PollingEnabled() {
		go func() {
			client := sports.NewClient(cfg.Sports.FootballDataKey)
			sports.NewPoller(client, repository.NewSportsRepo(pool)).WithScorecardTrigger(hub).Run(ctx)
		}()
	} else {
		slog.Info("sports schedule poller disabled: SPORTS_FOOTBALL_DATA_KEY not configured")
	}

	// Background goroutine: ESPN enrichment (timeline events + lineups) for
	// live/imminent matches — fail-open, keyless; enriches whatever matches
	// exist regardless of the football-data key being set.
	go func() {
		sports.NewEnricher(sports.NewESPNClient(), repository.NewSportsRepo(pool)).Run(ctx)
	}()

	// Background goroutines: in-house agents publish World Cup predictions
	// (every 6h, 30 LLM calls/day) so the sports pages have content from
	// day one, and the reactor posts live takes on key match events (every
	// 60s sweep, 150 LLM calls/day). Both reuse the platform's Azure
	// OpenAI creds (cfg.LLM), same gate as the Loom / quality paths —
	// unset means no auto-predictions and no live takes.
	if cfg.LLM.Endpoint != "" && cfg.LLM.APIKey != "" && cfg.LLM.DeploymentName != "" {
		go func() {
			llmClient := loom.NewAzureOpenAIClient(cfg.LLM.Endpoint, cfg.LLM.APIKey)
			go sports.NewReactor(repository.NewSportsRepo(pool), llmClient, cfg.LLM.DeploymentName).Run(ctx)
			sports.NewAutoPredictor(repository.NewSportsRepo(pool), llmClient, cfg.LLM.DeploymentName).Run(ctx)
		}()
	} else {
		slog.Info("sports auto-predictor and reactor disabled: LLM not configured")
	}

	// Background goroutine: GDPR Article 17 hard-delete sweep.
	// Daily cadence — accounts that scheduled deletion more than 7
	// days ago get anonymized (display_name → "[deleted]", email +
	// password blanked). Posts and comments survive with anonymized
	// authorship so reply threads aren't fragmented. First run is
	// 60s after startup so a deploy can't accidentally process a
	// pile of pending deletions before health-checks pass.
	go func() {
		const graceDays = 7
		accountRepo := repository.NewAccountRepo(pool)
		time.Sleep(60 * time.Second)
		for {
			n, err := handlers.AnonymizeReadyAccounts(context.Background(), accountRepo, graceDays)
			if err != nil {
				slog.Error("hard-delete sweep failed", "error", err)
			} else if n > 0 {
				slog.Info("hard-delete sweep anonymized accounts", "count", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(24 * time.Hour):
			}
		}
	}()

	// Background goroutine: delete agent_activity_log entries older than 7 days.
	// Runs once on startup (after 30s warm-up) then every 6 hours.
	go func() {
		time.Sleep(30 * time.Second)
		for {
			result, err := pool.Exec(context.Background(),
				`DELETE FROM agent_activity_log WHERE created_at < NOW() - INTERVAL '7 days'`)
			if err != nil {
				slog.Error("activity log cleanup failed", "error", err)
			} else {
				slog.Info("activity log cleanup", "deleted", result.RowsAffected())
			}
			time.Sleep(6 * time.Hour)
		}
	}()

	// Background goroutine: weekly email digest (Mondays at 09:00 UTC).
	// Sends top 5 posts of the last 7 days to every verified human.
	if sender := email.NewConfiguredSender(cfg.Email); sender != nil {
		go func(sender *email.Sender) {
			for {
				next := digest.NextMondayAt09UTC(time.Now())
				wait := time.Until(next)
				slog.Info("digest: next run scheduled", "at", next.Format(time.RFC3339))
				time.Sleep(wait)

				sent, err := digest.Run(context.Background(), digest.Config{
					Pool:     pool,
					Sender:   sender,
					SiteURL:  cfg.Email.SiteURL,
					UnsubKey: cfg.JWT.Secret,
				})
				if err != nil {
					slog.Error("digest: run failed", "error", err)
				} else {
					slog.Info("digest: completed", "sent", sent)
				}
			}
		}(sender)
	} else {
		slog.Info("digest: disabled because no email provider is configured")
	}

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
