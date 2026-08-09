package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/api"
)

// ExportHandler handles dataset export endpoints.
type ExportHandler struct {
	pool *pgxpool.Pool
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(pool *pgxpool.Pool) *ExportHandler {
	return &ExportHandler{pool: pool}
}

// exportAuthor represents an author in export output.
type exportAuthor struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Model      string  `json:"model,omitempty"`
	TrustScore float64 `json:"trust_score"`
}

// exportProvenance represents provenance in export output.
type exportProvenance struct {
	Sources    []string `json:"sources"`
	Confidence float64  `json:"confidence"`
	Method     string   `json:"method"`
}

// exportPost is the shape returned by the posts export endpoint.
type exportPost struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Body             string            `json:"body"`
	PostType         string            `json:"post_type"`
	Author           exportAuthor      `json:"author"`
	Provenance       *exportProvenance `json:"provenance,omitempty"`
	EpistemicStatus  string            `json:"epistemic_status"`
	VoteScore        int               `json:"vote_score"`
	CommentCount     int               `json:"comment_count"`
	Community        string            `json:"community"`
	Tags             []string          `json:"tags"`
	CreatedAt        time.Time         `json:"created_at"`
}

// exportComment is a comment in debate/thread export.
type exportComment struct {
	ID       string          `json:"id"`
	Author   string          `json:"author"`
	Trust    float64         `json:"trust"`
	Body     string          `json:"body"`
	Depth    int             `json:"depth"`
	Score    int             `json:"score"`
	Replies  []exportComment `json:"replies,omitempty"`
}

// exportDebate is the shape returned by the debates export endpoint.
type exportDebate struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	PositionA       string          `json:"position_a"`
	PositionB       string          `json:"position_b"`
	Comments        []exportComment `json:"comments"`
	EpistemicStatus string          `json:"epistemic_status"`
	VoteScore       int             `json:"vote_score"`
}

// exportThread is the shape returned by the threads export endpoint.
type exportThread struct {
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	Body   string          `json:"body"`
	Author exportAuthor    `json:"author"`
	Thread []exportComment `json:"thread"`
}

// parseFloatQuery parses a float64 query parameter with a default value.
func parseFloatQuery(r *http.Request, key string, defaultVal float64) float64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return f
}

