package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/singleflight"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/auth"
	"github.com/RoamXAI/loomfeed/internal/cache"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/feed"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// feedSingleflight deduplicates concurrent identical feed requests
// at the process level. When 10 visitors hit the same hot page in
// the same instant, only ONE underlying SQL query runs and all 10
// share its result. Without this, every concurrent request re-runs
// the JOIN-heavy feed query — origin DB sees N copies of the same
// query for no reason.
//
// Both anon and authenticated viewers go through this — the cached
// base payload is identical for everyone; per-user overlay happens
// after the singleflight returns.
var feedSingleflight singleflight.Group

// FeedHandler handles feed endpoints.
type FeedHandler struct {
	posts       *repository.PostRepo
	communities *repository.CommunityRepo
	cfg         *config.Config
	cache       *cache.RedisCache
	follows     *repository.FollowRepo
	votes       *repository.VoteRepo
	bookmarks   *repository.BookmarkRepo
	blocks      *repository.BlockRepo
	mutes       *repository.MuteRepo
}

// NewFeedHandler creates a new FeedHandler.
func NewFeedHandler(posts *repository.PostRepo, communities *repository.CommunityRepo, cfg *config.Config) *FeedHandler {
	return &FeedHandler{
		posts:       posts,
		communities: communities,
		cfg:         cfg,
	}
}

// WithCache sets the Redis cache for feed responses.
func (h *FeedHandler) WithCache(c *cache.RedisCache) {
	h.cache = c
}

// WithFollows sets the FollowRepo dependency for including followed users' posts in the subscribed feed.
func (h *FeedHandler) WithFollows(follows *repository.FollowRepo) {
	h.follows = follows
}

// WithVotes sets the VoteRepo for populating user vote state on feed posts.
func (h *FeedHandler) WithVotes(votes *repository.VoteRepo) {
	h.votes = votes
}

// WithBookmarks sets the BookmarkRepo for populating user bookmark state on feed posts.
func (h *FeedHandler) WithBookmarks(bookmarks *repository.BookmarkRepo) {
	h.bookmarks = bookmarks
}

// WithBlocksMutes wires the block + mute repos so authenticated
// feed queries can filter out posts from blocked authors and muted
// communities. Both safe to leave nil — when unset, filterBlocked
// is a no-op pass-through.
func (h *FeedHandler) WithBlocksMutes(blocks *repository.BlockRepo, mutes *repository.MuteRepo) {
	h.blocks = blocks
	h.mutes = mutes
}

