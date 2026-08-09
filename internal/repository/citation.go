package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Citation represents a citation between two posts.
type Citation struct {
	ID           string    `json:"id"`
	SourcePostID string    `json:"source_post_id"`
	CitedPostID  string    `json:"cited_post_id"`
	CitationType string    `json:"citation_type"`
	CreatedAt    time.Time `json:"created_at"`
}

// CitationNode represents a post node in the citation graph.
type CitationNode struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Type   string `json:"type"`
	Score  int    `json:"score"`
}

// CitationEdge represents a directed edge in the citation graph.
type CitationEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// CitationGraph is the combined nodes + edges for visualization.
type CitationGraph struct {
	Nodes []CitationNode `json:"nodes"`
	Edges []CitationEdge `json:"edges"`
}

// CitationRepo handles database operations for citations.
type CitationRepo struct {
	pool *pgxpool.Pool
}

// NewCitationRepo creates a new CitationRepo.
func NewCitationRepo(pool *pgxpool.Pool) *CitationRepo {
	return &CitationRepo{pool: pool}
}

// Create inserts a new citation. citation_type must be one of: references, supports, contradicts, extends, quotes.
func (r *CitationRepo) Create(ctx context.Context, sourcePostID, citedPostID, citationType string) error {
	validTypes := map[string]bool{
		"references":  true,
		"supports":    true,
		"contradicts": true,
		"extends":     true,
		"quotes":      true,
	}
	if !validTypes[citationType] {
		return fmt.Errorf("invalid citation_type: %s", citationType)
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO citations (source_post_id, cited_post_id, citation_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (source_post_id, cited_post_id) DO UPDATE SET citation_type = $3`,
		sourcePostID, citedPostID, citationType,
	)
	if err != nil {
		return fmt.Errorf("insert citation: %w", err)
	}
	return nil
}

// GetByPost returns all citations for a post (both as source and cited).
func (r *CitationRepo) GetByPost(ctx context.Context, postID string) ([]Citation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, source_post_id, cited_post_id, citation_type, created_at
		FROM citations
		WHERE source_post_id = $1 OR cited_post_id = $1
		ORDER BY created_at DESC`,
		postID,
	)
	if err != nil {
		return nil, fmt.Errorf("list citations: %w", err)
	}
	defer rows.Close()

	var citations []Citation
	for rows.Next() {
		var c Citation
		if err := rows.Scan(&c.ID, &c.SourcePostID, &c.CitedPostID, &c.CitationType, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan citation: %w", err)
		}
		citations = append(citations, c)
	}
	return citations, rows.Err()
}

// GetGraph returns the citation graph for a post, traversing up to `depth` levels.
// Uses iterative BFS on the relational citations table.
func (r *CitationRepo) GetGraph(ctx context.Context, postID string, depth int) (*CitationGraph, error) {
	if depth < 1 {
		depth = 2
	}
	if depth > 5 {
		depth = 5
	}

	graph := &CitationGraph{
		Nodes: []CitationNode{},
		Edges: []CitationEdge{},
	}

	// BFS: collect all post IDs within `depth` hops
	visited := map[string]bool{}
	frontier := []string{postID}
	visited[postID] = true

	var allEdges []CitationEdge

	for d := 0; d < depth && len(frontier) > 0; d++ {
		// Query all citations involving the frontier posts
		rows, err := r.pool.Query(ctx, `
			SELECT source_post_id, cited_post_id, citation_type
			FROM citations
			WHERE source_post_id = ANY($1) OR cited_post_id = ANY($1)`,
			frontier,
		)
		if err != nil {
			return nil, fmt.Errorf("graph BFS depth %d: %w", d, err)
		}

		var nextFrontier []string
		for rows.Next() {
			var src, dst, ctype string
			if err := rows.Scan(&src, &dst, &ctype); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan graph edge: %w", err)
			}
			allEdges = append(allEdges, CitationEdge{Source: src, Target: dst, Type: ctype})
			if !visited[src] {
				visited[src] = true
				nextFrontier = append(nextFrontier, src)
			}
			if !visited[dst] {
				visited[dst] = true
				nextFrontier = append(nextFrontier, dst)
			}
		}
		rows.Close()
		frontier = nextFrontier
	}

	if len(visited) == 0 {
		return graph, nil
	}

	// Deduplicate edges
	edgeSet := map[string]CitationEdge{}
	for _, e := range allEdges {
		key := e.Source + "→" + e.Target
		edgeSet[key] = e
	}
	for _, e := range edgeSet {
		graph.Edges = append(graph.Edges, e)
	}

	// Fetch node metadata for all visited post IDs
	ids := make([]string, 0, len(visited))
	for id := range visited {
		ids = append(ids, id)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.title, part.display_name, p.post_type, p.vote_score
		FROM posts p
		JOIN participants part ON part.id = p.author_id
		WHERE p.id = ANY($1)`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch graph nodes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var n CitationNode
		if err := rows.Scan(&n.ID, &n.Title, &n.Author, &n.Type, &n.Score); err != nil {
			return nil, fmt.Errorf("scan graph node: %w", err)
		}
		graph.Nodes = append(graph.Nodes, n)
	}

	return graph, rows.Err()
}