// Posts handles GET /api/v1/export/posts.
func (h *ExportHandler) Posts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	community := q.Get("community")
	postType := q.Get("post_type")
	minScore := parseIntQuery(r, "min_score", 0)
	minTrust := parseFloatQuery(r, "min_trust", 0)
	epistemicStatus := q.Get("epistemic_status")
	since := q.Get("since")
	until := q.Get("until")
	limit := parseIntQuery(r, "limit", 100)
	offset := parseIntQuery(r, "offset", 0)
	format := q.Get("format")
	if format == "" {
		format = "jsonl"
	}

	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	// Build dynamic query
	query := `
		SELECT p.id, p.title, p.body, p.post_type,
			   par.display_name, par.type, COALESCE(ai.model_name, ''), par.trust_score,
			   prov.sources, prov.confidence_score, prov.generation_method,
			   COALESCE(p.epistemic_status, 'hypothesis'), p.vote_score, p.comment_count,
			   c.slug, p.tags, p.created_at
		FROM posts p
		JOIN participants par ON par.id = p.author_id
		JOIN communities c ON c.id = p.community_id
		LEFT JOIN agent_identities ai ON ai.participant_id = par.id
		LEFT JOIN provenances prov ON prov.id = p.provenance_id
		WHERE p.deleted_at IS NULL`

	args := []any{}
	argIdx := 0
	nextArg := func() string {
		argIdx++
		return fmt.Sprintf("$%d", argIdx)
	}

	if community != "" {
		query += " AND c.slug = " + nextArg()
		args = append(args, community)
	}
	if postType != "" {
		query += " AND p.post_type = " + nextArg()
		args = append(args, postType)
	}
	if minScore > 0 {
		query += " AND p.vote_score >= " + nextArg()
		args = append(args, minScore)
	}
	if minTrust > 0 {
		query += " AND par.trust_score >= " + nextArg()
		args = append(args, minTrust)
	}
	if epistemicStatus != "" {
		query += " AND p.epistemic_status = " + nextArg()
		args = append(args, epistemicStatus)
	}
	if since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err == nil {
			query += " AND p.created_at >= " + nextArg()
			args = append(args, t)
		}
	}
	if until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err == nil {
			query += " AND p.created_at <= " + nextArg()
			args = append(args, t)
		}
	}

	query += " ORDER BY p.created_at DESC"
	query += " LIMIT " + nextArg()
	args = append(args, limit)
	query += " OFFSET " + nextArg()
	args = append(args, offset)

	rows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		slog.Error("export posts query failed", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to query posts")
		return
	}
	defer rows.Close()

	var results []exportPost
	for rows.Next() {
		var ep exportPost
		var authorName, authorType, modelName string
		var trustScore float64
		var provSources []string
		var provConfidence *float64
		var provMethod *string
		var tags []string

		if err := rows.Scan(
			&ep.ID, &ep.Title, &ep.Body, &ep.PostType,
			&authorName, &authorType, &modelName, &trustScore,
			&provSources, &provConfidence, &provMethod,
			&ep.EpistemicStatus, &ep.VoteScore, &ep.CommentCount,
			&ep.Community, &tags, &ep.CreatedAt,
		); err != nil {
			slog.Error("export posts scan failed", "error", err)
			api.Error(w, http.StatusInternalServerError, "failed to read post")
			return
		}

		ep.Author = exportAuthor{
			Name:       authorName,
			Type:       authorType,
			Model:      modelName,
			TrustScore: trustScore,
		}
		if provConfidence != nil {
			ep.Provenance = &exportProvenance{
				Sources:    provSources,
				Confidence: *provConfidence,
			}
			if provMethod != nil {
				ep.Provenance.Method = *provMethod
			}
		}
		if tags != nil {
			ep.Tags = tags
		} else {
			ep.Tags = []string{}
		}

		results = append(results, ep)
	}
	if err := rows.Err(); err != nil {
		slog.Error("export posts rows error", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to read posts")
		return
	}

	if results == nil {
		results = []exportPost{}
	}

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(results)
		return
	}

	// JSONL (default)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	flusher, canFlush := w.(http.Flusher)
	for _, ep := range results {
		_ = enc.Encode(ep)
		if canFlush {
			flusher.Flush()
		}
	}
}

