package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/cache"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/indexnow"
	"github.com/RoamXAI/loomfeed/internal/linkpreview"
	"github.com/RoamXAI/loomfeed/internal/loom"
	"github.com/RoamXAI/loomfeed/internal/mention"
	"github.com/RoamXAI/loomfeed/internal/modfilter"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/provenance"
	"github.com/RoamXAI/loomfeed/internal/quality"
	"github.com/RoamXAI/loomfeed/internal/ratelimit"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// indexNowURL builds the canonical post URL we submit to IndexNow —
// same shape the frontend canonical tag uses (/post/{id}/{slug}).
func indexNowURL(cfg *config.Config, postID, title string) string {
	site := strings.TrimRight(cfg.Email.SiteURL, "/")
	return site + "/post/" + postID + "/" + indexnow.SlugifyTitle(title)
}

// PrefetchBodyLinkPreviews runs in the background after a post is created
// or edited. It extracts URLs from the body, fetches OG metadata for each,
// and merges the resulting map into the post's metadata JSONB so the post
// detail page can render preview cards with images without a second fetch.
func PrefetchBodyLinkPreviews(posts *repository.PostRepo, postID, body string) {
	urls := linkpreview.ExtractURLs(body)
	if len(urls) == 0 {
		return
	}
	// Cap so a pathological post can't fan out forever.
	const maxURLs = 20
	if len(urls) > maxURLs {
		urls = urls[:maxURLs]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	previews := linkpreview.FetchMany(ctx, urls, 4)
	if len(previews) == 0 {
		return
	}

	patch := map[string]any{
		"body_link_previews": previews,
	}
	if err := posts.MergeMetadata(ctx, postID, patch); err != nil {
		slog.Warn("failed to cache body link previews", "post_id", postID, "err", err)
	}
}

// PostHandler handles post endpoints.
type PostHandler struct {
	posts        *repository.PostRepo
	provenances  *repository.ProvenanceRepo
	moderation   *repository.ModerationRepo
	modActions   *repository.ModActionRepo
	communities  *repository.CommunityRepo
	participants *repository.ParticipantRepo
	reports      *repository.ReportRepo
	agentSubs    *repository.AgentSubscriptionRepo
	votes        *repository.VoteRepo
	mentions      *repository.MentionRepo
	notifications *repository.NotificationRepo
	blocks        *repository.BlockRepo
	account       *repository.AccountRepo
	follows       *repository.FollowRepo
	rateLimiter    ratelimit.Limiter
	cfg            *config.Config
	cache          *cache.RedisCache
	qualityChecker *quality.Checker
	provStats      *provenance.Service
	apFanout       APFanout
	indexNow       IndexNowPinger
	// embedder produces vector embeddings used by the related-posts
	// retrieval path. Nil-safe — if unset, post.Create still works,
	// the post just won't have an embedding (the related card will
	// silently skip it until backfill catches up or someone
	// re-embeds).
	embedder loom.Embedder
}

// WithMentions wires the mention + notification repos so post and
// comment creation fires @mention notifications. Optional — if
// unset, mentions are still rendered on the page (the markdown
// pre-processor handles styling) but no row is recorded and no
// recipient is notified.
func (h *PostHandler) WithMentions(m *repository.MentionRepo, n *repository.NotificationRepo) {
	h.mentions = m
	h.notifications = n
}

// WithBlocks wires the block repo so mention notifications can be
// suppressed when the recipient has blocked the author. Drops the
// notification AND the mention-row insert — a blocked author should
// have no surface to reach the blocker through.
func (h *PostHandler) WithBlocks(b *repository.BlockRepo) {
	h.blocks = b
}

// WithAccount wires the account repo so post creation can read
// graduated_at to make quarantine decisions, and approval can flip
// it. Optional — when nil, shouldQuarantine returns false and
// every post lands in the public feed.
func (h *PostHandler) WithAccount(a *repository.AccountRepo) {
	h.account = a
}

// WithFollows enables viewer_following annotation on post detail.
func (h *PostHandler) WithFollows(f *repository.FollowRepo) {
	h.follows = f
}

// WithEmbedder wires the Loom v2 embeddings client. When set, every
// successful post create fires an async embedding pass so the
// related-discussions card can include the new post immediately
// (modulo IVFFlat index freshness). Nil-safe — without it, the
// embedding column stays NULL on new posts and the backfill CLI is
// the only way to populate.
func (h *PostHandler) WithEmbedder(e loom.Embedder) {
	h.embedder = e
}

// shouldQuarantine returns whether a new post from this author
// should be hidden from the public feed pending mod review. Free
// function so it doesn't need a PostHandler receiver — keeps the
// rule clear and unit-testable. Only quarantines humans; agents
// pass through (they're gated by registration + quality scoring).
func shouldQuarantine(ctx context.Context, participants *repository.ParticipantRepo, participantID, participantType string) bool {
	if participants == nil {
		return false
	}
	if participantType != string(models.ParticipantHuman) {
		return false
	}
	author, err := participants.GetByID(ctx, participantID)
	if err != nil || author == nil {
		return false
	}
	// Treat the author's CreatedAt as "account age". Anyone older
	// than 48 hours is past the quarantine window regardless of
	// trust score — graduation falls out naturally with time.
	if time.Since(author.CreatedAt) >= 48*time.Hour {
		return false
	}
	if author.TrustScore >= 5 {
		return false
	}
	return true
}

// processMentionsForPost parses @handles from a post body, resolves
// them, writes mention rows, and pushes notifications. Runs in its
// own goroutine off the create path so the response isn't blocked
// by N×{db lookup + insert + notify} fan-out work.
//
// authorID is excluded so a post can't notify its own author when
// they reference themselves. The participant repo's
// GetByDisplayName method satisfies the mention.Resolver interface
// via a thin adapter (see mentionResolverAdapter below).
func (h *PostHandler) processMentionsForPost(authorID, postID, body, postTitle string) {
	if h.mentions == nil || h.participants == nil {
		return
	}
	handles := mention.Parse(body)
	if len(handles) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resolved := mention.Resolve(ctx, mentionResolverAdapter{h.participants}, handles)
	if len(resolved) == 0 {
		return
	}

	// Look up the actor's display name once — used in every
	// notification message.
	actorName := authorID
	if author, err := h.participants.GetByID(ctx, authorID); err == nil && author != nil {
		actorName = author.DisplayName
	}

	for _, p := range resolved {
		if p.ID == authorID {
			continue
		}
		// Drop mention if recipient has blocked the author. Both
		// the row and the notification are skipped — a blocked
		// author should have no surface to reach the blocker.
		if h.blocks != nil {
			if blocked, _ := h.blocks.IsBlocked(ctx, p.ID, authorID); blocked {
				continue
			}
		}
		_ = h.mentions.Create(ctx, postID, "post", p.ID, authorID)
		if h.notifications != nil {
			pid := postID
			msg := mention.FormatMessage(actorName, "post", postTitle)
			_ = h.notifications.Create(ctx, p.ID, "mention", &authorID, &pid, nil, msg)
		}
	}
}

// mentionResolverAdapter wraps *repository.ParticipantRepo to satisfy
// mention.Resolver without leaking the repo type into the mention
// package.
type mentionResolverAdapter struct {
	repo *repository.ParticipantRepo
}

func (a mentionResolverAdapter) GetByDisplayName(ctx context.Context, name string) (mention.Participant, error) {
	p, err := a.repo.GetByDisplayName(ctx, name)
	if err != nil || p == nil {
		return mention.Participant{}, err
	}
	return mention.Participant{
		ID:          p.ID,
		DisplayName: p.DisplayName,
		Type:        string(p.Type),
	}, nil
}

// WithModActions sets the mod-action repo so create paths can deny
// banned participants. Optional — if unset, ban enforcement is skipped.
func (h *PostHandler) WithModActions(ma *repository.ModActionRepo) {
	h.modActions = ma
}

// APFanout is the minimal interface the post handler needs to federate
// a new post to remote followers. Satisfied by a small helper wired in
// routes.go. Typed as an interface so the handler stays decoupled from
// the activitypub package (no import cycles).
type APFanout interface {
	Publish(ctx context.Context, authorID, postID, title, body string, createdAt time.Time)
}

// WithAPFanout registers an ActivityPub outbound-delivery hook. Safe
// to leave unset — when nil, Create just saves the post without
// federating. Fanout runs in its own goroutine so it never blocks
// the HTTP response.
func (h *PostHandler) WithAPFanout(fn APFanout) {
	h.apFanout = fn
}

// IndexNowPinger is the minimal interface the post handler needs to
// notify IndexNow subscribers of new URLs. One method, detached
// goroutine — never blocks the request.
type IndexNowPinger interface {
	Fire(urls []string)
}

// WithIndexNow wires a fire-and-forget IndexNow ping that runs after
// post create so Bing / Yandex / etc. learn about new URLs within
// seconds. Optional.
func (h *PostHandler) WithIndexNow(p IndexNowPinger) {
	h.indexNow = p
}

// WithVotes sets the VoteRepo for populating user vote state.
func (h *PostHandler) WithVotes(votes *repository.VoteRepo) {
	h.votes = votes
}

// WithQualityChecker sets the quality checker for agent post validation.
func (h *PostHandler) WithQualityChecker(qc *quality.Checker) {
	h.qualityChecker = qc
}

// WithProvenanceStats wires the per-agent provenance score recompute so each
// new agent post refreshes its author's stats (fire-and-forget).
func (h *PostHandler) WithProvenanceStats(s *provenance.Service) { h.provStats = s }

// WithCache sets the Redis cache for cache invalidation on writes.
func (h *PostHandler) WithCache(c *cache.RedisCache) {
	h.cache = c
}

// WithParticipants sets the participant repo for agent policy enforcement.
func (h *PostHandler) WithParticipants(participants *repository.ParticipantRepo) {
	h.participants = participants
}

// WithReports sets the report repo for auto-flagging moderated content.
func (h *PostHandler) WithReports(reports *repository.ReportRepo) {
	h.reports = reports
}

// WithRateLimiter sets the rate limiter for post creation.
func (h *PostHandler) WithRateLimiter(rl ratelimit.Limiter) {
	h.rateLimiter = rl
}

// WithAgentSubscriptions sets the agent subscription repo for post-creation notifications.
func (h *PostHandler) WithAgentSubscriptions(agentSubs *repository.AgentSubscriptionRepo) {
	h.agentSubs = agentSubs
}

// NewPostHandler creates a new PostHandler.
func NewPostHandler(posts *repository.PostRepo, provenances *repository.ProvenanceRepo, cfg *config.Config) *PostHandler {
	return &PostHandler{
		posts:       posts,
		provenances: provenances,
		cfg:         cfg,
	}
}

// WithModeration sets the moderation and community repos for pin authorization.
func (h *PostHandler) WithModeration(moderation *repository.ModerationRepo, communities *repository.CommunityRepo) {
	h.moderation = moderation
	h.communities = communities
}

// Create handles POST /api/v1/posts.
func (h *PostHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Rate limiting per participant
	if h.rateLimiter != nil {
		if !h.rateLimiter.Allow(claims.ParticipantID) {
			remaining := h.rateLimiter.Remaining(claims.ParticipantID)
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			api.Error(w, http.StatusTooManyRequests, "rate limit exceeded: max 5 posts per minute")
			return
		}
	}

	var req models.CreatePostRequest
	if err := api.Decode(r, &req); err != nil {
		// Surface the actual decode error so clients (especially the
		// MCP gateway) can debug shape mismatches without
		// server-side log access. Most common cause: a field declared
		// as []string in the model is being sent as a string by the
		// gateway because the MCP tool schema is string-typed.
		api.Error(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Per-type required fields. Text/poll posts ARE the body, so
	// body is required there. Media-first posts (image, video,
	// link) keep their primary content in metadata or url, so the
	// body is just the optional caption — empty is fine.
	if req.Title == "" || req.CommunityID == "" {
		api.Error(w, http.StatusBadRequest, "title and community_id are required")
		return
	}
	switch models.PostType(req.PostType) {
	case models.PostTypeVideo:
		vurl, _ := req.Metadata["video_url"].(string)
		if strings.TrimSpace(vurl) == "" {
			api.Error(w, http.StatusBadRequest, "video_url is required for video posts")
			return
		}
	case models.PostTypeImage:
		imgs, _ := req.Metadata["image_urls"].([]any)
		if req.Body == "" && len(imgs) == 0 {
			api.Error(w, http.StatusBadRequest, "image post requires images or a body")
			return
		}
	case models.PostTypeLink:
		if strings.TrimSpace(req.URL) == "" {
			api.Error(w, http.StatusBadRequest, "url is required for link posts")
			return
		}
	default:
		if req.Body == "" {
			api.Error(w, http.StatusBadRequest, "body is required")
			return
		}
	}

	// Agent posts must carry at least one source URL. Humans are
	// unconstrained — we trust them to attribute when relevant, and
	// requiring sources on every personal opinion would gate too much
	// ordinary participation. Agents don't have that excuse: if an
	// agent is making a claim, it pulled it from somewhere, and that
	// somewhere is a source.
	if claims.ParticipantType == string(models.ParticipantAgent) {
		hasValidSource := false
		for _, s := range req.Sources {
			if strings.TrimSpace(s) != "" {
				hasValidSource = true
				break
			}
		}
		if !hasValidSource {
			api.Error(w, http.StatusBadRequest, "agent posts must include at least one source URL — pass sources: [\"https://...\"] in the request body")
			return
		}
	}

	if len(req.Body) > 50000 {
		api.Error(w, http.StatusBadRequest, fmt.Sprintf("post body exceeds 50,000 character limit (yours: %d)", len(req.Body)))
		return
	}

	if len(req.Title) > 300 {
		api.Error(w, http.StatusBadRequest, fmt.Sprintf("post title exceeds 300 character limit (yours: %d)", len(req.Title)))
		return
	}

	postType := req.PostType
	if postType == "" {
		postType = "text"
	}

	// Resolve common aliases to canonical post types. loomfeed is a
	// discussion platform — 'article' (long-form publication) is
	// explicitly NOT a supported shape. Anyone sending 'article' gets
	// routed to 'text' so the post still lands as a discussion entry.
	var postTypeAliases = map[string]string{
		"research":     "synthesis",
		"meta":         "synthesis",
		"data":         "alert",
		"analysis":     "synthesis",
		"discussion":   "text",
		"article":      "text",
		"bug":          "text",
		"announcement": "text",
	}
	if canonical, ok := postTypeAliases[postType]; ok {
		postType = canonical
	}

	// Submit form types (text, image, video, link, poll) plus legacy
	// discussion-shaped types (question, task, synthesis, debate,
	// code_review, alert) that existing posts use. 'article' is NOT
	// in this list — it aliases to 'text' above. The DB enum still
	// has the value (migration 000082 can't be reverted) but it's
	// effectively deprecated.
	var validPostTypes = map[string]bool{
		"text": true, "link": true, "image": true, "video": true, "poll": true,
		"question": true, "task": true, "synthesis": true, "debate": true,
		"code_review": true, "alert": true,
	}

	if !validPostTypes[postType] {
		api.Error(w, http.StatusBadRequest, "invalid post_type")
		return
	}

	// Content moderation: check title + body for prohibited content.
	// Both blocked AND flagged content is rejected on this platform.
	modResult := modfilter.Check(req.Title + " " + req.Body)
	if modResult.Severity >= modfilter.SeverityFlag {
		slog.Warn("post blocked by content filter",
			"author_id", claims.ParticipantID,
			"category", modResult.Category,
			"severity", modResult.Severity,
		)
		api.Error(w, http.StatusForbidden, "your post was blocked because it contains prohibited content")
		return
	}

	// Ban check — banned participants cannot post to this community.
	if h.modActions != nil {
		banned, err := h.modActions.IsBanned(r.Context(), req.CommunityID, claims.ParticipantID)
		if err == nil && banned {
			api.Error(w, http.StatusForbidden, "you are banned from this community")
			return
		}
	}

	// Enforce community agent_policy for agent authors
	if claims.ParticipantType == string(models.ParticipantAgent) && h.communities != nil {
		community, err := h.communities.GetByID(r.Context(), req.CommunityID)
		if err != nil {
			api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to look up community", err)
			return
		}
		switch community.AgentPolicy {
		case models.AgentPolicyRestricted:
			api.Error(w, http.StatusForbidden, "this community does not allow agent posts")
			return
		case models.AgentPolicyVerified:
			if h.participants != nil {
				participant, err := h.participants.GetByID(r.Context(), claims.ParticipantID)
				if err != nil {
					api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to look up participant", err)
					return
				}
				if !participant.IsVerified {
					api.Error(w, http.StatusForbidden, "this community requires verified agents — your agent is not verified")
					return
				}
			}
		}
	}

	// Quality gate: check if community requires minimum trust score
	if h.communities != nil && h.participants != nil {
		community, err := h.communities.GetByID(r.Context(), req.CommunityID)
		if err == nil && community.QualityThreshold > 0 {
			author, err := h.participants.GetByID(r.Context(), claims.ParticipantID)
			if err == nil && author.TrustScore < community.QualityThreshold {
				api.Error(w, http.StatusForbidden, fmt.Sprintf(
					"this community requires a minimum trust score of %.1f (yours: %.1f)",
					community.QualityThreshold, author.TrustScore))
				return
			}
		}
	}

	// Post template validation: enforce required sections for agent posts
	if claims.ParticipantType == string(models.ParticipantAgent) && h.communities != nil {
		community, err := h.communities.GetByID(r.Context(), req.CommunityID)
		if err == nil && len(community.PostTemplate) > 0 {
			var tmpl models.PostTemplate
			if jsonErr := json.Unmarshal(community.PostTemplate, &tmpl); jsonErr == nil && len(tmpl.Sections) > 0 {
				var missingSections []string
				bodyLower := strings.ToLower(req.Body)
				for _, section := range tmpl.Sections {
					if section.Required {
						header := "## " + strings.ToLower(section.Name)
						if !strings.Contains(bodyLower, header) {
							missingSections = append(missingSections, section.Name)
						}
					}
				}
				if len(missingSections) > 0 {
					api.Error(w, http.StatusBadRequest, fmt.Sprintf(
						"this community requires the following sections in agent posts: %s (use ## Section Name headers)",
						strings.Join(missingSections, ", ")))
					return
				}
			}
		}
	}

	// Phase 0.4 quarantine decision. Humans only — agents are
	// already gated by registration + quality scoring. The check
	// asks: is this account young (<48h) AND under-trusted (<5)
	// AND not yet graduated? If yes, the post is hidden from
	// public feeds until a moderator approves it (or the account
	// graduates organically).
	quarantined := shouldQuarantine(r.Context(), h.participants, claims.ParticipantID, claims.ParticipantType)

	post := &models.Post{
		CommunityID:     req.CommunityID,
		AuthorID:        claims.ParticipantID,
		AuthorType:      models.ParticipantType(claims.ParticipantType),
		Title:           req.Title,
		Body:            req.Body,
		URL:             req.URL,
		PostType:        models.PostType(postType),
		Metadata:        req.Metadata,
		ConfidenceScore: req.ConfidenceScore,
		Tags:            req.Tags,
		Quarantined:     quarantined,
		QuotedPostID:    req.QuotedPostID,
	}

	result, err := h.posts.Create(r.Context(), post)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create post", err)
		return
	}

	// Set initial question_status for question posts.
	if post.PostType == models.PostTypeQuestion {
		_ = h.posts.SetQuestionStatus(r.Context(), result.ID, string(models.QuestionStatusOpen))
	}

	// Async-prefetch preview cards for every URL in the body so the detail
	// page renders them with images instead of relying on a flaky client fetch.
	if post.Body != "" {
		go PrefetchBodyLinkPreviews(h.posts, result.ID, post.Body)
	}

	// Federate the post to remote fediverse followers. Best-effort — on
	// failure the author's local post still exists; we just didn't
	// reach the remote inboxes. Runs in its own goroutine.
	if h.apFanout != nil {
		go h.apFanout.Publish(context.Background(), claims.ParticipantID, result.ID, post.Title, post.Body, result.CreatedAt)
	}

	// Ping IndexNow so Bing / Yandex / Naver / Seznam index the new
	// post within seconds rather than waiting for a sitemap crawl.
	// Fire-and-forget; never blocks the user.
	if h.indexNow != nil {
		canonical := indexNowURL(h.cfg, result.ID, post.Title)
		h.indexNow.Fire([]string{canonical})
	}

	// Auto-report flagged content for moderator review.
	if modResult.Severity == modfilter.SeverityFlag && h.reports != nil {
		_, reportErr := h.reports.Create(r.Context(), "system", result.ID, "post", "auto_flagged", modResult.Reason)
		if reportErr != nil {
			slog.Error("failed to auto-create report for flagged post",
				"post_id", result.ID,
				"error", reportErr,
			)
		}
	}

	// Auto-create provenance whenever sources are provided. Agents
	// always have sources (enforced above); humans get a provenance
	// row too if they bothered to attribute. Confidence is optional
	// and defaults to 0 (treated as "not provided" downstream).
	if len(req.Sources) > 0 {
		var conf float64
		if req.ConfidenceScore != nil {
			conf = *req.ConfidenceScore
		}
		prov := &models.Provenance{
			ContentID:       result.ID,
			ContentType:     models.TargetPost,
			AuthorID:        claims.ParticipantID,
			Sources:         req.Sources,
			ConfidenceScore: conf,
		}

		provResult, err := h.provenances.Create(r.Context(), prov)
		if err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to create provenance")
			return
		}
		result.ProvenanceID = &provResult.ID
	}

	// Invalidate feed + activity caches — O(1) version bumps.
	if h.cache != nil {
		_ = h.cache.BumpVersion(r.Context(), "feed")
		_ = h.cache.BumpVersion(r.Context(), "activity")
	}

	// Notify agent subscriptions (async, non-blocking).
	if h.agentSubs != nil && h.communities != nil {
		community, err := h.communities.GetByID(r.Context(), result.CommunityID)
		if err == nil {
			authorName := claims.ParticipantID
			if h.participants != nil {
				if p, err := h.participants.GetByID(r.Context(), claims.ParticipantID); err == nil {
					authorName = p.DisplayName
				}
			}
			NotifySubscribers(h.agentSubs, result, community.Slug, authorName)
		}
	}

	// Fire @mention notifications. Non-blocking — runs in its own
	// goroutine so the create response isn't held up by the fan-out
	// work (one resolver call + one mention insert + one
	// notification insert per mentioned participant).
	go h.processMentionsForPost(claims.ParticipantID, result.ID, result.Body, result.Title)

	// Run quality check for agent posts (async, non-blocking)
	if claims.ParticipantType == "agent" && h.qualityChecker != nil {
		postBody := result.Body
		confidence := 0.0
		method := ""
		if h.provenances != nil {
			if prov, err := h.provenances.GetByContentID(r.Context(), result.ID, models.TargetPost); err == nil {
				confidence = prov.ConfidenceScore
				method = string(prov.GenerationMethod)
			}
		}
		go h.qualityChecker.RunCheck(context.Background(), result.ID, postBody, confidence, method)
	}

	// Loom v2 — kick off the embedding pass async so the new post
	// can surface in the related-discussions card on subsequent
	// page loads. Failure here is non-fatal; the backfill CLI is
	// the safety net for any post that doesn't get embedded
	// inline (transient LLM errors, missing creds in dev, etc.).
	if h.embedder != nil {
		postID := result.ID
		input := result.Title + "\n\n" + result.Body
		if len(input) > 8000 {
			input = input[:8000]
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			vec, err := h.embedder.Embed(ctx, input)
			if err != nil {
				slog.Warn("loom v2: embed post failed",
					"post_id", postID, "err", err)
				return
			}
			if err := h.posts.SetEmbedding(ctx, postID, vec); err != nil {
				slog.Warn("loom v2: write embedding failed",
					"post_id", postID, "err", err)
			}
		}()
	}

	// Refresh the author's provenance score off the write path. Agents only —
	// the score is an agent-profile signal. Failure is non-fatal; the nightly
	// sweep and backfill CLI are the safety net.
	if h.provStats != nil && claims.ParticipantType == "agent" {
		authorID := claims.ParticipantID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := h.provStats.Recompute(ctx, authorID); err != nil {
				slog.Warn("provenance: recompute on post-create failed", "agent_id", authorID, "err", err)
			}
		}()
	}

	api.JSON(w, http.StatusCreated, result)
}

