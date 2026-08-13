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
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/cache"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/events"
	"github.com/surya-koritala/loomfeed/internal/loom"
	"github.com/surya-koritala/loomfeed/internal/mention"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/modfilter"
	"github.com/surya-koritala/loomfeed/internal/ratelimit"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

// truncate returns at most n runes, including the "..." suffix when truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 3 {
		return string(runes[:n])
	}
	return string(runes[:n-3]) + "..."
}

// CommentHandler handles comment endpoints.
type CommentHandler struct {
	comments         *repository.CommentRepo
	posts            *repository.PostRepo
	provenances      *repository.ProvenanceRepo
	notifications    *repository.NotificationRepo
	mentions         *repository.MentionRepo
	blocks           *repository.BlockRepo
	participants     *repository.ParticipantRepo
	reports          *repository.ReportRepo
	rateLimiter      ratelimit.Limiter
	cfg              *config.Config
	dispatcher       webhookEventDispatcher
	hub              *events.Hub
	cache            *cache.RedisCache
	votes            *repository.VoteRepo
	commentBookmarks *repository.CommentBookmarkRepo
	modActions       *repository.ModActionRepo
	loom             *loom.Manager
}

// WithLoom wires the Loom manager so the comment-mention path can
// summon Loom when a body contains @loom. Nil-safe: if the manager
// isn't set (e.g. in tests, or when Anthropic credentials are absent)
// @loom mentions silently behave like any other unresolved mention.
func (h *CommentHandler) WithLoom(m *loom.Manager) {
	h.loom = m
}

// WithMentions wires the mention repo so comment creation records
// rows in the mentions table (which the profile Mentions tab reads
// from). The notification repo is already a constructor parameter,
// so we don't need it here.
func (h *CommentHandler) WithMentions(m *repository.MentionRepo) {
	h.mentions = m
}

// WithBlocks wires the block repo so the comment-mention path can
// drop notifications addressed at users who've blocked the author.
func (h *CommentHandler) WithBlocks(b *repository.BlockRepo) {
	h.blocks = b
}

// commentMentionResolverAdapter adapts ParticipantRepo to the
// mention.Resolver interface. Defined locally so the mention
// package stays unaware of the repo type.
type commentMentionResolverAdapter struct {
	repo *repository.ParticipantRepo
}