// Debates handles GET /api/v1/export/debates.
func (h *ExportHandler) Debates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	limit := parseIntQuery(r, "limit", 100)
	offset := parseIntQuery(r, "offset", 0)
	format := q.Get("format")
	if format == "" {
		format = "jsonl"
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	// Fetch debate posts
	debateRows, err := h.pool.Query(ctx, `
		SELECT p.id, p.title, p.body, p.metadata,
			   COALESCE(p.epistemic_status, 'hypothesis'), p.vote_score
		FROM posts p
		WHERE p.post_type = 'debate' AND p.deleted_at IS NULL
		ORDER BY p.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		slog.Error("export debates query failed", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to query debates")
		return
	}
	defer debateRows.Close()

	type debateRaw struct {
		id              string
		title           string
		body            string
		metadata        map[string]any
		epistemicStatus string
		voteScore       int
	}
	var debates []debateRaw
	for debateRows.Next() {
		var d debateRaw
		var metaBytes []byte
		if err := debateRows.Scan(&d.id, &d.title, &d.body, &metaBytes, &d.epistemicStatus, &d.voteScore); err != nil {
			slog.Error("export debates scan failed", "error", err)
			api.Error(w, http.StatusInternalServerError, "failed to read debate")
			return
		}
		if metaBytes != nil {
			_ = json.Unmarshal(metaBytes, &d.metadata)
		}
		debates = append(debates, d)
	}
	if err := debateRows.Err(); err != nil {
		slog.Error("export debates rows error", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to read debates")
		return
	}

	var results []exportDebate
	for _, d := range debates {
		ed := exportDebate{
			ID:              d.id,
			Title:           d.title,
			EpistemicStatus: d.epistemicStatus,
			VoteScore:       d.voteScore,
		}

		// Extract positions from metadata (debate posts store position_a, position_b)
		if d.metadata != nil {
			if a, ok := d.metadata["position_a"].(string); ok {
				ed.PositionA = a
			}
			if b, ok := d.metadata["position_b"].(string); ok {
				ed.PositionB = b
			}
		}
		// Fall back to body if positions not in metadata
		if ed.PositionA == "" && ed.PositionB == "" {
			ed.PositionA = d.body
		}

		// Fetch comments for this debate
		commentRows, err := h.pool.Query(ctx, `
			SELECT c.id, par.display_name, par.trust_score, c.body, c.depth, c.vote_score, c.parent_comment_id
			FROM comments c
			JOIN participants par ON par.id = c.author_id
			WHERE c.post_id = $1 AND c.deleted_at IS NULL
			ORDER BY c.created_at ASC
		`, d.id)
		if err != nil {
			slog.Error("export debate comments query failed", "error", err)
			continue
		}

		var flatComments []exportComment
		for commentRows.Next() {
			var ec exportComment
			var parentID *string
			if err := commentRows.Scan(&ec.ID, &ec.Author, &ec.Trust, &ec.Body, &ec.Depth, &ec.Score, &parentID); err != nil {
				slog.Error("export debate comment scan failed", "error", err)
				continue
			}
			flatComments = append(flatComments, ec)
		}
		commentRows.Close()

		ed.Comments = flatComments
		if ed.Comments == nil {
			ed.Comments = []exportComment{}
		}

		results = append(results, ed)
	}

	if results == nil {
		results = []exportDebate{}
	}

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(results)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	flusher, canFlush := w.(http.Flusher)
	for _, ed := range results {
		_ = enc.Encode(ed)
		if canFlush {
			flusher.Flush()
		}
	}
}

// Threads handles GET /api/v1/export/threads.
func (h *ExportHandler) Threads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	limit := parseIntQuery(r, "limit", 100)
	offset := parseIntQuery(r, "offset", 0)
	format := q.Get("format")
	if format == "" {
		format = "jsonl"
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	// Fetch posts with author info
	postRows, err := h.pool.Query(ctx, `
		SELECT p.id, p.title, p.body,
			   par.display_name, par.type, COALESCE(ai.model_name, ''), par.trust_score
		FROM posts p
		JOIN participants par ON par.id = p.author_id
		LEFT JOIN agent_identities ai ON ai.participant_id = par.id
		WHERE p.deleted_at IS NULL
		ORDER BY p.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		slog.Error("export threads query failed", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to query threads")
		return
	}
	defer postRows.Close()

	type postRaw struct {
		id         string
		title      string
		body       string
		author     exportAuthor
	}
	var posts []postRaw
	for postRows.Next() {
		var p postRaw
		if err := postRows.Scan(&p.id, &p.title, &p.body,
			&p.author.Name, &p.author.Type, &p.author.Model, &p.author.TrustScore); err != nil {
			slog.Error("export threads post scan failed", "error", err)
			api.Error(w, http.StatusInternalServerError, "failed to read thread")
			return
		}
		posts = append(posts, p)
	}
	if err := postRows.Err(); err != nil {
		slog.Error("export threads post rows error", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to read threads")
		return
	}

	var results []exportThread
	for _, p := range posts {
		et := exportThread{
			ID:     p.id,
			Title:  p.title,
			Body:   p.body,
			Author: p.author,
		}

		// Fetch all comments for this post
		commentRows, err := h.pool.Query(ctx, `
			SELECT c.id, par.display_name, par.trust_score, c.body, c.depth, c.vote_score, c.parent_comment_id
			FROM comments c
			JOIN participants par ON par.id = c.author_id
			WHERE c.post_id = $1 AND c.deleted_at IS NULL
			ORDER BY c.created_at ASC
		`, p.id)
		if err != nil {
			slog.Error("export thread comments query failed", "error", err)
			continue
		}

		var flat []flatComment
		for commentRows.Next() {
			var fc flatComment
			if err := commentRows.Scan(&fc.ID, &fc.Author, &fc.Trust, &fc.Body, &fc.Depth, &fc.Score, &fc.parentID); err != nil {
				slog.Error("export thread comment scan failed", "error", err)
				continue
			}
			flat = append(flat, fc)
		}
		commentRows.Close()

		// Build tree from flat comments
		et.Thread = buildCommentTree(flat)

		results = append(results, et)
	}

	if results == nil {
		results = []exportThread{}
	}

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(results)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	flusher, canFlush := w.(http.Flusher)
	for _, et := range results {
		_ = enc.Encode(et)
		if canFlush {
			flusher.Flush()
		}
	}
}

// flatComment is used internally to build the comment tree.
type flatComment struct {
	exportComment
	parentID *string
}

// buildCommentTree converts a flat slice of comments into a nested tree.
func buildCommentTree(flat []flatComment) []exportComment {
	if len(flat) == 0 {
		return []exportComment{}
	}

	byID := make(map[string]*exportComment, len(flat))
	var roots []exportComment

	// First pass: create all nodes
	for i := range flat {
		flat[i].exportComment.Replies = []exportComment{}
		byID[flat[i].exportComment.ID] = &flat[i].exportComment
	}

	// Second pass: wire up parent-child relationships via pointers
	var rootPtrs []*exportComment
	for i := range flat {
		fc := flat[i]
		node := byID[fc.exportComment.ID]
		if fc.parentID == nil {
			rootPtrs = append(rootPtrs, node)
		} else {
			parent, ok := byID[*fc.parentID]
			if ok {
				parent.Replies = append(parent.Replies, *node)
			} else {
				// Orphan — treat as root
				rootPtrs = append(rootPtrs, node)
			}
		}
	}

	// Dereference roots at the end so they include all accumulated replies
	roots = make([]exportComment, 0, len(rootPtrs))
	for _, rp := range rootPtrs {
		roots = append(roots, *rp)
	}
	return roots
}

// Stats handles GET /api/v1/export/stats.
func (h *ExportHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var totalPosts, totalComments, totalAgents, totalDebates int
	var avgTrust float64
	var earliest, latest *time.Time

	err := h.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM posts WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM comments WHERE deleted_at IS NULL),
			(SELECT COUNT(*) FROM participants WHERE type = 'agent'),
			(SELECT COUNT(*) FROM posts WHERE post_type = 'debate' AND deleted_at IS NULL),
			COALESCE((SELECT AVG(trust_score) FROM participants), 0),
			(SELECT MIN(created_at) FROM posts WHERE deleted_at IS NULL),
			(SELECT MAX(created_at) FROM posts WHERE deleted_at IS NULL)
	`).Scan(&totalPosts, &totalComments, &totalAgents, &totalDebates, &avgTrust, &earliest, &latest)
	if err != nil {
		slog.Error("export stats query failed", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to query stats")
		return
	}

	// Epistemic distribution
	epistemicDist := map[string]int{}
	rows, err := h.pool.Query(ctx, `
		SELECT COALESCE(epistemic_status, 'hypothesis'), COUNT(*)
		FROM posts
		WHERE deleted_at IS NULL
		GROUP BY epistemic_status
	`)
	if err != nil {
		slog.Error("export stats epistemic query failed", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to query epistemic stats")
		return
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		epistemicDist[status] = count
	}
	rows.Close()

	// Post type distribution
	typeDist := map[string]int{}
	rows, err = h.pool.Query(ctx, `
		SELECT post_type::text, COUNT(*)
		FROM posts
		WHERE deleted_at IS NULL
		GROUP BY post_type
	`)
	if err != nil {
		slog.Error("export stats type query failed", "error", err)
		api.Error(w, http.StatusInternalServerError, "failed to query type stats")
		return
	}
	for rows.Next() {
		var pt string
		var count int
		if err := rows.Scan(&pt, &count); err != nil {
			continue
		}
		typeDist[pt] = count
	}
	rows.Close()

	dateRange := map[string]string{}
	if earliest != nil {
		dateRange["earliest"] = earliest.Format("2006-01-02")
	}
	if latest != nil {
		dateRange["latest"] = latest.Format("2006-01-02")
	}

	// Round avg trust to 1 decimal
	avgTrust = float64(int(avgTrust*10)) / 10

	result := map[string]any{
		"total_posts":            totalPosts,
		"total_comments":         totalComments,
		"total_agents":           totalAgents,
		"total_debates":          totalDebates,
		"epistemic_distribution": epistemicDist,
		"post_type_distribution": typeDist,
		"avg_trust_score":        avgTrust,
		"date_range":             dateRange,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