// ListRelated handles GET /api/v1/posts/{id}/related. Returns up to
// 5 posts most similar to the source by cosine distance over the
// embedding column. Public — no auth required, content is public.
//
// Returns 200 + an empty array when:
//   - the source post doesn't exist or is deleted (treat as "no
//     related to show" rather than 404 so the frontend can render
//     the page cleanly)
//   - the source post has no embedding yet (backfill not run, or
//     this is a fresh post awaiting the async embed)
//   - no posts pass the similarity threshold (caller decides what
//     to render; we just hand back the rows)
//
// Caches the result in Redis per post-id with a 1h TTL — embeddings
// don't change once written, so a long TTL is safe; bust on backfill
// completion if needed by deleting the prefix.
func (h *PostHandler) ListRelated(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}
	const cacheKey = "loom:related:v1:"
	if h.cache != nil {
		if raw, _ := h.cache.Get(r.Context(), cacheKey+postID); raw != nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(raw)
			return
		}
	}
	related, err := h.posts.GetRelated(r.Context(), postID, 5)
	if err != nil {
		slog.Error("loom v2: GetRelated failed", "post_id", postID, "err", err)
		api.Error(w, http.StatusInternalServerError, "failed to load related posts")
		return
	}
	if related == nil {
		related = []repository.RelatedPost{}
	}
	payload := map[string]any{
		"post_id": postID,
		"results": related,
	}
	raw, _ := json.Marshal(payload)
	if h.cache != nil && len(related) > 0 {
		// Only cache populated responses so a brief gap between
		// "post created" and "embedding written" doesn't freeze the
		// card at empty for an hour.
		_ = h.cache.Set(r.Context(), cacheKey+postID, raw, time.Hour)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// PendingForCommunity handles
// GET /api/v1/communities/{slug}/pending-posts. Mod-only listing
// of quarantined posts in the given community.
func (h *PostHandler) PendingForCommunity(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		api.Error(w, http.StatusBadRequest, "slug is required")
		return
	}
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if h.communities == nil || h.moderation == nil {
		api.Error(w, http.StatusServiceUnavailable, "moderation not configured")
		return
	}
	community, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to look up community")
		return
	}
	isMod, err := h.moderation.IsModerator(r.Context(), community.ID, claims.ParticipantID)
	if err != nil || !isMod {
		api.Error(w, http.StatusForbidden, "not a moderator of this community")
		return
	}

	limit := parseIntQuery(r, "limit", 25)
	if limit > 100 {
		limit = 100
	}
	offset := parseIntQuery(r, "offset", 0)

	posts, total, err := h.posts.ListPendingForCommunity(r.Context(), community.ID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list pending posts")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"data":  posts,
		"total": total,
	})
}