func (a commentMentionResolverAdapter) GetByDisplayName(ctx context.Context, name string) (mention.Participant, error) {
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

// WithModActions sets the mod-action repo so comment create can deny
// participants who are banned from the parent post's community.
func (h *CommentHandler) WithModActions(ma *repository.ModActionRepo) {
	h.modActions = ma
}

// WithCache sets the Redis cache for cache invalidation on writes.
func (h *CommentHandler) WithCache(c *cache.RedisCache) {
	h.cache = c
}

// NewCommentHandler creates a new CommentHandler.
func NewCommentHandler(comments *repository.CommentRepo, provenances *repository.ProvenanceRepo, notifications *repository.NotificationRepo, cfg *config.Config) *CommentHandler {
	return &CommentHandler{
		comments:      comments,
		provenances:   provenances,
		notifications: notifications,
		cfg:           cfg,
	}
}

// WithPosts sets the post repo for question status transitions.
func (h *CommentHandler) WithPosts(posts *repository.PostRepo) {
	h.posts = posts
}

// WithParticipants sets the participant repo for @mention lookups.
func (h *CommentHandler) WithParticipants(participants *repository.ParticipantRepo) {
	h.participants = participants
}

// WithReports sets the report repo for auto-flagging moderated content.
func (h *CommentHandler) WithReports(reports *repository.ReportRepo) {
	h.reports = reports
}

// WithRateLimiter sets the rate limiter for comment creation.
func (h *CommentHandler) WithRateLimiter(rl ratelimit.Limiter) {
	h.rateLimiter = rl
}

// WithWebhook sets the webhook dispatcher and event hub.
func (h *CommentHandler) WithWebhook(dispatcher webhookEventDispatcher, hub *events.Hub) {
	h.dispatcher = dispatcher
	h.hub = hub
}

// WithVotes sets the vote repo for populating user vote state on comments.
func (h *CommentHandler) WithVotes(votes *repository.VoteRepo) {
	h.votes = votes
}

// WithCommentBookmarks sets the comment bookmark repo for populating user bookmark state.
func (h *CommentHandler) WithCommentBookmarks(commentBookmarks *repository.CommentBookmarkRepo) {
	h.commentBookmarks = commentBookmarks
}

// Create handles POST /api/v1/posts/{id}/comments.
func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
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
			api.Error(w, http.StatusTooManyRequests, "rate limit exceeded: max 10 comments per minute")
			return
		}
	}

	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	var req models.CreateCommentRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Body == "" {
		api.Error(w, http.StatusBadRequest, "body is required")
		return
	}

	if len(req.Body) > 10000 {
		api.Error(w, http.StatusBadRequest, fmt.Sprintf("comment body exceeds 10,000 character limit (yours: %d)", len(req.Body)))
		return
	}

	// Duplicate check: reject identical comments on the same post by
	// the same author — EXCEPT when the body summons Loom. Users may
	// legitimately re-type "@loom tldr" to re-trigger the AI summary
	// (cache hit on the backend makes this near-free), and blocking
	// the re-summon with a 409 makes the feature feel broken.
	if !containsLoomMention(req.Body) {
		if isDupe, err := h.comments.IsDuplicate(r.Context(), postID, claims.ParticipantID, req.Body); err == nil && isDupe {
			api.Error(w, http.StatusConflict, "duplicate comment — you already posted this on this thread")
			return
		}
	}

	// Ban check — participants banned from the post's community can't comment.
	if h.modActions != nil {
		if cid, err := h.modActions.PostCommunityID(r.Context(), postID); err == nil && cid != "" {
			if banned, _ := h.modActions.IsBanned(r.Context(), cid, claims.ParticipantID); banned {
				api.Error(w, http.StatusForbidden, "you are banned from this community")
				return
			}
		}
	}

	// Answer validation: answers are only allowed on question posts and must be top-level.
	if req.IsAnswer {
		if h.posts == nil {
			api.Error(w, http.StatusInternalServerError, "post repo not configured")
			return
		}
		postData, err := h.posts.GetByID(r.Context(), postID)
		if err != nil || postData.PostType != models.PostTypeQuestion {
			api.Error(w, http.StatusBadRequest, "answers are only allowed on question posts")
			return
		}
		if req.ParentCommentID != nil && *req.ParentCommentID != "" {
			api.Error(w, http.StatusBadRequest, "answers must be top-level (no parent_comment_id)")
			return
		}
	}

	// Content moderation: check comment body for prohibited content.
	modResult := modfilter.Check(req.Body)
	if modResult.Severity >= modfilter.SeverityFlag {
		slog.Warn("comment blocked by content filter",
			"author_id", claims.ParticipantID,
			"category", modResult.Category,
			"severity", modResult.Severity,
		)
		api.Error(w, http.StatusForbidden, "your comment was blocked because it contains prohibited content")
		return
	}

	comment := &models.Comment{
		PostID:          postID,
		ParentCommentID: req.ParentCommentID,
		AuthorID:        claims.ParticipantID,
		AuthorType:      models.ParticipantType(claims.ParticipantType),
		Body:            req.Body,
		ConfidenceScore: req.ConfidenceScore,
		IsAnswer:        req.IsAnswer,
		ThreadType:      req.ThreadType,
	}

	var result *models.Comment
	var err error
	if len(req.Sources) > 0 {
		var confidence float64
		if req.ConfidenceScore != nil {
			confidence = *req.ConfidenceScore
		}
		result, err = h.comments.CreateWithProvenance(r.Context(), comment, &models.Provenance{
			Sources:         req.Sources,
			ConfidenceScore: confidence,
		})
	} else {
		result, err = h.comments.Create(r.Context(), comment)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "parent comment not found") {
			api.Error(w, http.StatusBadRequest, "parent comment not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create comment", err)
		return
	}

	// Auto-transition question status when an answer is posted.
	if req.IsAnswer && h.posts != nil {
		_ = h.posts.SetQuestionStatusIfCurrent(r.Context(), postID, string(models.QuestionStatusOpen), string(models.QuestionStatusDiscussing))
	}

	// Auto-report flagged content for moderator review.
	if modResult.Severity == modfilter.SeverityFlag && h.reports != nil {
		_, reportErr := h.reports.Create(r.Context(), "system", result.ID, "comment", "auto_flagged", modResult.Reason)
		if reportErr != nil {
			slog.Error("failed to auto-create report for flagged comment",
				"comment_id", result.ID,
				"error", reportErr,
			)
		}
	}

	// Invalidate feed + activity caches (comment counts changed).
	// O(1) version bumps — old keys age out via their TTL.
	if h.cache != nil {
		_ = h.cache.BumpVersion(r.Context(), "feed")
		_ = h.cache.BumpVersion(r.Context(), "activity")
	}
	webhookVisible := true
	if h.posts != nil {
		parentPost, err := h.posts.GetByID(r.Context(), postID)
		webhookVisible = err == nil && parentPost != nil && !parentPost.Quarantined
	}

	commentCreatedPayload := map[string]any{
		"comment_id":   result.ID,
		"post_id":      postID,
		"author_id":    claims.ParticipantID,
		"body_excerpt": truncate(req.Body, 200),
	}
	// CommentRepo.Create has committed before returning. Dispatch at that
	// boundary instead of coupling the event to best-effort notification work.
	if h.dispatcher != nil && webhookVisible {
		dispatchWebhookFallback(h.dispatcher, webhook.EventCommentCreated, commentCreatedPayload)
	}

	// Notify post author about the new comment (if commenter is not
	// the post author). Also notify the parent comment's author for
	// threaded replies (if different from post author + commenter).
	// Both look-ups are async + non-fatal.
	go func() {
		if h.posts == nil {
			return
		}
		ctx := context.Background()
		postData, err := h.posts.GetByID(ctx, postID)
		if err != nil {
			return
		}
		actorID := claims.ParticipantID
		commentID := result.ID

		// Resolve actor display name once for richer copy.
		actorName := "Someone"
		if h.participants != nil {
			if a, err := h.participants.GetByID(ctx, actorID); err == nil && a != nil && a.DisplayName != "" {
				actorName = a.DisplayName
			}
		}

		// Track who we've already notified so a reply doesn't
		// double-hit a single recipient (e.g. when Alice replies
		// to her own comment on her own post).
		notified := map[string]struct{}{actorID: {}}

		// Reply-to-comment notification (priority): if the new
		// comment has a parent, ping the parent's author first so
		// the right notification type lands instead of the
		// generic post_comment.
		if h.comments != nil && req.ParentCommentID != nil && *req.ParentCommentID != "" {
			parent, err := h.comments.GetByID(ctx, *req.ParentCommentID)
			if err == nil && parent != nil && parent.AuthorID != "" {
				if _, dup := notified[parent.AuthorID]; !dup {
					msg := fmt.Sprintf("%s replied to your comment", actorName)
					_ = h.notifications.Create(ctx, parent.AuthorID, "comment_reply", &actorID, &postID, &commentID, msg)
					notified[parent.AuthorID] = struct{}{}
				}
			}
		}

		// Post-author notification — skip if commenter IS the post
		// author or if we already notified them via the reply path.
		postAuthorID := postData.AuthorID
		if _, dup := notified[postAuthorID]; !dup {
			msg := fmt.Sprintf("%s commented on your post", actorName)
			_ = h.notifications.Create(ctx, postAuthorID, "post_comment", &actorID, &postID, &commentID, msg)
			notified[postAuthorID] = struct{}{}
		}

		// (We could fall through here to push webhooks for the
		// extra recipients — keeping the existing payload since
		// downstream consumers only watch comment.created.)
		_ = postAuthorID

		// SSE complements the webhook with live in-app updates.
		if h.hub != nil {
			data, _ := json.Marshal(commentCreatedPayload)
			// Post-author inbox (personal notification).
			h.hub.Publish(postAuthorID, events.Event{Type: "comment.created", Data: string(data)})
			// Post room — drives /post/{id} live-thread mode for
			// every client currently viewing the post.
			h.hub.Publish("post:"+postID, events.Event{Type: "comment.created", Data: string(data)})
		}
	}()

	// Parse @mentions and notify async. The previous implementation
	// used a broader regex that included spaces and never inserted
	// rows into the mentions table — meaning the /u/me/mentions tab
	// could never light up. Now we use the canonical
	// mention.Parse + mention.Resolve, write to BOTH the mentions
	// table and the notifications stream, and only fire webhook /
	// SSE on resolved hits (the old code did so on every name
	// regardless of whether the participant existed).
	if h.participants != nil {
		mentionBody := req.Body
		commenterID := claims.ParticipantID
		commentID := result.ID
		postIDCopy := postID
		// Snapshot the parent-post title for the notification message.
		// Best-effort lookup; falls back to empty title.
		var parentTitle string
		if h.posts != nil {
			if parent, err := h.posts.GetByID(r.Context(), postID); err == nil && parent != nil {
				parentTitle = parent.Title
			}
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			handles := mention.Parse(mentionBody)
			if len(handles) == 0 {
				return
			}

			// @loom mentions are not user-to-user notifications;
			// they kick off a Loom summon and are filtered out of
			// the normal resolve/notify pipeline. Filter before
			// Resolve so we don't waste a participants lookup on it,
			// and so the Loom participant row (display_name 'Loom')
			// isn't notified like a regular user.
			filteredHandles := make([]string, 0, len(handles))
			loomMentioned := false
			for _, handle := range handles {
				if strings.EqualFold(handle, "loom") {
					loomMentioned = true
					continue
				}
				filteredHandles = append(filteredHandles, handle)
			}
			if loomMentioned && h.loom != nil {
				// PARKED: @loom currently does not trigger a summon.
				// The summarize intent we shipped first turned out
				// to be redundant with the platform's existing
				// provenance + citation + Community-Notes posture —
				// it was an AI re-doing work the platform already
				// does better. The plumbing (manager, worker, cache,
				// routes, metrics, post-card UI) stays in place so
				// the next intent can be wired in fast once we
				// decide what Loom should actually do.
				_ = commentID
				_ = commenterID
				_ = postIDCopy
				_ = mentionBody
			}

			if len(filteredHandles) == 0 {
				return
			}
			resolved := mention.Resolve(ctx, commentMentionResolverAdapter{h.participants}, filteredHandles)
			if len(resolved) == 0 {
				return
			}

			// Resolve the actor's display name once.
			actorName := commenterID
			if author, err := h.participants.GetByID(ctx, commenterID); err == nil && author != nil {
				actorName = author.DisplayName
			}

			for _, p := range resolved {
				if p.ID == commenterID {
					continue
				}
				// Drop the mention if the recipient has blocked
				// the commenter — they shouldn't be reachable
				// through @-tags.
				if h.blocks != nil {
					if blocked, _ := h.blocks.IsBlocked(ctx, p.ID, commenterID); blocked {
						continue
					}
				}
				mentionPublic := false
				if h.mentions != nil {
					mentionPublic, _ = h.mentions.CreateForPublicComment(ctx, commentID, p.ID, commenterID)
				}
				actorID := commenterID
				cID := commentID
				msg := mention.FormatMessage(actorName, "comment", parentTitle)
				_ = h.notifications.Create(ctx, p.ID, "mention", &actorID, &postIDCopy, &cID, msg)

				// Dispatch webhook + SSE for mention.
				if h.dispatcher != nil && mentionPublic {
					payload := map[string]any{
						"comment_id":   cID,
						"post_id":      postIDCopy,
						"mentioned_by": commenterID,
						"mentioned_id": p.ID,
					}
					dispatchWebhookFallback(h.dispatcher, webhook.EventMention, payload)
					if h.hub != nil {
						data, _ := json.Marshal(payload)
						h.hub.Publish(p.ID, events.Event{Type: "mention", Data: string(data)})
					}
				}
			}
		}()
	}

	// Return the comment with author data so the frontend can render it properly
	full, err := h.comments.GetByIDWithAuthor(r.Context(), result.ID)
	if err != nil {
		// Fallback to raw comment if join fails
		api.JSON(w, http.StatusCreated, result)
		return
	}
	api.JSON(w, http.StatusCreated, full)
}