// filterBlocked removes posts whose author the viewer has blocked,
// or whose community the viewer has muted. Runs in Go after the
// feed query so we don't have to JOIN the block/mute tables onto
// every variant of ListGlobal*. Trade-off: we over-fetch by the
// blocked count, which is fine — blocks are rare and bounded.
//
// On a typical viewer (0 blocks, 0 mutes) this is a fast path:
// both lookups return empty slices and we don't allocate.
func (h *FeedHandler) filterBlocked(ctx context.Context, posts []models.PostWithAuthor, viewerID string) []models.PostWithAuthor {
	if viewerID == "" || len(posts) == 0 {
		return posts
	}
	var blockedSet, mutedSet map[string]struct{}
	if h.blocks != nil {
		ids, err := h.blocks.ListBlockedIDs(ctx, viewerID)
		if err == nil && len(ids) > 0 {
			blockedSet = make(map[string]struct{}, len(ids))
			for _, id := range ids {
				blockedSet[id] = struct{}{}
			}
		}
	}
	if h.mutes != nil {
		ids, err := h.mutes.ListMutedIDs(ctx, viewerID)
		if err == nil && len(ids) > 0 {
			mutedSet = make(map[string]struct{}, len(ids))
			for _, id := range ids {
				mutedSet[id] = struct{}{}
			}
		}
	}
	if blockedSet == nil && mutedSet == nil {
		return posts
	}
	out := make([]models.PostWithAuthor, 0, len(posts))
	for _, p := range posts {
		if blockedSet != nil {
			if _, blocked := blockedSet[p.AuthorID]; blocked {
				continue
			}
		}
		if mutedSet != nil {
			if _, muted := mutedSet[p.CommunityID]; muted {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// populateUserVotes sets UserVote on each post for the authenticated user.
func (h *FeedHandler) populateUserVotes(ctx context.Context, posts []models.PostWithAuthor, voterID string) {
	if h.votes == nil || voterID == "" || len(posts) == 0 {
		return
	}
	postIDs := make([]string, len(posts))
	for i, p := range posts {
		postIDs[i] = p.ID
	}
	votes, err := h.votes.GetUserVotesForPosts(ctx, voterID, postIDs)
	if err != nil || len(votes) == 0 {
		return
	}
	for i := range posts {
		if dir, ok := votes[posts[i].ID]; ok {
			posts[i].UserVote = &dir
		}
	}
}

// personalizeFeed re-orders a feed page using the viewer's follow
// graph and community subscriptions. Cheap signals — no ML, no
// model — but enough to surface "the people you follow / topics
// you care about" within the page the global ranker already chose.
//
// Scoring model: each post starts with a positional weight
// (descending from len → 1) so the global ranked_score order is
// preserved as the baseline. Boosts are added on top:
//
//   follow      +len * 0.50   (followed-author posts land near page top)
//   subscribe   +len * 0.25   (subscribed-community posts surface in the
//                              upper third even when their global rank is low)
//   stacks      followed AND subscribed → strongest signal
//
// Within the same boost tier the input order is preserved via
// stable sort, so two followed posts keep their original relative
// global ranking. Posts with no follow/sub signal keep their
// position relative to each other.
//
// Cost: two batch queries (GetFollowingIDs + ListSubscriptions),
// O(N) classify + O(N log N) sort over the page (usually 25
// posts). Roughly 5-15ms on top of the cached base read.
//
// Degrades to a no-op when:
//   - viewer is anonymous
//   - follows / communities repos aren't wired
//   - viewer follows nothing AND subscribes to nothing
//   - the page has 0 or 1 post (nothing to re-order)
func (h *FeedHandler) personalizeFeed(ctx context.Context, posts []models.PostWithAuthor, viewerID string) []models.PostWithAuthor {
	if viewerID == "" || len(posts) <= 1 {
		return posts
	}

	var followSet map[string]struct{}
	if h.follows != nil {
		if ids, err := h.follows.GetFollowingIDs(ctx, viewerID); err == nil && len(ids) > 0 {
			followSet = make(map[string]struct{}, len(ids))
			for _, id := range ids {
				followSet[id] = struct{}{}
			}
		}
	}

	var subSet map[string]struct{}
	if h.communities != nil {
		if subs, err := h.communities.ListSubscriptions(ctx, viewerID); err == nil && len(subs) > 0 {
			subSet = make(map[string]struct{}, len(subs))
			for _, c := range subs {
				subSet[c.ID] = struct{}{}
			}
		}
	}

	// No per-user signal at all → nothing to do, keep global order.
	if followSet == nil && subSet == nil {
		return posts
	}

	n := float64(len(posts))
	type scored struct {
		idx   int
		score float64
	}
	scoredPosts := make([]scored, len(posts))
	for i := range posts {
		// Positional baseline: rank 0 gets score=n, rank n-1 gets
		// score=1. Preserves the global ranker's order for posts
		// without any personal signal.
		base := n - float64(i)
		var boost float64
		if followSet != nil {
			if _, ok := followSet[posts[i].AuthorID]; ok {
				boost += n * 0.50
			}
		}
		if subSet != nil {
			if _, ok := subSet[posts[i].CommunityID]; ok {
				boost += n * 0.25
			}
		}
		scoredPosts[i] = scored{idx: i, score: base + boost}
	}

	sort.SliceStable(scoredPosts, func(a, b int) bool {
		return scoredPosts[a].score > scoredPosts[b].score
	})

	out := make([]models.PostWithAuthor, len(posts))
	for i, s := range scoredPosts {
		out[i] = posts[s.idx]
	}
	return out
}

// populateUserBookmarks sets UserBookmarked on each post for the authenticated user.
func (h *FeedHandler) populateUserBookmarks(ctx context.Context, posts []models.PostWithAuthor, participantID string) {
	if h.bookmarks == nil || participantID == "" || len(posts) == 0 {
		return
	}
	postIDs := make([]string, len(posts))
	for i, p := range posts {
		postIDs[i] = p.ID
	}
	bookmarks, err := h.bookmarks.GetUserBookmarksForPosts(ctx, participantID, postIDs)
	if err != nil || len(bookmarks) == 0 {
		return
	}
	for i := range posts {
		if bookmarks[posts[i].ID] {
			posts[i].UserBookmarked = true
		}
	}
}

// populateViewerFollowing marks posts whose author the viewer follows —
// the same batch-annotation shape as populateUserVotes. Best-effort:
// on error the CTA just renders as not-following.
func (h *FeedHandler) populateViewerFollowing(ctx context.Context, posts []models.PostWithAuthor, viewerID string) {
	if h.follows == nil || viewerID == "" || len(posts) == 0 {
		return
	}
	authorIDs := make([]string, 0, len(posts))
	seen := map[string]bool{}
	for _, p := range posts {
		if !seen[p.AuthorID] {
			seen[p.AuthorID] = true
			authorIDs = append(authorIDs, p.AuthorID)
		}
	}
	set, err := h.follows.FollowedSet(ctx, viewerID, authorIDs)
	if err != nil || len(set) == 0 {
		return
	}
	for i := range posts {
		posts[i].ViewerFollowing = set[posts[i].AuthorID]
	}
}

// Global handles GET /api/v1/feed.
//
// Both anonymous and authenticated viewers share the same cached
// base payload — the underlying SQL is identical for everyone.
// What differs is the per-viewer overlay: blocked-author filtering
// plus UserVote / UserBookmarked. Those are cheap batch queries
// (~5-15ms total) layered on top of the cached base, so the
// expensive 8-JOIN feed query runs once per (sort, limit, offset,
// type, cursor) tuple every 5 minutes regardless of how many
// signed-in users browse.
//
// Before this change auth users bypassed the cache entirely and
// paid ~850ms per page navigation. Now they pay ~30-60ms (cache
// read + overlay) and the cold path stays singleflight-deduped.
func (h *FeedHandler) Global(w http.ResponseWriter, r *http.Request) {
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "hot"
	}
	postType := r.URL.Query().Get("type")
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	cursor := r.URL.Query().Get("cursor")

	// Build cache subkey from query params. The "feed" namespace
	// version is appended by the cache layer; bumping it via
	// BumpVersion("feed") invalidates every key here in O(1).
	cacheKey := fmt.Sprintf("global:%s:%d:%d:%s", sort, limit, offset, postType)
	if cursor != "" {
		cacheKey = fmt.Sprintf("global:%s:%d:c:%s:%s", sort, limit, cursor, postType)
	}

	claims := middleware.GetClaims(r.Context())

	// Try cache for everyone — auth users overlay per-user state on
	// top of the cached base below.
	if h.cache != nil {
		if cached, _ := h.cache.GetVersioned(r.Context(), "feed", cacheKey); cached != nil {
			h.writeFeedResponse(w, r.Context(), cached, claims, "HIT")
			return
		}
	}

	// Cache miss — deduplicate concurrent identical requests through
	// singleflight so 100 concurrent visitors hitting a cold cache
	// only run ONE underlying SQL query. Without this, every cache
	// miss under load fans out to N parallel JOIN-heavy queries on
	// origin DB, pool starves, and everyone waits.
	result, err, _ := feedSingleflight.Do(cacheKey, func() (any, error) {
		return h.fetchAndCacheBaseFeed(r.Context(), cacheKey, sort, postType, limit, offset, cursor)
	})
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch global feed")
		return
	}
	h.writeFeedResponse(w, r.Context(), result.([]byte), claims, "MISS")
}

// cachedFeedShape mirrors models.PaginatedResponse but types Data as
// the concrete post slice so authenticated requests can decode →
// overlay user-specific fields → re-marshal without round-tripping
// through interface{}. JSON-compatible with PaginatedResponse, so the
// cached bytes work in both directions.
type cachedFeedShape struct {
	Data        []models.PostWithAuthor `json:"data"`
	Total       int                     `json:"total"`
	Limit       int                     `json:"limit"`
	Offset      int                     `json:"offset"`
	HasMore     bool                    `json:"has_more"`
	NextCursor  string                  `json:"next_cursor,omitempty"`
	RetrievedAt time.Time               `json:"retrieved_at"`
}

// writeFeedResponse sends the cached base payload to the client.
// Anonymous viewers get the bytes verbatim. Authenticated viewers
// have blocked-author filtering and UserVote / UserBookmarked
// overlaid before the response is re-marshaled.
//
// If overlay or re-marshal fails the anon payload is sent as a
// fallback — better to render the feed with stale user state than
// 500 a page that would otherwise work.
func (h *FeedHandler) writeFeedResponse(w http.ResponseWriter, ctx context.Context, base []byte, claims *auth.Claims, cacheStatus string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, s-maxage=60, stale-while-revalidate=300")
	w.Header().Set("X-Cache", cacheStatus)

	if claims == nil {
		_, _ = w.Write(base)
		return
	}

	var resp cachedFeedShape
	if err := json.Unmarshal(base, &resp); err != nil {
		_, _ = w.Write(base)
		return
	}

	resp.Data = h.filterBlocked(ctx, resp.Data, claims.ParticipantID)
	resp.Data = h.personalizeFeed(ctx, resp.Data, claims.ParticipantID)
	h.populateUserVotes(ctx, resp.Data, claims.ParticipantID)
	h.populateUserBookmarks(ctx, resp.Data, claims.ParticipantID)
	h.populateViewerFollowing(ctx, resp.Data, claims.ParticipantID)
	resp.HasMore = resp.Offset+resp.Limit < resp.Total
	if len(resp.Data) > 0 {
		resp.NextCursor = resp.Data[len(resp.Data)-1].ID
	} else {
		resp.NextCursor = ""
	}

	out, err := json.Marshal(resp)
	if err != nil {
		_, _ = w.Write(base)
		return
	}
	_, _ = w.Write(out)
}

// fetchAndCacheBaseFeed runs the underlying SQL and writes the
// marshaled base payload to Redis. The result is the canonical feed
// state for the (sort, limit, offset, type, cursor) tuple WITHOUT
// any per-user overlay (UserVote / UserBookmarked / block filter).
// Both anonymous and authenticated handlers consume this same
// payload — auth handlers overlay viewer-specific fields in
// writeFeedResponse after reading the cache.
//
// Designed to be the work function for singleflight: 100 concurrent
// identical requests collapse to a single SQL execution, all
// callers share the result.
//
// 5-minute Redis TTL: hot/new/top feeds don't shift fast enough that
// 60s vs 5min is human-perceptible, and quintupling the TTL means
// the cold-path SQL fires 5x less often.
func (h *FeedHandler) fetchAndCacheBaseFeed(ctx context.Context, cacheKey, sort, postType string, limit, offset int, cursor string) ([]byte, error) {
	var posts []models.PostWithAuthor
	var total int
	var err error

	switch {
	case (sort == "for_you" || sort == "hot") && cursor == "":
		// 8x candidate pool gives Diversify real headroom to mix
		// communities, authors, and (after Tier 2) per-user
		// affinities. Was 2x, which left only ~5 swap candidates
		// past the requested page — Diversify could enforce its
		// streak rules but couldn't introduce novelty that wasn't
		// already in the global top N. At ~57k posts the 200-row
		// fetch is still <20ms on the materialized ranked_score
		// index.
		candidateCount := limit * 8
		posts, total, err = h.posts.ListGlobalRanked(ctx, postType, candidateCount, offset)
		if err == nil && len(posts) > limit {
			posts = feed.Diversify(posts, limit)
		}
	case sort == "live":
		posts, total, err = h.posts.ListGlobalLive(ctx, postType, limit, offset)
	default:
		posts, total, err = h.posts.ListGlobal(ctx, sort, postType, limit, offset, cursor)
	}
	if err != nil {
		return nil, err
	}

	resp := models.PaginatedResponse{
		Data:        posts,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		HasMore:     offset+limit < total,
		RetrievedAt: time.Now(),
	}
	if len(posts) > 0 {
		resp.NextCursor = posts[len(posts)-1].ID
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	if h.cache != nil {
		_ = h.cache.SetVersioned(ctx, "feed", cacheKey, data, 5*time.Minute)
	}
	return data, nil
}

// Subscribed handles GET /api/v1/feed/subscribed — returns posts from communities the user subscribes to.
func (h *FeedHandler) Subscribed(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "login required")
		return
	}
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "hot"
	}
	postType := r.URL.Query().Get("type")
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	cursor := r.URL.Query().Get("cursor")

	// Subscribed feed is per-user — not cacheable.

	// Live sort is platform-wide by design — it surfaces whatever is
	// *currently active* on Loomfeed, regardless of which communities
	// a given user follows. Short-circuit straight to ListGlobalLive so
	// the home tab's 15s refresher actually sees new posts land.
	if sort == "live" {
		posts, total, err := h.posts.ListGlobalLive(r.Context(), postType, limit, offset)
		if err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to fetch feed")
			return
		}
		posts = h.filterBlocked(r.Context(), posts, claims.ParticipantID)
		h.populateUserVotes(r.Context(), posts, claims.ParticipantID)
		h.populateUserBookmarks(r.Context(), posts, claims.ParticipantID)
		h.populateViewerFollowing(r.Context(), posts, claims.ParticipantID)
		api.JSON(w, http.StatusOK, models.PaginatedResponse{
			Data:        posts,
			Total:       total,
			Limit:       limit,
			Offset:      offset,
			HasMore:     offset+limit < total,
			RetrievedAt: time.Now(),
		})
		return
	}

	// Fetch followed user IDs to include their posts in the feed
	var followedIDs []string
	if h.follows != nil {
		followedIDs, _ = h.follows.GetFollowingIDs(r.Context(), claims.ParticipantID)
	}

	fetchLimit := limit
	if sort == "hot" && cursor == "" {
		fetchLimit = limit * 2
	}

	posts, total, err := h.posts.ListBySubscriptionsAndFollows(
		r.Context(), claims.ParticipantID, followedIDs, sort, postType, fetchLimit, offset, cursor)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch feed")
		return
	}

	if sort == "hot" && cursor == "" && len(posts) > limit {
		posts = feed.Diversify(posts, limit)
	}

	// Populate user vote and bookmark state
	posts = h.filterBlocked(r.Context(), posts, claims.ParticipantID)
	h.populateUserVotes(r.Context(), posts, claims.ParticipantID)
	h.populateUserBookmarks(r.Context(), posts, claims.ParticipantID)
	h.populateViewerFollowing(r.Context(), posts, claims.ParticipantID)

	resp := models.PaginatedResponse{
		Data:        posts,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		HasMore:     offset+limit < total,
		RetrievedAt: time.Now(),
	}

	if len(posts) > 0 {
		resp.NextCursor = posts[len(posts)-1].ID
	}

	api.JSON(w, http.StatusOK, resp)
}

