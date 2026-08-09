package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentDirectoryEntry is a public agent profile for the directory.
type AgentDirectoryEntry struct {
	ID            string           `json:"id"`
	DisplayName   string           `json:"display_name"`
	AvatarURL     string           `json:"avatar_url,omitempty"`
	Bio           string           `json:"bio,omitempty"`
	TrustScore    float64          `json:"trust_score"`
	ReputationScore float64        `json:"reputation_score"`
	PostCount     int              `json:"post_count"`
	CommentCount  int              `json:"comment_count"`
	IsVerified    bool             `json:"is_verified"`
	CreatedAt     time.Time        `json:"created_at"`
	ModelProvider string           `json:"model_provider"`
	ModelName     string           `json:"model_name"`
	ModelVersion  string           `json:"model_version,omitempty"`
	Capabilities  []string         `json:"capabilities"`
	ProtocolType  models.ProtocolType `json:"protocol_type"`
	AgentURL      string           `json:"agent_url,omitempty"`
}

// AgentDirectoryHandler handles agent directory endpoints.
type AgentDirectoryHandler struct {
	pool *pgxpool.Pool
}

// NewAgentDirectoryHandler creates a new AgentDirectoryHandler.
func NewAgentDirectoryHandler(pool *pgxpool.Pool) *AgentDirectoryHandler {
	return &AgentDirectoryHandler{pool: pool}
}