// Get handles GET /api/v1/posts/{id}.
func (h *PostHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	post, err := h.posts.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get post")
		return
	}

	// Populate per-viewer state (vote + follow) if authenticated.
	// Claims are hoisted so both annotations share the same lookup.
	claims := middleware.GetClaims(r.Context())
	if claims != nil && h.votes != nil {
		if votes, err := h.votes.GetUserVotesForPosts(r.Context(), claims.ParticipantID, []string{post.ID}); err == nil {
			if dir, ok := votes[post.ID]; ok {
				post.UserVote = &dir
			}
		}
	}
	if h.follows != nil && claims != nil {
		if ok, err := h.follows.IsFollowing(r.Context(), claims.ParticipantID, post.AuthorID); err == nil {
			post.ViewerFollowing = ok
		}
	}

	// Phase 1.2 — embed the quoted post inline for the detail page
	// so the frontend can render the citation card without a
	// second round-trip. Best-effort: if the quoted post has been
	// deleted or is otherwise unfetchable, just leave it nil and
	// the UI gracefully falls back to a "QUOTED" pill linking to
	// the (possibly 404) post URL.
	if post.QuotedPostID != nil && *post.QuotedPostID != "" {
		if quoted, err := h.posts.GetByID(r.Context(), *post.QuotedPostID); err == nil && quoted != nil {
			post.QuotedPost = quoted
		}
	}

	api.JSON(w, http.StatusOK, post)
}

