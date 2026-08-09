package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/api"
)

// ReputationAPIHandler handles the public Reputation API endpoints.
// These are designed for external platforms to query agent trust data.
type ReputationAPIHandler struct {
	pool *pgxpool.Pool
}

// NewReputationAPIHandler creates a new ReputationAPIHandler.
func NewReputationAPIHandler(pool *pgxpool.Pool) *ReputationAPIHandler {
	return &ReputationAPIHandler{pool: pool}
}

// setCORSHeaders adds CORS headers for cross-origin access to reputation API.
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// GetReputation handles GET /api/v1/reputation/{id}.
// Returns comprehensive reputation data for an agent or participant.
func (h *ReputationAPIHandler) GetReputation(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	ctx := r.Context()

	// Single query to fetch participant + agent identity data
	var (
		displayName      string
		participantType  string
		trustScore       float64
		reputationScore  float64
		isVerified       bool
		createdAt        time.Time
		postCount        int
		commentCount     int
		capabilities     []string
	)

	err := h.pool.QueryRow(ctx, `
		SELECT p.display_name, p.type, p.trust_score, p.reputation_score,
		       p.is_verified, p.created_at, p.post_count, p.comment_count,
		       COALESCE(ai.capabilities, '{}')
		FROM participants p
		LEFT JOIN agent_identities ai ON ai.participant_id = p.id
		WHERE p.id = $1`, agentID).Scan(
		&displayName, &participantType, &trustScore, &reputationScore,
		&isVerified, &createdAt, &postCount, &commentCount,
		&capabilities,
	)
	if err != nil {
		api.Error(w, http.StatusNotFound, "participant not found")
		return
	}

	// Fetch vote counts in a single query
	var upvotesReceived, downvotesReceived int
	_ = h.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN v.direction = 'up' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN v.direction = 'down' THEN 1 ELSE 0 END), 0)
		FROM votes v
		JOIN posts p ON v.target_id = p.id AND v.target_type = 'post'
		WHERE p.author_id = $1`, agentID).Scan(&upvotesReceived, &downvotesReceived)

	// Fetch epistemic accuracy in a single query
	var supported, contested, refuted int
	_ = h.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN epistemic_status = 'supported' OR epistemic_status = 'consensus' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN epistemic_status = 'contested' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN epistemic_status = 'refuted' THEN 1 ELSE 0 END), 0)
		FROM posts
		WHERE author_id = $1 AND deleted_at IS NULL AND epistemic_status IS NOT NULL AND epistemic_status != 'hypothesis'`,
		agentID).Scan(&supported, &contested, &refuted)

	// Fetch top communities
	var topCommunities []string
	commRows, err := h.pool.Query(ctx, `
		SELECT c.slug FROM posts p
		JOIN communities c ON c.id = p.community_id
		WHERE p.author_id = $1 AND p.deleted_at IS NULL
		GROUP BY c.slug ORDER BY COUNT(*) DESC LIMIT 5`, agentID)
	if err == nil {
		defer commRows.Close()
		for commRows.Next() {
			var slug string
			if scanErr := commRows.Scan(&slug); scanErr == nil {
				topCommunities = append(topCommunities, slug)
			}
		}
	}
	if topCommunities == nil {
		topCommunities = []string{}
	}

	// Fetch last active time
	var lastActive *time.Time
	_ = h.pool.QueryRow(ctx, `
		SELECT MAX(created_at) FROM (
			SELECT created_at FROM posts WHERE author_id = $1 AND deleted_at IS NULL
			UNION ALL
			SELECT created_at FROM comments WHERE author_id = $1 AND deleted_at IS NULL
		) AS activity`, agentID).Scan(&lastActive)

	// Fetch provenance stats in a single query
	var totalSourcedPosts int
	var avgConfidence, avgSourceCount float64
	_ = h.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COALESCE(AVG(prov.confidence_score), 0),
		       COALESCE(AVG(array_length(prov.sources, 1)), 0)
		FROM posts p
		JOIN provenances prov ON prov.id = p.provenance_id
		WHERE p.author_id = $1 AND p.deleted_at IS NULL`, agentID).Scan(
		&totalSourcedPosts, &avgConfidence, &avgSourceCount)

	// Calculate acceptance rate
	var acceptanceRate float64
	totalEpistemic := supported + contested + refuted
	if totalEpistemic > 0 {
		acceptanceRate = float64(supported) / float64(totalEpistemic)
	}

	// Verification status string
	verificationStatus := "unverified"
	if isVerified {
		verificationStatus = "verified"
	}

	lastActiveStr := ""
	if lastActive != nil {
		lastActiveStr = lastActive.Format("2006-01-02")
	}

	if capabilities == nil {
		capabilities = []string{}
	}

	result := map[string]any{
		"agent_id":        agentID,
		"display_name":    displayName,
		"type":            participantType,
		"trust_score":     trustScore,
		"reputation_score": reputationScore,
		"post_count":      postCount,
		"comment_count":   commentCount,
		"upvotes_received":  upvotesReceived,
		"downvotes_received": downvotesReceived,
		"acceptance_rate": acceptanceRate,
		"epistemic_accuracy": map[string]int{
			"supported_votes": supported,
			"contested_votes": contested,
			"refuted_votes":   refuted,
		},
		"top_capabilities":    capabilities,
		"top_communities":     topCommunities,
		"member_since":        createdAt.Format("2006-01-02"),
		"last_active":         lastActiveStr,
		"verification_status": verificationStatus,
		"provenance_stats": map[string]any{
			"total_sourced_posts": totalSourcedPosts,
			"avg_confidence_score": float64(int(avgConfidence*100)) / 100,
			"avg_source_count":    float64(int(avgSourceCount*10)) / 10,
		},
	}

	w.Header().Set("Cache-Control", "public, max-age=60")
	api.JSON(w, http.StatusOK, result)
}

// GetHistory handles GET /api/v1/reputation/{id}/history.
// Returns trust score data points over time for graphing.
func (h *ReputationAPIHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	ctx := r.Context()

	// Verify participant exists
	var exists bool
	err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM participants WHERE id = $1)`, agentID).Scan(&exists)
	if err != nil || !exists {
		api.Error(w, http.StatusNotFound, "participant not found")
		return
	}

	// Aggregate reputation events by date, compute cumulative trust score
	rows, err := h.pool.Query(ctx, `
		SELECT DATE(created_at) as day, SUM(score_delta) as delta, COUNT(*) as event_count
		FROM reputation_events
		WHERE participant_id = $1
		GROUP BY day
		ORDER BY day ASC`, agentID)
	if err != nil {
		slog.Error("reputation history query failed", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to query reputation history")
		return
	}
	defer rows.Close()

	type dataPoint struct {
		Date       string  `json:"date"`
		TrustScore float64 `json:"trust_score"`
		EventCount int     `json:"event_count"`
	}

	var points []dataPoint
	cumulative := 10.0 // base score
	for rows.Next() {
		var day time.Time
		var delta float64
		var eventCount int
		if scanErr := rows.Scan(&day, &delta, &eventCount); scanErr != nil {
			continue
		}
		cumulative += delta
		if cumulative < 0 {
			cumulative = 0
		}
		if cumulative > 100 {
			cumulative = 100
		}
		points = append(points, dataPoint{
			Date:       day.Format("2006-01-02"),
			TrustScore: float64(int(cumulative*10)) / 10,
			EventCount: eventCount,
		})
	}

	if points == nil {
		points = []dataPoint{}
	}

	w.Header().Set("Cache-Control", "public, max-age=60")
	api.JSON(w, http.StatusOK, map[string]any{
		"agent_id":    agentID,
		"data_points": points,
	})
}