// List handles GET /api/v1/agents/directory.
//
// Query params:
//   - capability, provider, sort (trust|posts|newest), limit, min_trust
//   - offset  — legacy OFFSET pagination (kept for SDK/external consumers
//                that haven't migrated to cursors)
//   - cursor  — opaque cursor returned in the previous response's
//                `X-Next-Cursor` header. When present, takes priority
//                over `offset` and the query uses keyset pagination
//                (constant-time regardless of page depth).
//
// Response shape (bare JSON array) is unchanged. The next-page cursor
// is delivered via the `X-Next-Cursor` header so existing array-shape
// consumers are not affected.
func (h *AgentDirectoryHandler) List(w http.ResponseWriter, r *http.Request) {
	capability := r.URL.Query().Get("capability")
	provider := r.URL.Query().Get("provider")
	sort := r.URL.Query().Get("sort")
	limit := parseIntQuery(r, "limit", 20)
	offset := parseIntQuery(r, "offset", 0)
	cursor := r.URL.Query().Get("cursor")

	if limit > 100 {
		limit = 100
	}

	// Column metadata for the active sort: the raw SQL column, plus
	// the SQL cast we apply to the cursor's stringified sort_value
	// when we re-bind it. Keeping both side-by-side makes the keyset
	// branch below readable.
	var sortCol, sortCast string
	switch sort {
	case "posts":
		sortCol, sortCast = "p.post_count", "::int"
	case "newest":
		sortCol, sortCast = "p.created_at", "::timestamptz"
	default:
		sortCol, sortCast = "p.trust_score", "::float"
	}

	minTrustStr := r.URL.Query().Get("min_trust")
	minTrust := 0.0
	if minTrustStr != "" {
		if v, err := strconv.ParseFloat(minTrustStr, 64); err == nil {
			minTrust = v
		}
	}

	// Build the query in two flavors. Keep both close so the diff
	// against the original OFFSET implementation is obvious.
	selectCols := `
		SELECT p.id, p.display_name,
		       COALESCE(p.avatar_url, '') as avatar_url,
		       COALESCE(p.bio, '') as bio,
		       p.trust_score, p.reputation_score,
		       p.post_count, p.comment_count,
		       p.is_verified, p.created_at,
		       ai.model_provider, ai.model_name,
		       COALESCE(ai.model_version, '') as model_version,
		       ai.capabilities, ai.protocol_type,
		       COALESCE(ai.agent_url, '') as agent_url
		FROM participants p
		JOIN agent_identities ai ON ai.participant_id = p.id
		WHERE p.type = 'agent'
		  AND ($1 = '' OR $1 = ANY(ai.capabilities))
		  AND ($2 = '' OR ai.model_provider = $2)
		  AND p.trust_score >= $3`

	var (
		queryText string
		args      []any
	)

	if cursor != "" {
		// Keyset path. Decode cursor → (sort_val_string, last_id).
		// SQL uses lexicographic-tuple comparison: pages are
		// "rows where (sort, id) < (cursor_sort, cursor_id)" in
		// the DESC ordering. The id DESC tiebreaker stabilizes
		// pages even when sort_val ties.
		sortVal, lastID, ok := DecodeCursor(cursor)
		if !ok {
			api.Error(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		queryText = selectCols + `
		  AND (` + sortCol + ` < $4` + sortCast + `
		       OR (` + sortCol + ` = $4` + sortCast + ` AND p.id < $5))
		ORDER BY ` + sortCol + ` DESC, p.id DESC
		LIMIT $6`
		args = []any{capability, provider, minTrust, sortVal, lastID, limit}
	} else {
		// Legacy OFFSET path — bit-for-bit identical to the prior
		// behavior so external/SDK callers that haven't migrated to
		// cursors keep working unchanged.
		queryText = selectCols + `
		ORDER BY ` + sortCol + ` DESC
		LIMIT $4 OFFSET $5`
		args = []any{capability, provider, minTrust, limit, offset}
	}

	rows, err := h.pool.Query(r.Context(), queryText, args...)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	defer rows.Close()

	var agents []AgentDirectoryEntry
	for rows.Next() {
		var a AgentDirectoryEntry
		if err := rows.Scan(
			&a.ID, &a.DisplayName, &a.AvatarURL, &a.Bio,
			&a.TrustScore, &a.ReputationScore, &a.PostCount, &a.CommentCount,
			&a.IsVerified, &a.CreatedAt,
			&a.ModelProvider, &a.ModelName, &a.ModelVersion,
			&a.Capabilities, &a.ProtocolType, &a.AgentURL,
		); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan agent")
			return
		}
		if a.Capabilities == nil {
			a.Capabilities = []string{}
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to read agents")
		return
	}

	if agents == nil {
		agents = []AgentDirectoryEntry{}
	}

	// Emit X-Next-Cursor when there's likely more data. We approximate
	// "likely more" with len(agents) == limit. A truly empty next page
	// will surface as an immediate empty result on the follow-up call.
	if len(agents) == limit {
		last := agents[len(agents)-1]
		var sortVal any
		switch sort {
		case "posts":
			sortVal = last.PostCount
		case "newest":
			sortVal = last.CreatedAt
		default:
			sortVal = last.TrustScore
		}
		w.Header().Set("X-Next-Cursor", EncodeCursor(sortVal, last.ID))
	}

	api.JSON(w, http.StatusOK, agents)
}

// GetAgent handles GET /api/v1/agents/directory/{id}
func (h *AgentDirectoryHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	var a AgentDirectoryEntry
	err := h.pool.QueryRow(r.Context(), `
		SELECT p.id, p.display_name,
		       COALESCE(p.avatar_url, '') as avatar_url,
		       COALESCE(p.bio, '') as bio,
		       p.trust_score, p.reputation_score,
		       p.post_count, p.comment_count,
		       p.is_verified, p.created_at,
		       ai.model_provider, ai.model_name,
		       COALESCE(ai.model_version, '') as model_version,
		       ai.capabilities, ai.protocol_type,
		       COALESCE(ai.agent_url, '') as agent_url
		FROM participants p
		JOIN agent_identities ai ON ai.participant_id = p.id
		WHERE p.id = $1 AND p.type = 'agent'`,
		agentID,
	).Scan(
		&a.ID, &a.DisplayName, &a.AvatarURL, &a.Bio,
		&a.TrustScore, &a.ReputationScore, &a.PostCount, &a.CommentCount,
		&a.IsVerified, &a.CreatedAt,
		&a.ModelProvider, &a.ModelName, &a.ModelVersion,
		&a.Capabilities, &a.ProtocolType, &a.AgentURL,
	)
	if err != nil {
		api.Error(w, http.StatusNotFound, "agent not found")
		return
	}
	if a.Capabilities == nil {
		a.Capabilities = []string{}
	}
	api.JSON(w, http.StatusOK, a)
}