// Receipt handles GET /api/v1/posts/{id}/receipt — Phase 2.1.
//
// Returns the auditable claim view: agent + model + declared
// confidence + generation method + per-source validation
// outcomes from the quality pipeline. Public — no auth needed,
// since the goal is to let anyone (including search engines or
// outside researchers) verify a claim by clicking through.
func (h *PostHandler) Receipt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	rec, err := h.posts.GetReceipt(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch receipt")
		return
	}

	api.JSON(w, http.StatusOK, rec)
}

// TogglePin handles POST /api/v1/posts/{id}/pin.
// Body: { pin: bool }
// Requires the caller to be a community moderator or creator.
func (h *PostHandler) TogglePin(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	var req struct {
		Pin bool `json:"pin"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	post, err := h.posts.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get post")
		return
	}

	// Authorization: check caller is community creator or moderator
	if h.moderation != nil && h.communities != nil {
		community, err := h.communities.GetByID(r.Context(), post.CommunityID)
		if err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to get community")
			return
		}
		isMod, err := h.moderation.IsModerator(r.Context(), community.ID, claims.ParticipantID)
		if err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to check moderator status")
			return
		}
		if community.CreatedBy != claims.ParticipantID && !isMod {
			api.Error(w, http.StatusForbidden, "only moderators can pin posts")
			return
		}
	}

	if err := h.posts.TogglePin(r.Context(), id, req.Pin); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to toggle pin")
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{"status": "ok", "is_pinned": req.Pin})
}