// ListByPost handles GET /api/v1/posts/{id}/comments.
// Accepts ?sort=best|new|old|controversial (default: best).
func (h *CommentHandler) ListByPost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "best"
	}

	mode := r.URL.Query().Get("mode")
	threadType := r.URL.Query().Get("thread")

	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	cursor := decodeCursorID(r.URL.Query().Get("cursor"))

	comments, err := h.comments.ListByPost(r.Context(), postID, sort, limit, offset, mode, threadType, cursor)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list comments")
		return
	}

	// Populate user vote and bookmark state if authenticated
	if claims := middleware.GetClaims(r.Context()); claims != nil && len(comments) > 0 {
		commentIDs := make([]string, len(comments))
		for i, c := range comments {
			commentIDs[i] = c.ID
		}

		// Populate user votes
		if h.votes != nil {
			if votes, err := h.votes.GetUserVotesForComments(r.Context(), claims.ParticipantID, commentIDs); err == nil {
				for i := range comments {
					if dir, ok := votes[comments[i].ID]; ok {
						comments[i].UserVote = &dir
					}
				}
			}
		}

		// Populate user bookmarks
		if h.commentBookmarks != nil {
			if bookmarks, err := h.commentBookmarks.GetUserBookmarksForComments(r.Context(), claims.ParticipantID, commentIDs); err == nil {
				for i := range comments {
					if bookmarks[comments[i].ID] {
						comments[i].UserBookmarked = true
					}
				}
			}
		}
	}

	// Keep the long-standing bare-array body compatible. Cursor-aware clients
	// read the continuation token from the header; OFFSET remains accepted for
	// one deprecation cycle.
	if len(comments) == limit && len(comments) > 0 {
		last := comments[len(comments)-1]
		w.Header().Set("X-Next-Cursor", EncodeCursor(last.CreatedAt, last.ID))
		w.Header().Set("Access-Control-Expose-Headers", "X-Next-Cursor")
	}

	api.JSON(w, http.StatusOK, comments)
}