// ByCommunity handles GET /api/v1/communities/{slug}/feed.
func (h *FeedHandler) ByCommunity(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		api.Error(w, http.StatusBadRequest, "slug is required")
		return
	}

	community, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get community")
		return
	}

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "hot"
	}
	postType := r.URL.Query().Get("type")
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	cursor := r.URL.Query().Get("cursor")

	cacheKey := fmt.Sprintf("community:%s:%s:%d:%d", slug, sort, limit, offset)
	if cursor != "" {
		cacheKey = fmt.Sprintf("community:%s:%s:%d:c:%s", slug, sort, limit, cursor)
	}

	// Cache only for unauthenticated users
	commClaims := middleware.GetClaims(r.Context())
	if h.cache != nil && commClaims == nil {
		if cached, _ := h.cache.GetVersioned(r.Context(), "feed", cacheKey); cached != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(cached)
			return
		}
	}

	posts, total, err := h.posts.ListByCommunity(r.Context(), community.ID, sort, postType, limit, offset, cursor)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch community feed")
		return
	}

	// Populate user vote and bookmark state if authenticated
	if commClaims != nil {
		h.populateUserVotes(r.Context(), posts, commClaims.ParticipantID)
		h.populateUserBookmarks(r.Context(), posts, commClaims.ParticipantID)
		h.populateViewerFollowing(r.Context(), posts, commClaims.ParticipantID)
	}

	resp := models.PaginatedResponse{
		Data:        posts,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		HasMore:     offset+limit < total,
		RetrievedAt: time.Now(),
	}

	if len(posts) > 0 {
		resp.NextCursor = posts[len(posts)-1].ID
	}

	if h.cache != nil && commClaims == nil {
		if data, err := json.Marshal(resp); err == nil {
			// 60s TTL for anon feed: hot/new/top don't shift fast
			// enough that 30s vs 60s is human-perceptible, and
			// doubling the TTL means the cold-path SQL fires half
			// as often. Authenticated users always bypass cache
			// anyway (claims != nil short-circuits above), so this
			// only affects logged-out browsing — which is the
			// common case.
			_ = h.cache.SetVersioned(r.Context(), "feed", cacheKey, data, 60*time.Second)
		}
	}

	w.Header().Set("X-Cache", "MISS")
	api.JSON(w, http.StatusOK, resp)
}

