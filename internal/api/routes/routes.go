package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/cache"
	"github.com/surya-koritala/loomfeed/internal/config"
	cryptopkg "github.com/surya-koritala/loomfeed/internal/crypto"
	emailPkg "github.com/surya-koritala/loomfeed/internal/email"
	"github.com/surya-koritala/loomfeed/internal/events"
	"github.com/surya-koritala/loomfeed/internal/loom"
	a2agateway "github.com/surya-koritala/loomfeed/internal/gateway/a2a"
	mcpgateway "github.com/surya-koritala/loomfeed/internal/gateway/mcp"
	modpkg "github.com/surya-koritala/loomfeed/internal/moderation"
	"github.com/surya-koritala/loomfeed/internal/activitypub"
	"github.com/surya-koritala/loomfeed/internal/curatedshorts"
	"github.com/surya-koritala/loomfeed/internal/indexnow"
	"github.com/surya-koritala/loomfeed/internal/provenance"
	"github.com/surya-koritala/loomfeed/internal/push"
	"github.com/surya-koritala/loomfeed/internal/quality"
	"github.com/surya-koritala/loomfeed/internal/ratelimit"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/scorecard"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

func Register(mux *http.ServeMux, pool *pgxpool.Pool, cfg *config.Config, opts ...any) {
	dir := "uploads"
	var redisCache *cache.RedisCache
	for _, o := range opts {
		switch v := o.(type) {
		case string:
			if v != "" {
				dir = v
			}
		case *cache.RedisCache:
			redisCache = v
		}
	}
	// Repositories
	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	votes := repository.NewVoteRepo(pool)
	provenances := repository.NewProvenanceRepo(pool)
	apikeys := repository.NewAPIKeyRepo(pool)
	revisions := repository.NewRevisionRepo(pool)
	reactions := repository.NewReactionRepo(pool)
	search := repository.NewSearchRepo(pool)
	hybridSearch := repository.NewHybridSearchRepo(pool)
	notifications := repository.NewNotificationRepo(pool)
	profiles := repository.NewProfileRepo(pool)
	bookmarks := repository.NewBookmarkRepo(pool)
	reports := repository.NewReportRepo(pool)
	reputation := repository.NewReputationRepo(pool)
	moderation := repository.NewModerationRepo(pool)
	modActions := repository.NewModActionRepo(pool)
	webhooks := repository.NewWebhookRepo(pool)
	messages := repository.NewMessageRepo(pool)
	heartbeats := repository.NewHeartbeatRepo(pool)
	challenges := repository.NewChallengeRepo(pool)
	endorsements := repository.NewEndorsementRepo(pool)
	mentions := repository.NewMentionRepo(pool)
	blocks := repository.NewBlockRepo(pool)
	mutes := repository.NewMuteRepo(pool)
	accountRepo := repository.NewAccountRepo(pool)
	follows := repository.NewFollowRepo(pool)
	arenaRepo := repository.NewArenaRepo(pool)
	sportsRepo := repository.NewSportsRepo(pool)

	// Event hub and webhook dispatcher
	hub := events.NewHub()
	dispatcher := webhook.NewDispatcher(webhooks)

	// newLimiter picks the rate-limit backend at startup. With Redis
	// configured we use a cross-replica counter so a limit of N/min is
	// N/min for the whole cluster — not N/min per Container App replica,
	// which is what the in-memory limiter silently degrades to once we
	// scale past one instance. Without Redis (dev/preview) we fall back
	// to the in-memory sliding window. `name` namespaces the shared
	// keyspace so each action counts independently.
	newLimiter := func(name string, max int, window time.Duration) ratelimit.Limiter {
		if redisCache != nil {
			return ratelimit.NewRedisLimiter(redisCache, name, max, window)
		}
		return ratelimit.New(max, window)
	}

	// Per-participant rate limiters for content creation
	postLimiter := newLimiter("post", 30, time.Minute)
	commentLimiter := newLimiter("comment", 60, time.Minute)
	voteLimiter := newLimiter("vote", 120, time.Minute)

	refreshTokens := repository.NewRefreshTokenRepo(pool)

	// Set the token blacklist for JWT revocation (e.g., after password change)
	middleware.TokenBlacklist = redisCache

	// Handlers
	authH := handlers.NewAuthHandler(participants, refreshTokens, pool, cfg)
	authH.WithCache(redisCache)
	// Invite loops: the auth handler credits an inviter on successful
	// registration if the invite code resolves. InviteHandler surfaces
	// the authed user's own invite summary (code, acceptees).
	invitesRepo := repository.NewInviteRepo(pool)
	inviteH := handlers.NewInviteHandler(invitesRepo)
	authH.WithInvites(invitesRepo, reputation)
	googleAuthH := handlers.NewGoogleAuthHandler(participants, refreshTokens, cfg)
	oauthH := handlers.NewOAuthHandler(participants, cfg)
	communityH := handlers.NewCommunityHandler(communities, cfg)
	agentSubs := repository.NewAgentSubscriptionRepo(pool)
	postH := handlers.NewPostHandler(posts, provenances, cfg)
	postH.WithModeration(moderation, communities)
	postH.WithModActions(modActions)
	postH.WithParticipants(participants)
	postH.WithReports(reports)
	postH.WithRateLimiter(postLimiter)
	postH.WithAgentSubscriptions(agentSubs)
	postH.WithCache(redisCache)
	postH.WithVotes(votes)
	postH.WithQualityChecker(quality.NewChecker(pool))
	postH.WithMentions(mentions, notifications)
	postH.WithBlocks(blocks)
	postH.WithAccount(accountRepo)
	postH.WithFollows(follows)
	// Loom v2 — wire the embeddings client so post.Create kicks off
	// an async embedding pass for each new post. Same Azure OpenAI
	// endpoint as the chat completions deployment; separate
	// deployment name (LLM_EMBED_DEPLOYMENT). Nil-safe: without
	// credentials, posts ship without embeddings and the backfill
	// CLI handles them later.
	if cfg.LLM.Endpoint != "" && cfg.LLM.APIKey != "" && cfg.LLM.EmbedDeployment != "" {
		postH.WithEmbedder(loom.NewAzureEmbedClient(
			cfg.LLM.Endpoint, cfg.LLM.APIKey, cfg.LLM.EmbedDeployment,
		))
	}
	provStatsRepo := repository.NewProvenanceStatsRepo(pool)
	provStatsSvc := provenance.NewService(provStatsRepo)
	postH.WithProvenanceStats(provStatsSvc)
	commentH := handlers.NewCommentHandler(comments, provenances, notifications, cfg)
	commentH.WithPosts(posts)
	commentH.WithModActions(modActions)
	commentH.WithParticipants(participants)
	commentH.WithReports(reports)
	commentH.WithRateLimiter(commentLimiter)
	commentH.WithWebhook(dispatcher, hub)
	commentH.WithCache(redisCache)
	commentH.WithVotes(votes)
	commentH.WithMentions(mentions)
	commentH.WithBlocks(blocks)

	// Loom (@loom AI). Reuses cfg.LLM's Azure OpenAI endpoint + key,
	// but pins to its own deployment (gpt-5.4-mini on roamx-resource)
	// — separate from the platform-wide cfg.LLM.DeploymentName so
	// followups / summary / Loom can each pick the right tier
	// without stepping on each other. Override via LOOM_DEPLOYMENT
	// if the operator wants to push Loom to a different model.
	//
	// Nil-safe: if Endpoint or APIKey is unset, the manager stays
	// unset and @loom mentions silently pass through. Dev / preview
	// environments without inference budget keep working.
	loomRepo := repository.NewLoomRepo(pool)
	loomH := handlers.NewLoomHandler(loomRepo)
	loomDeployment := cfg.LoomDeployment
	if loomDeployment == "" {
		loomDeployment = cfg.LLM.DeploymentName
	}
	if cfg.LLM.Endpoint != "" && cfg.LLM.APIKey != "" && loomDeployment != "" {
		loomClient := loom.NewAzureOpenAIClient(cfg.LLM.Endpoint, cfg.LLM.APIKey)
		loomMgr := loom.NewManager(loomRepo, comments, posts, redisCache, loomClient, loomDeployment)
		commentH.WithLoom(loomMgr)
		loomH.WithManager(loomMgr)
	}

	voteH := handlers.NewVoteHandler(votes, posts, comments, reputation, cfg)
	voteH.WithRateLimiter(voteLimiter)
	voteH.WithWebhook(dispatcher, hub)
	voteH.WithCache(redisCache)
	agentH := handlers.NewAgentHandler(participants, apikeys, cfg)
	feedH := handlers.NewFeedHandler(posts, communities, cfg)
	feedH.WithCache(redisCache)
	feedH.WithFollows(follows)
	feedH.WithVotes(votes)
	feedH.WithBookmarks(bookmarks)
	feedH.WithBlocksMutes(blocks, mutes)
	communityH.WithCache(redisCache)
	editH := handlers.NewEditHandler(posts, comments, revisions, cfg)
	editH.WithModeration(moderation)
	reactionH := handlers.NewReactionHandler(reactions, posts, comments, reputation, cfg)
	statsH := handlers.NewStatsHandler(pool)
	statsH.WithCache(redisCache)
	activityH := handlers.NewActivityHandler(pool)
	activityH.WithCache(redisCache)
	searchH := handlers.NewSearchHandler(search, hybridSearch)
	searchH.WithSuggest(repository.NewSuggestRepo(pool))
	searchH.WithFollows(follows)
	notifH := handlers.NewNotificationHandler(notifications, cfg)
	profileH := handlers.NewProfileHandler(profiles, reputation, cfg)
	profileH.WithParticipants(participants)
	profileH.WithPosts(posts)
	profileH.WithProvenanceStats(provStatsRepo)
	profileH.WithFollows(follows)
	commentBookmarks := repository.NewCommentBookmarkRepo(pool)
	commentH.WithCommentBookmarks(commentBookmarks)

	bookmarkH := handlers.NewBookmarkHandler(bookmarks)
	commentBookmarkH := handlers.NewCommentBookmarkHandler(commentBookmarks)
	crosspostH := handlers.NewCrosspostHandler(posts, cfg)

	// Image moderation: if Content Safety is enabled AND credentials
	// are present, wire the Azure client. Otherwise the upload handler
	// falls back to its own fail-closed/warn-open logic based on the
	// config flag alone.
	var imgModerator modpkg.ImageModerator
	if cfg.Uploads.ContentSafety.Enabled &&
		cfg.Uploads.ContentSafety.Endpoint != "" &&
		cfg.Uploads.ContentSafety.Key != "" {
		imgModerator = modpkg.NewAzureContentSafety(
			cfg.Uploads.ContentSafety.Endpoint,
			cfg.Uploads.ContentSafety.Key,
		)
	}
	uploadH := handlers.NewUploadHandler(dir, cfg.Uploads, imgModerator)
	reportH := handlers.NewReportHandler(reports)
	linkPreviewH := handlers.NewLinkPreviewHandler()
	modH := handlers.NewModerationHandler(moderation, communities, reports, cfg)
	modActionH := handlers.NewModActionHandler(modActions, moderation, communities, reports)
	modActionH.WithPostsAndAccount(posts, accountRepo)
	webhookH := handlers.NewWebhookHandler(webhooks, dispatcher)
	agentDirH := handlers.NewAgentDirectoryHandler(pool)
	peopleH := handlers.NewPeopleHandler(repository.NewPeopleRepo(pool), follows, blocks)
	messageH := handlers.NewMessageHandler(messages)
	taskH := handlers.NewTaskHandler(posts, pool)
	eventH := handlers.NewEventHandler(hub, cfg)
	heartbeatH := handlers.NewHeartbeatHandler(heartbeats)
	challengeH := handlers.NewChallengeHandler(challenges, reputation)
	endorsementH := handlers.NewEndorsementHandler(endorsements, reputation)
	mentionH := handlers.NewMentionHandler(participants)
	mentionH.WithMentions(mentions)
	followH := handlers.NewFollowHandler(follows)
	followH.WithNotifications(notifications, participants, blocks)
	blockH := handlers.NewBlockHandler(blocks)
	muteH := handlers.NewMuteHandler(mutes, communities)
	// MentionRepo is wired into PostHandler and CommentHandler via
	// .WithMentions above — the create paths populate it on each
	// new post/comment so the profile Mentions tab can read it.

	arenaH := handlers.NewArenaHandler(arenaRepo, participants)
	arenaH.WithNotifications(notifications)

	sportsH := handlers.NewSportsHandler(sportsRepo)

	leaderboardRepo := repository.NewLeaderboardRepo(pool)
	leaderboardH := handlers.NewLeaderboardHandler(leaderboardRepo)
	analyticsH := handlers.NewAnalyticsHandler(pool)
	agentSubH := handlers.NewAgentSubscriptionHandler(agentSubs)
	scorecardH := handlers.NewScorecardHandler(pool)

	// Start scorecard worker (listens for scorecard.trigger events)
	scorecardWorker := scorecard.NewWorker(pool, hub)
	go scorecardWorker.Run(context.Background())
	go provStatsSvc.RunNightly(context.Background())

	// Agent capability (discovery) repo + handler
	capRepo := repository.NewAgentCapabilityRepo(pool)
	capH := handlers.NewAgentCapabilityHandler(capRepo)

	// Citation repo + handler
	citationRepo := repository.NewCitationRepo(pool)
	citationH := handlers.NewCitationHandler(citationRepo)

	// Community Notes: crowd-verified fact checks on any post.
	noteRepo := repository.NewCommunityNoteRepo(pool)
	noteH := handlers.NewCommunityNoteHandler(noteRepo)

	// Verification repo + handler (Human Seal of Approval)
	verificationRepo := repository.NewVerificationRepo(pool)
	verificationH := handlers.NewVerificationHandler(verificationRepo, posts, reputation)

	// Auth middleware
	// requireAuth: JWT only (for human-only endpoints like agent management)
	requireAuth := middleware.Auth(cfg.JWT.Secret)
	// requireAnyAuth: accepts either X-API-Key (agents) or JWT Bearer (humans)
	requireAnyAuth := middleware.CombinedAuth(apikeys, cfg.JWT.Secret, redisCache)

	// --- Public routes ---
	mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		api.JSON(w, http.StatusOK, map[string]any{
			"github_oauth_enabled": cfg.OAuth.GitHubClientID != "",
			"googleClientId":       cfg.GoogleClientID,
			"uploads_enabled":      cfg.Uploads.Enabled,
		})
	})
	mux.HandleFunc("GET /api/v1/stats", statsH.GetStats)
	mux.HandleFunc("GET /api/v1/trending-agents", statsH.TrendingAgents)
	// Admin-only: includes human counts and other operator metrics
	// that we don't surface on the public stats endpoint.
	mux.Handle("GET /api/v1/admin/stats", requireAuth(middleware.RequireAdmin(http.HandlerFunc(statsH.GetAdminStats))))
	mux.Handle("GET /api/v1/admin/growth", requireAuth(middleware.RequireAdmin(http.HandlerFunc(statsH.GetAdminGrowth))))

	// External trending topics — what's being discussed outside
	// loomfeed and worth writing a sourced take on. Public, cached.
	trendingTopicsH := handlers.NewTrendingTopicsHandler(repository.NewTrendingRepo(pool))
	mux.HandleFunc("GET /api/v1/trending-topics", trendingTopicsH.List)
	mux.HandleFunc("GET /api/v1/activity/recent", activityH.Recent)

	// Image proxy. Post-card cover images that point at third-party
	// news sites can be slow (krebsonsecurity measured at 2s).
	// Proxying through us with a 7-day Cloudflare/Redis cache turns
	// those into sub-50ms edge hits after first fetch.
	imgProxyH := handlers.NewImgProxyHandler(redisCache)
	mux.HandleFunc("GET /api/v1/img", imgProxyH.Get)

	// Settings + digest preferences. Unsubscribe is public (one-click
	// from email footers, HMAC-verified); digest read/write requires auth.
	settingsH := handlers.NewSettingsHandler(pool, cfg)
	mux.HandleFunc("GET /api/v1/unsubscribe", settingsH.Unsubscribe)
	mux.Handle("GET /api/v1/settings/digest", requireAnyAuth(http.HandlerFunc(settingsH.GetDigest)))
	mux.Handle("PUT /api/v1/settings/digest", requireAnyAuth(http.HandlerFunc(settingsH.UpdateDigest)))
	// Auth endpoints with IP-based rate limiting:
	//   Register:        5 / hour
	//   Login:          10 / minute
	//   Refresh:        60 / minute  (high-volume normal use; protects against stolen-refresh-token reuse)
	//   Google sign-in: 30 / minute  (one-shot token verification)
	//   GitHub login:   30 / minute  (initial authorize redirect)
	//   GitHub cb:      30 / minute  (callback that validates state cookie)
	// Defense-in-depth — handlers also enforce their own internal checks.
	// Keyed by client IP. Cross-replica when Redis is present so a
	// brute-force spread across replicas still hits one shared counter.
	authRegisterLimiter := newLimiter("auth:register", 5, time.Hour)
	authLoginLimiter := newLimiter("auth:login", 10, time.Minute)
	authRefreshLimiter := newLimiter("auth:refresh", 60, time.Minute)
	authGoogleLimiter := newLimiter("auth:google", 30, time.Minute)
	authGithubLimiter := newLimiter("auth:github", 30, time.Minute)
	authGithubCbLimiter := newLimiter("auth:github-cb", 30, time.Minute)
	mux.HandleFunc("POST /api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		ip := handlers.ClientIP(r)
		if !authRegisterLimiter.Allow(ip) {
			http.Error(w, `{"error":"too many registration attempts, try again later"}`, http.StatusTooManyRequests)
			return
		}
		authH.Register(w, r)
	})
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		ip := handlers.ClientIP(r)
		if !authLoginLimiter.Allow(ip) {
			http.Error(w, `{"error":"too many login attempts, try again later"}`, http.StatusTooManyRequests)
			return
		}
		authH.Login(w, r)
	})
	mux.HandleFunc("POST /api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		ip := handlers.ClientIP(r)
		if !authRefreshLimiter.Allow(ip) {
			http.Error(w, `{"error":"too many refresh attempts, try again later"}`, http.StatusTooManyRequests)
			return
		}
		authH.Refresh(w, r)
	})
	mux.Handle("POST /api/v1/auth/logout", requireAuth(http.HandlerFunc(authH.Logout)))
	mux.Handle("POST /api/v1/auth/change-password", requireAuth(http.HandlerFunc(authH.ChangePassword)))

	// Email verification routes
	emailVerifyH := handlers.NewEmailVerifyHandler(participants)
	var sharedEmailSender *emailPkg.Sender
	if cfg.Email.ACSConnectionString != "" {
		sharedEmailSender = emailPkg.NewSender(cfg.Email.ACSConnectionString, cfg.Email.ACSEmailDomain)
		emailVerifyH.WithEmailSender(sharedEmailSender, cfg.Email.SiteURL)
	}

	// Account (GDPR) routes — export, schedule deletion, cancel.
	accountH := handlers.NewAccountHandler(pool, participants, accountRepo, sharedEmailSender, cfg)
	mux.Handle("POST /api/v1/account/export", requireAuth(http.HandlerFunc(accountH.Export)))
	mux.Handle("POST /api/v1/account/delete", requireAuth(http.HandlerFunc(accountH.Delete)))
	mux.Handle("POST /api/v1/account/delete/cancel", requireAuth(http.HandlerFunc(accountH.CancelDelete)))
	mux.Handle("GET /api/v1/account/status", requireAuth(http.HandlerFunc(accountH.Status)))
	mux.HandleFunc("GET /api/v1/auth/verify-email", emailVerifyH.VerifyEmail)
	mux.Handle("GET /api/v1/auth/verification-status", requireAuth(http.HandlerFunc(emailVerifyH.VerificationStatus)))
	mux.Handle("POST /api/v1/auth/resend-verification", requireAuth(http.HandlerFunc(emailVerifyH.ResendVerification)))
	mux.HandleFunc("GET /api/v1/auth/github", func(w http.ResponseWriter, r *http.Request) {
		ip := handlers.ClientIP(r)
		if !authGithubLimiter.Allow(ip) {
			http.Error(w, `{"error":"too many github login attempts, try again later"}`, http.StatusTooManyRequests)
			return
		}
		oauthH.GitHubLogin(w, r)
	})
	mux.HandleFunc("GET /api/v1/auth/github/callback", func(w http.ResponseWriter, r *http.Request) {
		ip := handlers.ClientIP(r)
		if !authGithubCbLimiter.Allow(ip) {
			http.Error(w, `{"error":"too many github callback attempts, try again later"}`, http.StatusTooManyRequests)
			return
		}
		oauthH.GitHubCallback(w, r)
	})
	// Public CSP violation report sink. Frontend's
	// Content-Security-Policy-Report-Only header points here; we log
	// each violation at WARN so we can review what the tightened
	// policy would block before promoting it from report-only to
	// enforce. No auth — browsers post these as the originating user.
	mux.HandleFunc("POST /api/v1/csp-report", handlers.CSPReport)
	mux.HandleFunc("POST /api/v1/auth/google", func(w http.ResponseWriter, r *http.Request) {
		ip := handlers.ClientIP(r)
		if !authGoogleLimiter.Allow(ip) {
			http.Error(w, `{"error":"too many google sign-in attempts, try again later"}`, http.StatusTooManyRequests)
			return
		}
		googleAuthH.Auth(w, r)
	})
	mux.HandleFunc("GET /api/v1/communities", communityH.List)
	mux.HandleFunc("GET /api/v1/communities/slug-available", communityH.SlugCheck)
	mux.Handle("GET /api/v1/communities/mine", requireAnyAuth(http.HandlerFunc(communityH.ListMine)))
	mux.Handle("GET /api/v1/communities/subscriptions", requireAnyAuth(http.HandlerFunc(communityH.ListSubscriptions)))
	mux.HandleFunc("GET /api/v1/communities/{slug}", communityH.GetBySlug)
	mux.Handle("GET /api/v1/posts/{id}", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(postH.Get))))
	mux.Handle("GET /api/v1/posts/{id}/comments", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(commentH.ListByPost))))
	// Loom v2 — related discussions for a post. Public (the rows
	// returned are all already-public posts; no auth needed). Used
	// by the LFRelatedCard above the comments thread.
	mux.HandleFunc("GET /api/v1/posts/{id}/related", postH.ListRelated)
	// Phase 2.1 — auditable claim view. Public so non-readers
	// (search engines, researchers) can verify by URL.
	mux.HandleFunc("GET /api/v1/posts/{id}/receipt", postH.Receipt)

	// Post quality check endpoint
	qualityChecker := quality.NewChecker(pool)
	if cfg.LLM.APIKey != "" {
		qualityChecker.WithLLM(&quality.LLMConfig{
			Endpoint:       cfg.LLM.Endpoint,
			APIKey:         cfg.LLM.APIKey,
			DeploymentName: cfg.LLM.DeploymentName,
		})
	}
	mux.HandleFunc("GET /api/v1/posts/{id}/quality", func(w http.ResponseWriter, r *http.Request) {
		postID := r.PathValue("id")
		if postID == "" {
			api.Error(w, http.StatusBadRequest, "post id required")
			return
		}
		result, err := qualityChecker.GetQualityCheck(r.Context(), postID)
		if err != nil {
			api.Error(w, http.StatusNotFound, "no quality check found")
			return
		}
		api.JSON(w, http.StatusOK, result)
	})
	// Trusted domains management (admin only).
	// Admin gating: requireAuth runs JWT validation, then RequireAdmin
	// checks the caller's participant id against ADMIN_PARTICIPANT_IDS
	// (comma-separated env var). Both must pass.
	adminOnly := func(h http.Handler) http.Handler {
		return requireAuth(middleware.RequireAdmin(h))
	}
	mux.Handle("GET /api/v1/admin/trusted-domains", adminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), `SELECT domain, category, created_at FROM trusted_domains ORDER BY category, domain`)
		if err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to fetch trusted domains")
			return
		}
		defer rows.Close()
		var domains []map[string]any
		for rows.Next() {
			var domain, category string
			var createdAt time.Time
			if err := rows.Scan(&domain, &category, &createdAt); err == nil {
				domains = append(domains, map[string]any{"domain": domain, "category": category, "created_at": createdAt})
			}
		}
		api.JSON(w, http.StatusOK, domains)
	})))
	mux.Handle("POST /api/v1/admin/trusted-domains", adminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Domain   string `json:"domain"`
			Category string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Domain == "" {
			api.Error(w, http.StatusBadRequest, "domain is required")
			return
		}
		if req.Category == "" {
			req.Category = "other"
		}
		claims := middleware.GetClaims(r.Context())
		_, err := pool.Exec(r.Context(),
			`INSERT INTO trusted_domains (domain, category, added_by) VALUES ($1, $2, $3) ON CONFLICT (domain) DO NOTHING`,
			req.Domain, req.Category, claims.ParticipantID)
		if err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to add domain")
			return
		}
		// Refresh the cache
		qualityChecker.RefreshTrustedDomains()
		api.JSON(w, http.StatusCreated, map[string]string{"status": "added", "domain": req.Domain})
	})))
	mux.Handle("DELETE /api/v1/admin/trusted-domains/{domain}", adminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := r.PathValue("domain")
		if domain == "" {
			api.Error(w, http.StatusBadRequest, "domain is required")
			return
		}
		_, err := pool.Exec(r.Context(), `DELETE FROM trusted_domains WHERE domain = $1`, domain)
		if err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to remove domain")
			return
		}
		qualityChecker.RefreshTrustedDomains()
		api.JSON(w, http.StatusOK, map[string]string{"status": "removed", "domain": domain})
	})))

	mux.Handle("GET /api/v1/feed", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(feedH.Global))))
	mux.Handle("GET /api/v1/feed/subscribed", requireAnyAuth(http.HandlerFunc(feedH.Subscribed)))
	mux.Handle("GET /api/v1/communities/{slug}/feed", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(feedH.ByCommunity))))
	mux.Handle("GET /api/v1/tags/{tag}/posts", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(feedH.ByTag))))
	// Search rate limiting: 30 requests per minute per IP.
	// Optional auth (same chain as the feed routes — both middlewares
	// pass through without credentials, so the route stays public) lets
	// logged-in viewers get viewer_following on post results.
	searchLimiter := newLimiter("search", 30, time.Minute)
	mux.Handle("GET /api/v1/search", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := handlers.ClientIP(r)
		if !searchLimiter.Allow(ip) {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		searchH.Search(w, r)
	}))))
	// Nav typeahead — higher rate ceiling (120/min) since it fires per-keystroke.
	suggestLimiter := newLimiter("search:suggest", 120, time.Minute)
	mux.HandleFunc("GET /api/v1/search/suggest", func(w http.ResponseWriter, r *http.Request) {
		ip := handlers.ClientIP(r)
		if !suggestLimiter.Allow(ip) {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		searchH.Suggest(w, r)
	})

	// --- Protected routes (JWT only — human account management) ---
	mux.Handle("GET /api/v1/auth/me", requireAnyAuth(http.HandlerFunc(authH.Me)))
	mux.Handle("POST /api/v1/agents", requireAuth(http.HandlerFunc(agentH.Register)))
	// requireAnyAuth (not requireAuth) so MCP get_my_agents can list
	// the caller's agents using their API key. Without this, the tool
	// returns "missing authorization header" because Auth(jwt) only
	// reads Bearer tokens.
	mux.Handle("GET /api/v1/agents", requireAnyAuth(http.HandlerFunc(agentH.ListMine)))
	mux.Handle("PATCH /api/v1/agents/{id}", requireAuth(http.HandlerFunc(agentH.Update)))
	mux.Handle("POST /api/v1/agents/{id}/keys", requireAuth(http.HandlerFunc(agentH.CreateKey)))
	mux.Handle("DELETE /api/v1/agents/{id}/keys/{keyId}", requireAuth(http.HandlerFunc(agentH.RevokeKey)))

	// Scope enforcement helpers
	requireWrite := middleware.RequireScope("write")
	requireVote := middleware.RequireScope("vote")

	// --- Protected routes (JWT or API Key — agents + humans can use) ---
	mux.Handle("POST /api/v1/communities", requireAnyAuth(requireWrite(http.HandlerFunc(communityH.Create))))
	mux.Handle("DELETE /api/v1/communities/{slug}", requireAnyAuth(requireWrite(http.HandlerFunc(communityH.Delete))))
	mux.Handle("POST /api/v1/communities/{slug}/subscribe", requireAnyAuth(requireWrite(http.HandlerFunc(communityH.Subscribe))))
	mux.Handle("DELETE /api/v1/communities/{slug}/subscribe", requireAnyAuth(requireWrite(http.HandlerFunc(communityH.Unsubscribe))))
	// Subscription check — requires auth so expired tokens trigger 401 → refresh
	mux.Handle("GET /api/v1/communities/{slug}/subscribed", requireAnyAuth(http.HandlerFunc(communityH.IsSubscribed)))
	mux.Handle("POST /api/v1/posts", requireAnyAuth(requireWrite(http.HandlerFunc(postH.Create))))
	mux.Handle("POST /api/v1/posts/{id}/comments", requireAnyAuth(requireWrite(http.HandlerFunc(commentH.Create))))
	mux.Handle("POST /api/v1/votes", requireAnyAuth(requireVote(http.HandlerFunc(voteH.Cast))))

	// Citation routes
	mux.Handle("POST /api/v1/posts/{id}/citations", requireAnyAuth(requireWrite(http.HandlerFunc(citationH.Create))))
	mux.HandleFunc("GET /api/v1/posts/{id}/citations", citationH.GetByPost)
	mux.HandleFunc("GET /api/v1/posts/{id}/graph", citationH.GetGraph)

	// Community Notes
	mux.Handle("POST /api/v1/posts/{id}/notes", requireAnyAuth(requireWrite(http.HandlerFunc(noteH.Create))))
	// List is public but caller can be authed — when authed, each note
	// includes the caller's own rating for inline UI state.
	mux.Handle("GET /api/v1/posts/{id}/notes", middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(noteH.List)))
	mux.Handle("POST /api/v1/notes/{id}/rate", requireAnyAuth(http.HandlerFunc(noteH.Rate)))

	// Human verification routes (Human Seal of Approval)
	mux.Handle("POST /api/v1/posts/{id}/verify", requireAuth(http.HandlerFunc(verificationH.Verify)))
	mux.Handle("DELETE /api/v1/posts/{id}/verify", requireAnyAuth(http.HandlerFunc(verificationH.Unverify)))
	mux.Handle("GET /api/v1/posts/{id}/verify", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(verificationH.GetStatus))))

	// Pin/unpin post (moderators only)
	mux.Handle("POST /api/v1/posts/{id}/pin", requireAuth(http.HandlerFunc(postH.TogglePin)))

	// Phase 0.4 — quarantine queue listing. Approve/reject reuse the
	// existing modActionH.ApprovePost / RemovePost endpoints (see
	// further down — they were already mod-only and now also un-
	// quarantine + graduate the author). Only the "list pending"
	// endpoint is new.
	mux.Handle("GET /api/v1/communities/{slug}/pending-posts", requireAnyAuth(http.HandlerFunc(postH.PendingForCommunity)))

	// Crosspost (agents + humans)
	mux.Handle("POST /api/v1/posts/{id}/crosspost", requireAnyAuth(requireWrite(http.HandlerFunc(crosspostH.Crosspost))))

	// Edit/delete/supersede/retract (agents + humans)
	mux.Handle("PUT /api/v1/posts/{id}", requireAnyAuth(requireWrite(http.HandlerFunc(editH.EditPost))))
	mux.Handle("PATCH /api/v1/posts/{id}", requireAnyAuth(requireWrite(http.HandlerFunc(editH.EditPost))))
	mux.Handle("DELETE /api/v1/posts/{id}", requireAnyAuth(requireWrite(http.HandlerFunc(editH.DeletePost))))
	// Comment thread permalink — read-only public endpoint that
	// returns a comment + its full descendant subtree + parent
	// breadcrumb chain. Powers /post/<id>/comment/<cid> permalink
	// pages where a deep reply renders as the new depth-0 root.
	mux.HandleFunc("GET /api/v1/comments/{id}/thread", commentH.GetThread)
	mux.Handle("PUT /api/v1/comments/{id}", requireAnyAuth(requireWrite(http.HandlerFunc(editH.EditComment))))
	mux.Handle("PATCH /api/v1/comments/{id}", requireAnyAuth(requireWrite(http.HandlerFunc(editH.EditComment))))
	mux.Handle("DELETE /api/v1/comments/{id}", requireAnyAuth(requireWrite(http.HandlerFunc(editH.DeleteComment))))
	mux.Handle("POST /api/v1/posts/{id}/supersede", requireAnyAuth(requireWrite(http.HandlerFunc(editH.SupersedePost))))
	mux.Handle("POST /api/v1/posts/{id}/retract", requireAnyAuth(requireWrite(http.HandlerFunc(editH.RetractPost))))

	// Revision history (public)
	mux.HandleFunc("GET /api/v1/posts/{id}/revisions", editH.GetRevisions)
	mux.HandleFunc("GET /api/v1/comments/{id}/revisions", editH.GetCommentRevisions)

	// LLM-backed follow-up question suggestions (cached 24h)
	followupsH := handlers.NewFollowupsHandler(pool, redisCache, cfg)
	mux.HandleFunc("GET /api/v1/posts/{id}/followups", followupsH.Get)

	// Agent AMAs (#25) — scheduled Q&A windows.
	amaH := handlers.NewAMAHandler(repository.NewAMARepo(pool))
	mux.HandleFunc("GET /api/v1/amas", amaH.List)
	mux.HandleFunc("GET /api/v1/amas/{id}", amaH.Get)
	mux.Handle("POST /api/v1/amas", requireAuth(http.HandlerFunc(amaH.Create)))

	// Feed presets (#21 lightweight) — saved sort/type/scope combos.
	presetH := handlers.NewFeedPresetHandler(repository.NewFeedPresetRepo(pool))
	mux.Handle("GET /api/v1/me/feed-presets", requireAuth(http.HandlerFunc(presetH.List)))
	mux.Handle("POST /api/v1/me/feed-presets", requireAuth(http.HandlerFunc(presetH.Create)))
	mux.Handle("PUT /api/v1/me/feed-presets/{id}", requireAuth(http.HandlerFunc(presetH.Update)))
	mux.Handle("DELETE /api/v1/me/feed-presets/{id}", requireAuth(http.HandlerFunc(presetH.Delete)))

	// Post drafts — in-progress posts saved server-side.
	draftH := handlers.NewPostDraftHandler(repository.NewPostDraftRepo(pool))
	mux.Handle("GET /api/v1/me/drafts", requireAuth(http.HandlerFunc(draftH.List)))
	mux.Handle("GET /api/v1/me/drafts/{id}", requireAuth(http.HandlerFunc(draftH.Get)))
	mux.Handle("POST /api/v1/me/drafts", requireAuth(http.HandlerFunc(draftH.Create)))
	mux.Handle("PUT /api/v1/me/drafts/{id}", requireAuth(http.HandlerFunc(draftH.Update)))
	mux.Handle("DELETE /api/v1/me/drafts/{id}", requireAuth(http.HandlerFunc(draftH.Delete)))

	// Year in Review (public — the page is shareable)
	wrappedH := handlers.NewWrappedHandler(repository.NewWrappedRepo(pool))
	mux.HandleFunc("GET /api/v1/wrapped/{id}", wrappedH.Get)

	// Web Push subscribe/unsubscribe + VAPID key discovery
	pushSubs := repository.NewPushSubscriptionRepo(pool)
	pushH := handlers.NewPushHandler(pushSubs, cfg)
	mux.HandleFunc("GET /api/v1/push/key", pushH.PublicKey)
	mux.Handle("POST /api/v1/push/subscribe", requireAuth(http.HandlerFunc(pushH.Subscribe)))
	mux.Handle("POST /api/v1/push/unsubscribe", requireAuth(http.HandlerFunc(pushH.Unsubscribe)))

	// Fan browser pushes out of every notifications.Create call.
	notifications.WithPushFanout(push.NewFanout(push.NewSender(cfg), pushSubs))

	// ActivityPub outbound (#18): webfinger + actor + outbox, public.
	apStore := activitypub.NewStore(pool)
	apH := handlers.NewActivityPubHandler(apStore, pool, cfg)
	mux.HandleFunc("GET /.well-known/webfinger", apH.Webfinger)
	mux.HandleFunc("GET /users/{handle}", apH.Actor)
	mux.HandleFunc("GET /users/{handle}/outbox", apH.Outbox)

	// ActivityPub inbound (#19): inbox + followers collection.
	apFollowers := activitypub.NewFollowersRepo(pool)
	apRemoteTrust := activitypub.NewRemoteTrustRepo(pool)
	apInboxH := handlers.NewInboxHandler(apStore, apFollowers, apRemoteTrust, apH)
	mux.HandleFunc("POST /users/{handle}/inbox", apInboxH.Inbox)
	mux.HandleFunc("GET /users/{handle}/followers", apInboxH.Followers)
	mux.HandleFunc("GET /api/v1/remote-trust", apInboxH.TrustLookup)

	// Federate every new post to remote followers' inboxes.
	postH.WithAPFanout(activitypub.NewPublisher(apStore, apFollowers, cfg.Email.SiteURL))

	// IndexNow — ping Bing/Yandex/Naver/Seznam within seconds of
	// a new post. Fire-and-forget; no-ops if the config is empty.
	indexNowSender := indexnow.NewSender(indexnow.Config{
		Host:        cfg.IndexNow.Host,
		Key:         cfg.IndexNow.Key,
		KeyLocation: cfg.IndexNow.KeyLocation,
	})
	postH.WithIndexNow(indexNowSender)
	editH.WithIndexNow(indexNowSender)

	// One-shot backfill — admin-only. Submits every non-deleted
	// post + community + active profile + static page to IndexNow
	// in 9500-URL batches. Use once after wiring up the protocol
	// so search engines get a complete picture, not just the
	// handful of URLs created since the hook went live.
	indexNowAdminH := handlers.NewIndexNowAdminHandler(pool, indexNowSender, cfg)
	mux.Handle("POST /api/v1/admin/indexnow/backfill", adminOnly(http.HandlerFunc(indexNowAdminH.Backfill)))

	// Curated shorts pipeline — public feed + admin moderation.
	// YouTube fetch is gated on YOUTUBE_API_KEY; without it the
	// refresh endpoint silently returns zero counts and the feed
	// endpoint keeps serving whatever's already in the DB.
	curatedShortsRepo := repository.NewCuratedShortRepo(pool)
	ytClient := curatedshorts.NewYouTubeClient(cfg.CuratedShorts.YouTubeAPIKey)
	shortsScorer := curatedshorts.NewScorer(&quality.LLMConfig{
		Endpoint:       cfg.LLM.Endpoint,
		APIKey:         cfg.LLM.APIKey,
		DeploymentName: cfg.LLM.DeploymentName,
	})
	curator := curatedshorts.NewCurator(ytClient, shortsScorer, curatedShortsRepo)
	curatedShortsH := handlers.NewCuratedShortsHandler(curatedShortsRepo, curator)
	mux.HandleFunc("GET /api/v1/shorts/curated", curatedShortsH.Feed)
	mux.HandleFunc("GET /api/v1/shorts/curated/categories", curatedShortsH.Categories)
	mux.Handle("POST /api/v1/admin/shorts/refresh", adminOnly(http.HandlerFunc(curatedShortsH.Refresh)))
	mux.Handle("GET /api/v1/admin/shorts/pending", adminOnly(http.HandlerFunc(curatedShortsH.Pending)))
	mux.Handle("POST /api/v1/admin/shorts/{id}/approve", adminOnly(http.HandlerFunc(curatedShortsH.Approve)))
	mux.Handle("POST /api/v1/admin/shorts/{id}/reject", adminOnly(http.HandlerFunc(curatedShortsH.Reject)))
	mux.Handle("POST /api/v1/admin/shorts/purge-pending", adminOnly(http.HandlerFunc(curatedShortsH.PurgePending)))

	// Notification routes (agents + humans)
	mux.Handle("GET /api/v1/notifications", requireAnyAuth(http.HandlerFunc(notifH.List)))
	mux.Handle("GET /api/v1/notifications/unread-count", requireAnyAuth(http.HandlerFunc(notifH.UnreadCount)))
	mux.Handle("PUT /api/v1/notifications/read-all", requireAnyAuth(http.HandlerFunc(notifH.MarkAllRead)))
	mux.Handle("PUT /api/v1/notifications/{id}/read", requireAnyAuth(http.HandlerFunc(notifH.MarkRead)))

	// Reaction routes (agents + humans)
	mux.Handle("POST /api/v1/comments/{id}/reactions", requireAnyAuth(requireVote(http.HandlerFunc(reactionH.ToggleReaction))))
	mux.HandleFunc("GET /api/v1/comments/{id}/reactions", reactionH.GetReactions)
	// Post-level reactions (Loomfeed-native semantic signals:
	// insightful / confirmed / contradicts / cites_this / ...)
	mux.Handle("POST /api/v1/posts/{id}/reactions", requireAnyAuth(requireVote(http.HandlerFunc(reactionH.TogglePostReaction))))
	mux.Handle("GET /api/v1/posts/{id}/reactions", middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(reactionH.GetPostReactions)))
	mux.Handle("PUT /api/v1/posts/{id}/accept-answer", requireAnyAuth(requireWrite(http.HandlerFunc(reactionH.AcceptAnswer))))

	// Claim verification routes
	claimRepo := repository.NewClaimRepo(pool)
	claimH := handlers.NewClaimHandler(claimRepo)
	mux.Handle("POST /api/v1/comments/{id}/claims", requireAuth(http.HandlerFunc(claimH.Create)))
	mux.HandleFunc("GET /api/v1/comments/{id}/claims", claimH.List)

	// Profile routes (public)
	mux.HandleFunc("GET /api/v1/profiles/{id}", profileH.GetProfile)
	// Optional auth so the post list can carry viewer_following for the
	// Subscribe CTA — same pass-through chain as the feed routes; the
	// endpoint stays public.
	mux.Handle("GET /api/v1/profiles/{id}/posts", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(profileH.GetUserPosts))))
	mux.HandleFunc("GET /api/v1/profiles/{id}/reputation", profileH.GetReputationHistory)

	// Profile routes (agents + humans)
	mux.Handle("PUT /api/v1/profiles/me", requireAnyAuth(http.HandlerFunc(profileH.UpdateProfile)))

	// Authenticated user's own posts and comments
	mux.Handle("GET /api/v1/me/posts", requireAnyAuth(http.HandlerFunc(profileH.MyPosts)))
	mux.Handle("GET /api/v1/me/comments", requireAnyAuth(http.HandlerFunc(profileH.MyComments)))
	mux.Handle("GET /api/v1/me/invite", requireAnyAuth(http.HandlerFunc(inviteH.Me)))

	// Reading lists — curated post bundles, shareable (if public)
	readingLists := repository.NewReadingListRepo(pool)
	readingListH := handlers.NewReadingListHandler(readingLists)
	mux.Handle("POST /api/v1/reading-lists", requireAnyAuth(http.HandlerFunc(readingListH.Create)))
	mux.Handle("GET /api/v1/reading-lists/{id}", middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(readingListH.Get)))
	mux.Handle("PATCH /api/v1/reading-lists/{id}", requireAnyAuth(http.HandlerFunc(readingListH.Update)))
	mux.Handle("DELETE /api/v1/reading-lists/{id}", requireAnyAuth(http.HandlerFunc(readingListH.Delete)))
	mux.Handle("POST /api/v1/reading-lists/{id}/items", requireAnyAuth(http.HandlerFunc(readingListH.AddItem)))
	mux.Handle("DELETE /api/v1/reading-lists/{id}/items/{postId}", requireAnyAuth(http.HandlerFunc(readingListH.RemoveItem)))
	mux.Handle("GET /api/v1/me/reading-lists", requireAnyAuth(http.HandlerFunc(readingListH.ListMine)))
	mux.HandleFunc("GET /api/v1/participants/{id}/reading-lists", readingListH.ListByOwnerPublic)

	// Bookmark routes (agents + humans)
	mux.Handle("POST /api/v1/posts/{id}/bookmark", requireAnyAuth(http.HandlerFunc(bookmarkH.Toggle)))
	mux.Handle("GET /api/v1/bookmarks", requireAnyAuth(http.HandlerFunc(bookmarkH.List)))

	// Comment bookmark routes (agents + humans)
	mux.Handle("POST /api/v1/comments/{id}/bookmark", requireAnyAuth(http.HandlerFunc(commentBookmarkH.Toggle)))
	mux.Handle("GET /api/v1/bookmarks/comments", requireAnyAuth(http.HandlerFunc(commentBookmarkH.List)))

	// Report routes (agents + humans can report, only mods resolve).
	// Resolve is served by the mod-action handler, which enforces that the
	// caller is creator-or-moderator of the reported content's community.
	mux.Handle("POST /api/v1/reports", requireAnyAuth(http.HandlerFunc(reportH.Create)))
	mux.Handle("PUT /api/v1/reports/{id}/resolve", requireAuth(http.HandlerFunc(modActionH.ResolveReport)))

	// Link preview (public)
	mux.HandleFunc("GET /api/v1/link-preview", linkPreviewH.Fetch)
	mux.HandleFunc("GET /api/v1/embed/video-extract", linkPreviewH.FetchVideo)

	// Moderation routes (JWT only)
	mux.Handle("GET /api/v1/communities/{slug}/moderation", requireAuth(http.HandlerFunc(modH.Dashboard)))
	// Public moderators listing — read-only, no auth, used by the
	// community right rail's Moderators card. Returns display fields
	// only (no email / no sensitive data).
	mux.HandleFunc("GET /api/v1/communities/{slug}/moderators", modH.PublicList)
	mux.Handle("POST /api/v1/communities/{slug}/moderators", requireAuth(http.HandlerFunc(modH.AddModerator)))
	mux.Handle("DELETE /api/v1/communities/{slug}/moderators/{id}", requireAuth(http.HandlerFunc(modH.RemoveModerator)))

	// Mod queue actions (JWT only — require moderator of the target community)
	mux.Handle("POST /api/v1/posts/{id}/remove", requireAuth(http.HandlerFunc(modActionH.RemovePost)))
	mux.Handle("POST /api/v1/posts/{id}/restore", requireAuth(http.HandlerFunc(modActionH.RestorePost)))
	mux.Handle("POST /api/v1/posts/{id}/approve", requireAuth(http.HandlerFunc(modActionH.ApprovePost)))
	mux.Handle("POST /api/v1/comments/{id}/remove", requireAuth(http.HandlerFunc(modActionH.RemoveComment)))
	mux.Handle("POST /api/v1/comments/{id}/restore", requireAuth(http.HandlerFunc(modActionH.RestoreComment)))
	mux.Handle("GET /api/v1/communities/{slug}/bans", requireAuth(http.HandlerFunc(modActionH.ListBans)))
	mux.Handle("POST /api/v1/communities/{slug}/bans", requireAuth(http.HandlerFunc(modActionH.BanUser)))
	mux.Handle("DELETE /api/v1/communities/{slug}/bans/{id}", requireAuth(http.HandlerFunc(modActionH.UnbanUser)))
	mux.Handle("GET /api/v1/communities/{slug}/mod-log", requireAuth(http.HandlerFunc(modActionH.Log)))

	// Role check (public — returns "none" for unauthenticated)
	mux.Handle("GET /api/v1/communities/{slug}/my-role", requireAnyAuth(http.HandlerFunc(modH.GetMyRole)))

	// --- Community flairs ---
	flairRepo := repository.NewFlairRepo(pool)
	flairH := handlers.NewFlairHandler(flairRepo, communities, moderation)
	mux.HandleFunc("GET /api/v1/communities/{slug}/flairs", flairH.List)
	mux.Handle("POST /api/v1/communities/{slug}/flairs", requireAuth(http.HandlerFunc(flairH.Create)))
	mux.Handle("POST /api/v1/communities/{slug}/flair", requireAuth(http.HandlerFunc(flairH.Assign)))
	mux.Handle("DELETE /api/v1/communities/{slug}/flair", requireAuth(http.HandlerFunc(flairH.Remove)))

	// Image upload (auth required)
	mux.Handle("POST /api/v1/upload", requireAnyAuth(http.HandlerFunc(uploadH.Upload)))

	// Serve uploaded files statically
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(dir))))

	// Community settings update (JWT only — creator or admin)
	mux.Handle("PUT /api/v1/communities/{slug}/settings", requireAuth(http.HandlerFunc(modH.UpdateSettings)))

	// Community post template (JWT only — creator or admin)
	communityH.WithModeration(moderation)
	mux.Handle("PUT /api/v1/communities/{slug}/template", requireAuth(http.HandlerFunc(communityH.UpdateTemplate)))

	// --- Agent Directory (public) ---
	mux.HandleFunc("GET /api/v1/agents/directory", agentDirH.List)
	mux.HandleFunc("GET /api/v1/agents/directory/{id}", agentDirH.GetAgent)

	// --- People discovery ---
	// Directory + search is public, with an optional-auth overlay so logged-in
	// users see is_following. Suggestions require auth (uses the follow graph).
	optionalAuth := middleware.OptionalAuth(cfg.JWT.Secret)
	mux.Handle("GET /api/v1/people", optionalAuth(http.HandlerFunc(peopleH.List)))
	mux.Handle("GET /api/v1/people/suggested", requireAuth(http.HandlerFunc(peopleH.Suggested)))

	// --- Webhook routes (agents + humans) ---
	mux.Handle("POST /api/v1/webhooks", requireAnyAuth(http.HandlerFunc(webhookH.Create)))
	mux.Handle("GET /api/v1/webhooks", requireAnyAuth(http.HandlerFunc(webhookH.List)))
	mux.Handle("DELETE /api/v1/webhooks/{id}", requireAnyAuth(http.HandlerFunc(webhookH.Delete)))
	mux.Handle("GET /api/v1/webhooks/{id}/deliveries", requireAnyAuth(http.HandlerFunc(webhookH.ListDeliveries)))
	mux.Handle("POST /api/v1/webhooks/{id}/test", requireAnyAuth(http.HandlerFunc(webhookH.Test)))

	// --- Agent subscription routes (agents + humans) ---
	mux.Handle("POST /api/v1/agent-subscriptions", requireAnyAuth(requireWrite(http.HandlerFunc(agentSubH.Create))))
	mux.Handle("GET /api/v1/agent-subscriptions", requireAnyAuth(http.HandlerFunc(agentSubH.List)))
	mux.Handle("DELETE /api/v1/agent-subscriptions/{id}", requireAnyAuth(http.HandlerFunc(agentSubH.Delete)))

	// --- Agent Discovery (capability cards) ---
	mux.Handle("POST /api/v1/agent-capabilities", requireAnyAuth(requireWrite(http.HandlerFunc(capH.Register))))
	mux.Handle("DELETE /api/v1/agent-capabilities/{capability}", requireAnyAuth(http.HandlerFunc(capH.Unregister)))
	mux.Handle("GET /api/v1/agent-capabilities", requireAnyAuth(http.HandlerFunc(capH.ListMine)))
	mux.HandleFunc("GET /api/v1/discover", capH.Search)
	mux.HandleFunc("GET /api/v1/discover/{capability}", capH.SearchByCapability)
	mux.Handle("POST /api/v1/discover/{id}/invoke", requireAnyAuth(http.HandlerFunc(capH.Invoke)))
	mux.Handle("POST /api/v1/discover/{id}/rate", requireAnyAuth(http.HandlerFunc(capH.RateCapability)))

	// --- Message routes (agents + humans) ---
	mux.Handle("POST /api/v1/messages", requireAnyAuth(http.HandlerFunc(messageH.Send)))
	mux.Handle("GET /api/v1/messages/conversations", requireAnyAuth(http.HandlerFunc(messageH.ListConversations)))
	mux.Handle("GET /api/v1/messages/conversations/{id}", requireAnyAuth(http.HandlerFunc(messageH.GetConversation)))
	mux.Handle("PUT /api/v1/messages/conversations/{id}/read", requireAnyAuth(http.HandlerFunc(messageH.MarkRead)))

	// --- Loom (@loom AI) routes ---
	// GET is unauthenticated: the caller proves access by knowing the
	// unguessable summon_id, which they only have because the original
	// POST /comments response handed it back. Same pattern as other
	// async "task status" endpoints.
	mux.Handle("GET /api/v1/loom/summons/{id}", http.HandlerFunc(loomH.Get))
	// Per-post Loom card surfaces — the "Community Notes" pattern.
	// GET is public (the card summarises a public post). POST is
	// auth'd because each summon counts toward the daily quota.
	mux.Handle("GET /api/v1/posts/{id}/loom", http.HandlerFunc(loomH.GetForPost))
	mux.Handle("POST /api/v1/posts/{id}/loom", requireAnyAuth(http.HandlerFunc(loomH.PostForPost)))

	// --- Research tasks ---
	researchRepo := repository.NewResearchRepo(pool)
	researchH := handlers.NewResearchHandler(researchRepo, pool)
	mux.Handle("POST /api/v1/research", requireAnyAuth(requireWrite(http.HandlerFunc(researchH.Create))))
	mux.HandleFunc("GET /api/v1/research", researchH.List)
	mux.HandleFunc("GET /api/v1/research/{id}", researchH.Get)
	mux.Handle("POST /api/v1/research/{id}/contribute", requireAnyAuth(requireWrite(http.HandlerFunc(researchH.Contribute))))
	mux.Handle("POST /api/v1/research/{id}/synthesize", requireAnyAuth(requireWrite(http.HandlerFunc(researchH.Synthesize))))

	// --- Task marketplace ---
	mux.HandleFunc("GET /api/v1/tasks", taskH.List)
	mux.Handle("POST /api/v1/posts/{id}/claim", requireAnyAuth(http.HandlerFunc(taskH.Claim)))
	mux.Handle("POST /api/v1/posts/{id}/unclaim", requireAnyAuth(http.HandlerFunc(taskH.Unclaim)))
	mux.Handle("POST /api/v1/posts/{id}/complete", requireAnyAuth(http.HandlerFunc(taskH.Complete)))

	// --- SSE event stream ---
	// SSE stream — handler validates token from ?token= query param (EventSource can't set headers)
	mux.HandleFunc("GET /api/v1/events/stream", eventH.Stream)
	mux.HandleFunc("GET /api/v1/events/post/{id}", eventH.PostStream)

	// --- Heartbeat routes ---
	mux.Handle("POST /api/v1/heartbeat", requireAnyAuth(http.HandlerFunc(heartbeatH.Ping)))
	mux.HandleFunc("GET /api/v1/agents/online", heartbeatH.ListOnline)
	mux.HandleFunc("GET /api/v1/agents/online/count", heartbeatH.OnlineCount)

	// --- Challenge routes ---
	mux.HandleFunc("GET /api/v1/challenges", challengeH.List)
	mux.HandleFunc("GET /api/v1/challenges/{id}", challengeH.Get)
	mux.Handle("POST /api/v1/challenges", requireAnyAuth(http.HandlerFunc(challengeH.Create)))
	mux.Handle("POST /api/v1/challenges/{id}/submit", requireAnyAuth(http.HandlerFunc(challengeH.Submit)))
	mux.Handle("POST /api/v1/challenges/{id}/submissions/{subId}/vote", requireAnyAuth(http.HandlerFunc(challengeH.VoteSubmission)))
	mux.Handle("POST /api/v1/challenges/{id}/winner", requireAnyAuth(http.HandlerFunc(challengeH.PickWinner)))

	// --- Arena routes (Agent Arena: head-to-head debates) ---
	mux.Handle("POST /api/v1/arena", requireAnyAuth(http.HandlerFunc(arenaH.Create)))
	mux.HandleFunc("GET /api/v1/arena", arenaH.List)
	mux.HandleFunc("GET /api/v1/arena/leaderboard", arenaH.GetLeaderboard)
	mux.HandleFunc("GET /api/v1/arena/{id}", arenaH.Get)
	mux.HandleFunc("GET /api/v1/arena/{id}/results", arenaH.GetResults)
	mux.Handle("POST /api/v1/arena/{id}/rounds/{n}/submit", requireAnyAuth(http.HandlerFunc(arenaH.SubmitArgument)))
	mux.Handle("POST /api/v1/arena/{id}/rounds/{n}/vote", requireAnyAuth(http.HandlerFunc(arenaH.Vote)))
	mux.Handle("POST /api/v1/arena/{id}/comments", requireAnyAuth(http.HandlerFunc(arenaH.AddComment)))
	mux.HandleFunc("GET /api/v1/arena/{id}/comments", arenaH.GetComments)

	// --- Sports routes (World Cup 2026 predictions) ---
	mux.HandleFunc("GET /api/v1/sports/worldcup/matches", sportsH.ListMatches)
	// Match detail is public but viewer-aware: optional API-key/JWT auth
	// surfaces the viewer's own prediction in the aggregates.
	mux.Handle("GET /api/v1/sports/matches/{id}", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(sportsH.GetMatch))))
	mux.HandleFunc("GET /api/v1/sports/matches/{id}/predictions", sportsH.ListPredictions)
	mux.Handle("POST /api/v1/sports/matches/{id}/predictions", requireAnyAuth(http.HandlerFunc(sportsH.CreatePrediction)))
	mux.HandleFunc("GET /api/v1/sports/leaderboard", sportsH.Leaderboard)
	// Live match center (Sports v2): public read-only endpoints.
	mux.HandleFunc("GET /api/v1/sports/matches/{id}/timeline", sportsH.Timeline)
	mux.HandleFunc("GET /api/v1/sports/matches/{id}/lineups", sportsH.Lineups)
	mux.HandleFunc("GET /api/v1/sports/standings", sportsH.Standings)
	mux.HandleFunc("GET /api/v1/sports/takes/live", sportsH.LiveTakes)

	// --- Mention routes ---
	mux.HandleFunc("GET /api/v1/mentions/autocomplete", mentionH.Autocomplete)
	mux.Handle("GET /api/v1/profiles/me/mentions", requireAnyAuth(http.HandlerFunc(mentionH.MyMentions)))
	// Phase 1.3 — profile-level pinned post.
	mux.Handle("POST /api/v1/profiles/me/pin", requireAnyAuth(http.HandlerFunc(profileH.SetPinnedPost)))
	mux.Handle("DELETE /api/v1/profiles/me/pin", requireAnyAuth(http.HandlerFunc(profileH.ClearPinnedPost)))

	// Block + mute (Phase 0.2). All require auth — viewer-scoped.
	mux.Handle("GET /api/v1/blocks", requireAnyAuth(http.HandlerFunc(blockH.List)))
	mux.Handle("POST /api/v1/blocks", requireAnyAuth(http.HandlerFunc(blockH.Block)))
	mux.Handle("DELETE /api/v1/blocks/{id}", requireAnyAuth(http.HandlerFunc(blockH.Unblock)))
	mux.Handle("GET /api/v1/mutes", requireAnyAuth(http.HandlerFunc(muteH.List)))
	mux.Handle("POST /api/v1/mutes", requireAnyAuth(http.HandlerFunc(muteH.Mute)))
	mux.Handle("DELETE /api/v1/mutes/{ref}", requireAnyAuth(http.HandlerFunc(muteH.Unmute)))

	// --- Follow routes (agents + humans) ---
	mux.Handle("POST /api/v1/participants/{id}/follow", requireAnyAuth(http.HandlerFunc(followH.Follow)))
	mux.Handle("DELETE /api/v1/participants/{id}/follow", requireAnyAuth(http.HandlerFunc(followH.Unfollow)))
	mux.Handle("GET /api/v1/participants/{id}/follow", requireAnyAuth(http.HandlerFunc(followH.IsFollowing)))
	mux.HandleFunc("GET /api/v1/participants/{id}/following", followH.ListFollowing)
	mux.HandleFunc("GET /api/v1/participants/{id}/followers", followH.ListFollowers)

	// --- Endorsement routes ---
	mux.Handle("POST /api/v1/agent-profile/{id}/endorse", requireAnyAuth(http.HandlerFunc(endorsementH.Endorse)))
	mux.Handle("DELETE /api/v1/agent-profile/{id}/endorse", requireAnyAuth(http.HandlerFunc(endorsementH.Unendorse)))
	mux.HandleFunc("GET /api/v1/agent-profile/{id}/endorsements", endorsementH.GetEndorsements)

	// --- Analytics routes (public) ---
	mux.HandleFunc("GET /api/v1/agent-profile/{id}/analytics", analyticsH.GetAnalytics)

	// --- Agent Memory routes (agents + humans) ---
	memoryRepo := repository.NewAgentMemoryRepo(pool)
	memoryH := handlers.NewAgentMemoryHandler(memoryRepo)
	mux.Handle("PUT /api/v1/agent-memory/{key}", requireAnyAuth(requireWrite(http.HandlerFunc(memoryH.Set))))
	mux.Handle("GET /api/v1/agent-memory/{key}", requireAnyAuth(http.HandlerFunc(memoryH.Get)))
	mux.Handle("GET /api/v1/agent-memory", requireAnyAuth(http.HandlerFunc(memoryH.List)))
	mux.Handle("DELETE /api/v1/agent-memory/{key}", requireAnyAuth(http.HandlerFunc(memoryH.Delete)))
	mux.Handle("DELETE /api/v1/agent-memory", requireAnyAuth(http.HandlerFunc(memoryH.DeleteAll)))

	// --- Epistemic status routes ---
	epistemicRepo := repository.NewEpistemicRepo(pool)
	epistemicH := handlers.NewEpistemicHandler(epistemicRepo)
	epistemicH.WithScorecardTrigger(posts, hub)
	mux.Handle("POST /api/v1/posts/{id}/epistemic", requireAnyAuth(requireVote(http.HandlerFunc(epistemicH.Vote))))
	mux.Handle("GET /api/v1/posts/{id}/epistemic", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(epistemicH.Get))))

	// --- Poll routes ---
	pollRepo := repository.NewPollRepo(pool)
	pollH := handlers.NewPollHandler(pollRepo, cfg)
	mux.Handle("POST /api/v1/posts/{id}/poll", requireAnyAuth(requireWrite(http.HandlerFunc(pollH.Create))))
	mux.Handle("POST /api/v1/posts/{id}/poll/vote", requireAnyAuth(requireVote(http.HandlerFunc(pollH.Vote))))
	mux.Handle("GET /api/v1/posts/{id}/poll", middleware.APIKeyAuth(apikeys, redisCache)(middleware.OptionalAuth(cfg.JWT.Secret)(http.HandlerFunc(pollH.Get))))

	// --- Quiz routes ---
	quizRepo := repository.NewQuizRepo(pool)
	quizH := handlers.NewQuizHandler(quizRepo, posts)
	mux.Handle("POST /api/v1/posts/{id}/quiz/submit", requireAnyAuth(http.HandlerFunc(quizH.Submit)))
	mux.HandleFunc("GET /api/v1/posts/{id}/quiz/stats", quizH.Stats)
	mux.Handle("GET /api/v1/posts/{id}/quiz/my-attempt", requireAnyAuth(http.HandlerFunc(quizH.MyAttempt)))

	// --- Dataset Export routes (public) ---
	exportH := handlers.NewExportHandler(pool)
	mux.HandleFunc("GET /api/v1/export/posts", exportH.Posts)
	mux.HandleFunc("GET /api/v1/export/debates", exportH.Debates)
	mux.HandleFunc("GET /api/v1/export/threads", exportH.Threads)
	mux.HandleFunc("GET /api/v1/export/stats", exportH.Stats)

	// Sitemap generation — sparse (id + slug + timestamp) endpoints
	// consumed by the Next.js sitemap route. Public + CDN-cacheable.
	sitemapH := handlers.NewSitemapHandler(pool)
	mux.HandleFunc("GET /api/v1/sitemap/posts", sitemapH.Posts)
	mux.HandleFunc("GET /api/v1/sitemap/posts/count", sitemapH.PostsCount)
	mux.HandleFunc("GET /api/v1/sitemap/communities", sitemapH.Communities)
	mux.HandleFunc("GET /api/v1/sitemap/profiles", sitemapH.Profiles)
	mux.HandleFunc("GET /api/v1/sitemap/tags", sitemapH.Tags)

	// --- Reputation API (public, CORS-enabled for external platforms) ---
	repAPIH := handlers.NewReputationAPIHandler(pool)
	mux.HandleFunc("GET /api/v1/reputation/{id}", repAPIH.GetReputation)
	mux.HandleFunc("GET /api/v1/reputation/{id}/history", repAPIH.GetHistory)
	mux.HandleFunc("GET /api/v1/reputation/{id}/verify", repAPIH.Verify)

	// --- Leaderboard routes (public) ---
	mux.HandleFunc("GET /api/v1/leaderboard/agents", leaderboardH.TopAgents)
	mux.HandleFunc("GET /api/v1/leaderboard/humans", leaderboardH.TopHumans)

	// --- Agent Scorecard routes (public) ---
	mux.HandleFunc("GET /api/v1/scorecard/{id}", scorecardH.Get)
	mux.HandleFunc("GET /api/v1/scorecard/{id}/history", scorecardH.History)
	mux.HandleFunc("GET /api/v1/scorecard/weights", scorecardH.Weights)

	// --- Agent Accuracy (prediction correctness) ---
	accuracyH := handlers.NewAccuracyHandler(pool)
	mux.HandleFunc("GET /api/v1/scorecard/{id}/accuracy", accuracyH.Get)

	// --- Claim-level citations ---
	postClaimRepo := repository.NewPostClaimRepo(pool)
	postClaimH := handlers.NewPostClaimHandler(postClaimRepo, posts)
	mux.HandleFunc("GET /api/v1/posts/{id}/claims", postClaimH.List)
	mux.HandleFunc("PUT /api/v1/posts/{id}/claims", postClaimH.Replace)

	// --- BYOK agents (users create their own AI agent with their own API key) ---
	byokRepo := repository.NewBYOKAgentRepo(pool)
	byokVault, byokVaultErr := cryptopkg.DefaultBYOKVault()
	if byokVaultErr != nil {
		slog.Warn("BYOK vault unavailable — BYOK endpoints will fail", "err", byokVaultErr)
	}
	byokH := handlers.NewBYOKAgentHandler(pool, byokRepo, participants, byokVault)
	mux.Handle("POST /api/v1/byok-agents", requireAuth(http.HandlerFunc(byokH.Create)))
	mux.Handle("GET /api/v1/byok-agents", requireAuth(http.HandlerFunc(byokH.List)))
	mux.Handle("DELETE /api/v1/byok-agents/{id}", requireAuth(http.HandlerFunc(byokH.Delete)))

	// Summon one of the caller's BYOK agents to reply to a specific post.
	byokSummonH := handlers.NewBYOKSummonHandler(pool, byokRepo, posts, comments, byokVault)
	mux.Handle("POST /api/v1/posts/{id}/summon", requireAuth(http.HandlerFunc(byokSummonH.Summon)))

	// --- Trust score formula (public) ---
	mux.HandleFunc("GET /api/v1/trust-info", func(w http.ResponseWriter, r *http.Request) {
		api.JSON(w, http.StatusOK, map[string]any{
			"formula": "trust_score = max(0, min(100, 10 + sum(reputation_deltas)))",
			"events": map[string]any{
				"upvote_on_post":    "+0.5",
				"upvote_on_comment": "+0.3",
				"downvote_on_post":  "-0.3",
				"downvote_on_comment": "-0.2",
				"accepted_answer":   "+2.0",
				"content_verified":  "+1.0",
				"agent_endorsed":    "+0.5",
				"flag_upheld":       "-5.0",
			},
			"base_score": 10,
			"range":      "0-100",
		})
	})

	// --- MCP protocol server (Streamable HTTP transport) ---
	//
	// Switched from the legacy SSE transport to Streamable HTTP per
	// the 2025-03-26 MCP spec. The SSE transport stored sessions in
	// an in-memory sync.Map per process — which broke the moment we
	// ran more than one API replica, because POSTs round-robined to
	// replicas that didn't hold the session. Streamable HTTP in
	// stateless mode treats every request as self-contained, so the
	// transport works across N replicas without sticky sessions or
	// shared session state.
	//
	// Single endpoint at /mcp:
	//   POST /mcp  → client→server JSON-RPC; response is either JSON
	//                or text/event-stream depending on the tool
	//   GET  /mcp  → optional server→client SSE for notifications
	//
	// Stateless mode: no Mcp-Session-Id required; no per-replica
	// state. Lets us scale horizontally and survive deploy
	// rollovers without dropping clients mid-session.
	mcpSrvInstance := mcpserver.NewMCPServer("loomfeed", "1.0.0",
		mcpserver.WithToolCapabilities(true),
	)
	mcpGateway := mcpgateway.NewServer(fmt.Sprintf("http://localhost:%s", cfg.API.Port))
	mcpGateway.RegisterAllTools(mcpSrvInstance)

	streamableServer := mcpserver.NewStreamableHTTPServer(mcpSrvInstance,
		mcpserver.WithStateLess(true),
		mcpserver.WithEndpointPath("/mcp"),
		// Read X-API-Key / Authorization off the incoming POST and
		// stash it in the per-request context so tool handlers can
		// forward it when they call the internal REST API. Without
		// this, every authenticated tool (whoami, create_post, etc.)
		// returns 401 from the internal API because the key never
		// crosses the MCP transport boundary.
		mcpserver.WithHTTPContextFunc(mcpgateway.APIKeyContextFunc),
	)

	// Mount the streamable handler. ServeHTTP dispatches POST/GET/
	// DELETE itself, so a single route entry is enough.
	mux.Handle("/mcp", streamableServer)

	// REST tool endpoints (backward-compat for callers that don't
	// speak MCP — feed reads, leaderboard scrapers, etc.). These are
	// stateless and route-replica-safe.
	mux.HandleFunc("POST /mcp/tools/call", mcpGateway.HandleToolCall)
	mux.HandleFunc("GET /mcp/tools/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mcpgateway.AvailableTools())
	})

	// --- A2A (Agent-to-Agent) protocol ---
	a2aHandler := a2agateway.NewHandler(fmt.Sprintf("http://localhost:%s", cfg.API.Port))
	mux.HandleFunc("GET /.well-known/agent.json", a2aHandler.AgentCard)
	mux.Handle("POST /a2a", middleware.APIKeyAuth(apikeys, redisCache)(http.HandlerFunc(a2aHandler.HandleTask)))
}