// GetThread handles GET /api/v1/comments/{id}/thread.
// Returns the requested comment along with its full descendant
// subtree and a parent-chain breadcrumb. Used by the comment
// permalink page (/post/<id>/comment/<id>) so a deep reply can
// render at depth 0 with a "Replying to a thread by …" header.
//
// Public read-only — same auth posture as GET /posts/:id/comments.
// Walks the parent_comment_id chain in a recursive CTE; descendants
// pulled with a second recursive CTE rooted at the requested id.
func (h *CommentHandler) GetThread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "comment id is required")
		return
	}

	root, err := h.comments.GetByIDWithAuthor(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "comment not found")
		return
	}

	// Parent chain — walk up until parent_comment_id IS NULL.
	// Returned in root-most-first order so the breadcrumb reads
	// "Anika → Vector → Demeter → (this comment)".
	chain, err := h.comments.GetAncestorChain(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load ancestor chain")
		return
	}

	// Descendants — every comment whose ancestor chain contains
	// the root id. Returned flat; the frontend stitches the tree.
	// Capped at maxDepth=6 + limit=200 by default so a runaway
	// thread doesn't ship a 30-viewport-tall HTML doc on the
	// permalink page. Both knobs are tunable per request.
	maxDepth := parseIntDefault(r.URL.Query().Get("max_depth"), 6)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
	if maxDepth > 20 {
		maxDepth = 20
	}
	if limit > 500 {
		limit = 500
	}
	descendants, err := h.comments.ListDescendants(r.Context(), id, maxDepth, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load descendants")
		return
	}

	// truncated = true when we hit the limit, so the UI can render a
	// "thread continues — view full post" link instead of pretending
	// we returned the whole subtree.
	truncated := len(descendants) >= limit && limit > 0

	api.JSON(w, http.StatusOK, map[string]any{
		"comment":     root,
		"ancestors":   chain,
		"descendants": descendants,
		"post_id":     root.PostID,
		"truncated":   truncated,
		"max_depth":   maxDepth,
		"limit":       limit,
	})
}

// containsLoomMention checks whether a comment body contains an
// @loom mention. Uses the same mention.Parse regex the parser runs
// later (so we don't false-positive on emails like x@loom.com), then
// looks for the canonical handle. Case-insensitive match.
func containsLoomMention(body string) bool {
	for _, h := range mention.Parse(body) {
		if strings.EqualFold(h, "loom") {
			return true
		}
	}
	return false
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}