// ByTag handles GET /api/v1/tags/{tag}/posts — posts carrying an exact tag,
// powering the public topic landing pages at /t/<tag>. Mirrors ByCommunity's
// sort / pagination / 60s anon cache / vote-overlay; there's no entity to
// look up (tags are a denormalised column), so the tag string is validated
// directly. The ListByTag query is parameterised, so the tag is injection-safe.
func (h *FeedHandler) ByTag(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	if tag == "" {
		api.Error(w, http.StatusBadRequest, "tag is required")
		return
	}
	if len(tag) > 100 {
		api.Error(w, http.StatusBadRequest, "tag too long")
		return
	}

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "hot"
	}
	postType := r.URL.Query().Get("type")
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	cursor := r.URL.Query().Get("cursor")

	cacheKey := fmt.Sprintf("tag:%s:%s:%d:%d", tag, sort, limit, offset)
	if cursor != "" {
		cacheKey = fmt.Sprintf("tag:%s:%s:%d:c:%s", tag, sort, limit, cursor)
	}

	claims := middleware.GetClaims(r.Context())
	if h.cache != nil && claims == nil {
		if cached, _ := h.cache.GetVersioned(r.Context(), "feed", cacheKey); cached != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(cached)
			return
		}
	}

	posts, total, err := h.posts.ListByTag(r.Context(), tag, sort, postType, limit, offset, cursor)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch tag feed")
		return
	}

	if claims != nil {
		h.populateUserVotes(r.Context(), posts, claims.ParticipantID)
		h.populateUserBookmarks(r.Context(), posts, claims.ParticipantID)
		h.populateViewerFollowing(r.Context(), posts, claims.ParticipantID)
	}

	resp := models.PaginatedResponse{
		Data:        posts,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
		HasMore:     offset+limit < total,
		RetrievedAt: time.Now(),
	}
	if len(posts) > 0 {
		resp.NextCursor = posts[len(posts)-1].ID
	}

	if h.cache != nil && claims == nil {
		if data, err := json.Marshal(resp); err == nil {
			_ = h.cache.SetVersioned(r.Context(), "feed", cacheKey, data, 60*time.Second)
		}
	}

	w.Header().Set("X-Cache", "MISS")
	api.JSON(w, http.StatusOK, resp)
}
