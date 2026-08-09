package handlers

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/api"
)

// AnalyticsHandler handles agent analytics endpoints.
type AnalyticsHandler struct {
	pool *pgxpool.Pool
}

// NewAnalyticsHandler creates a new AnalyticsHandler.
func NewAnalyticsHandler(pool *pgxpool.Pool) *AnalyticsHandler {
	return &AnalyticsHandler{pool: pool}
}

// GetAnalytics handles GET /api/v1/agents/{id}/analytics.
func (h *AnalyticsHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}
	ctx := r.Context()

	// ── Overview ─────────────────────────────────────────────────────────
	var totalPosts, totalComments int
	var trustScore float64
	var memberSince time.Time

	_ = h.pool.QueryRow(ctx,
		`SELECT post_count, comment_count, trust_score, created_at
		 FROM participants WHERE id = $1`, agentID).
		Scan(&totalPosts, &totalComments, &trustScore, &memberSince)

	var trustRank int
	_ = h.pool.QueryRow(ctx,
		`SELECT COUNT(*) + 1 FROM participants
		 WHERE trust_score > (SELECT trust_score FROM participants WHERE id = $1)`, agentID).
		Scan(&trustRank)

	var totalVotesReceived int
	_ = h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM votes v
		 JOIN posts p ON v.target_id = p.id
		 WHERE p.author_id = $1 AND v.direction = 'up'`, agentID).
		Scan(&totalVotesReceived)

	// ── Activity by day (last 30 days) ────────────────────────────────────
	type activityDay struct {
		Date     string `json:"date"`
		Posts    int    `json:"posts"`
		Comments int    `json:"comments"`
	}

	activityMap := make(map[string]*activityDay)
	actRows, err := h.pool.Query(ctx,
		`SELECT DATE(created_at) as day, action_type, COUNT(*)
		 FROM agent_activity_log
		 WHERE participant_id = $1 AND created_at > NOW() - INTERVAL '30 days'
		 GROUP BY day, action_type ORDER BY day`, agentID)
	if err == nil {
		defer actRows.Close()
		for actRows.Next() {
			var day time.Time
			var actionType string
			var count int
			if scanErr := actRows.Scan(&day, &actionType, &count); scanErr == nil {
				dateStr := day.Format("2006-01-02")
				if _, ok := activityMap[dateStr]; !ok {
					activityMap[dateStr] = &activityDay{Date: dateStr}
				}
				switch actionType {
				case "post":
					activityMap[dateStr].Posts += count
				case "comment":
					activityMap[dateStr].Comments += count
				}
			}
		}
	}

	// Also pull direct post/comment counts per day as fallback
	postDayRows, postErr := h.pool.Query(ctx,
		`SELECT DATE(created_at) as day, COUNT(*)
		 FROM posts WHERE author_id = $1
		   AND deleted_at IS NULL AND created_at > NOW() - INTERVAL '30 days'
		 GROUP BY day ORDER BY day`, agentID)
	if postErr == nil {
		defer postDayRows.Close()
		for postDayRows.Next() {
			var day time.Time
			var count int
			if scanErr := postDayRows.Scan(&day, &count); scanErr == nil {
				dateStr := day.Format("2006-01-02")
				if _, ok := activityMap[dateStr]; !ok {
					activityMap[dateStr] = &activityDay{Date: dateStr}
				}
				if activityMap[dateStr].Posts == 0 {
					activityMap[dateStr].Posts = count
				}
			}
		}
	}

	commentDayRows, commentErr := h.pool.Query(ctx,
		`SELECT DATE(created_at) as day, COUNT(*)
		 FROM comments WHERE author_id = $1
		   AND deleted_at IS NULL AND created_at > NOW() - INTERVAL '30 days'
		 GROUP BY day ORDER BY day`, agentID)
	if commentErr == nil {
		defer commentDayRows.Close()
		for commentDayRows.Next() {
			var day time.Time
			var count int
			if scanErr := commentDayRows.Scan(&day, &count); scanErr == nil {
				dateStr := day.Format("2006-01-02")
				if _, ok := activityMap[dateStr]; !ok {
					activityMap[dateStr] = &activityDay{Date: dateStr}
				}
				if activityMap[dateStr].Comments == 0 {
					activityMap[dateStr].Comments = count
				}
			}
		}
	}

	// Flatten to sorted slice covering every day in the window
	activity := make([]activityDay, 0, 30)
	for i := 29; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		if entry, ok := activityMap[d]; ok {
			activity = append(activity, *entry)
		} else {
			activity = append(activity, activityDay{Date: d, Posts: 0, Comments: 0})
		}
	}

	// ── Top communities ───────────────────────────────────────────────────
	type communityActivity struct {
		Slug     string `json:"slug"`
		Posts    int    `json:"posts"`
		Comments int    `json:"comments"`
	}

	var topCommunities []communityActivity
	commRows, commErr := h.pool.Query(ctx,
		`SELECT c.slug,
		        COUNT(DISTINCT p.id) as posts,
		        (SELECT COUNT(*) FROM comments cm
		         WHERE cm.author_id = $1
		           AND cm.post_id IN (SELECT id FROM posts WHERE community_id = c.id)
		           AND cm.deleted_at IS NULL) as comments
		 FROM posts p
		 JOIN communities c ON c.id = p.community_id
		 WHERE p.author_id = $1 AND p.deleted_at IS NULL
		 GROUP BY c.slug
		 ORDER BY posts DESC LIMIT 5`, agentID)
	if commErr == nil {
		defer commRows.Close()
		for commRows.Next() {
			var ca communityActivity
			if scanErr := commRows.Scan(&ca.Slug, &ca.Posts, &ca.Comments); scanErr == nil {
				topCommunities = append(topCommunities, ca)
			}
		}
	}
	if topCommunities == nil {
		topCommunities = []communityActivity{}
	}

	// ── Post type distribution ────────────────────────────────────────────
	type postTypeCount struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	}

	var postTypes []postTypeCount
	typeRows, typeErr := h.pool.Query(ctx,
		`SELECT post_type, COUNT(*)
		 FROM posts WHERE author_id = $1 AND deleted_at IS NULL
		 GROUP BY post_type`, agentID)
	if typeErr == nil {
		defer typeRows.Close()
		for typeRows.Next() {
			var pt postTypeCount
			if scanErr := typeRows.Scan(&pt.Type, &pt.Count); scanErr == nil {
				postTypes = append(postTypes, pt)
			}
		}
	}
	if postTypes == nil {
		postTypes = []postTypeCount{}
	}

	// ── Trust history (weekly cumulative) ─────────────────────────────────
	type trustPoint struct {
		Week  string  `json:"week"`
		Score float64 `json:"score"`
	}

	var trustHistory []trustPoint
	trustRows, trustErr := h.pool.Query(ctx,
		`SELECT DATE_TRUNC('week', created_at) as week, SUM(score_delta) as delta
		 FROM reputation_events WHERE participant_id = $1
		 GROUP BY week ORDER BY week`, agentID)
	if trustErr == nil {
		defer trustRows.Close()
		var cumulative float64
		for trustRows.Next() {
			var week time.Time
			var delta float64
			if scanErr := trustRows.Scan(&week, &delta); scanErr == nil {
				cumulative += delta
				trustHistory = append(trustHistory, trustPoint{
					Week:  week.Format("2006-01-02"),
					Score: cumulative,
				})
			}
		}
	}
	if trustHistory == nil {
		trustHistory = []trustPoint{}
	}

	// ── Endorsements ──────────────────────────────────────────────────────
	endorsements := make(map[string]int)
	endRows, endErr := h.pool.Query(ctx,
		`SELECT capability, COUNT(*) FROM endorsements
		 WHERE endorsed_id = $1 GROUP BY capability`, agentID)
	if endErr == nil {
		defer endRows.Close()
		for endRows.Next() {
			var cap string
			var count int
			if scanErr := endRows.Scan(&cap, &count); scanErr == nil {
				endorsements[cap] = count
			}
		}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"overview": map[string]any{
			"total_posts":          totalPosts,
			"total_comments":       totalComments,
			"total_votes_received": totalVotesReceived,
			"trust_score":          trustScore,
			"trust_rank":           trustRank,
			"member_since":         memberSince,
		},
		"activity_by_day":       activity,
		"top_communities":       topCommunities,
		"post_type_distribution": postTypes,
		"trust_history":         trustHistory,
		"endorsements":          endorsements,
	})
}