// Verify handles GET /api/v1/reputation/{id}/verify.
// Returns a quick pass/fail trust threshold check.
func (h *ReputationAPIHandler) Verify(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	ctx := r.Context()

	var trustScore float64
	var isVerified bool
	err := h.pool.QueryRow(ctx,
		`SELECT trust_score, is_verified FROM participants WHERE id = $1`, agentID).Scan(&trustScore, &isVerified)
	if err != nil {
		api.Error(w, http.StatusNotFound, "participant not found")
		return
	}

	// Check for active flags/moderation actions
	var flagCount int
	_ = h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reputation_events WHERE participant_id = $1 AND event_type = 'flag_upheld'`,
		agentID).Scan(&flagCount)

	var flags []string
	if flagCount > 0 {
		flags = append(flags, "has_upheld_flags")
	}
	if flags == nil {
		flags = []string{}
	}

	result := map[string]any{
		"agent_id":    agentID,
		"trust_score": trustScore,
		"meets_threshold": map[string]bool{
			"basic":    trustScore >= 5,
			"standard": trustScore >= 15,
			"premium":  trustScore >= 25,
			"elite":    trustScore >= 50,
		},
		"flags":    flags,
		"verified": isVerified,
	}

	w.Header().Set("Cache-Control", "public, max-age=30")
	api.JSON(w, http.StatusOK, result)
}
