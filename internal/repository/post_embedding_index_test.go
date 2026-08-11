package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/database"
)

func TestPostEmbeddingHNSWIndex(t *testing.T) {
	pool := database.TestPool(t)
	var definition string
	if err := pool.QueryRow(context.Background(), `
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND indexname = 'idx_posts_embedding_hnsw'`,
	).Scan(&definition); err != nil {
		t.Fatalf("load post embedding index: %v", err)
	}

	for _, want := range []string{"USING hnsw", "halfvec(3072)", "halfvec_cosine_ops", "m='16'", "ef_construction='64'"} {
		if !strings.Contains(definition, want) {
			t.Errorf("index definition missing %q: %s", want, definition)
		}
	}
}

func TestRelatedPostsQueryUsesHNSWIndex(t *testing.T) {
	pool := database.TestPool(t)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	sourceID := seedEmbeddingPlannerFixture(t, tx)
	if _, err := tx.Exec(context.Background(), "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatalf("disable sequential scans: %v", err)
	}
	if _, err := tx.Exec(context.Background(), "SET LOCAL enable_sort = off"); err != nil {
		t.Fatalf("discourage explicit sorts: %v", err)
	}

	rows, err := tx.Query(context.Background(), "EXPLAIN "+relatedPostsSQL,
		sourceID, 5)
	if err != nil {
		t.Fatalf("explain related-posts query: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if !strings.Contains(plan.String(), "idx_posts_embedding_hnsw") {
		t.Fatalf("related-posts query does not use HNSW index:\n%s", plan.String())
	}
}

func TestSemanticHybridSearchQueryUsesHNSWIndex(t *testing.T) {
	pool := database.TestPool(t)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	seedEmbeddingPlannerFixture(t, tx)
	if _, err := tx.Exec(context.Background(), "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatalf("disable sequential scans: %v", err)
	}
	if _, err := tx.Exec(context.Background(), "SET LOCAL enable_sort = off"); err != nil {
		t.Fatalf("discourage explicit sorts: %v", err)
	}

	repo := &HybridSearchRepo{hasTrgm: false}
	searchSQL, _ := repo.semanticHybridSearchSQL(SearchFilters{})
	queryEmbedding := make([]float32, PostEmbeddingDimensions)
	queryEmbedding[0] = 1
	rows, err := tx.Query(
		context.Background(),
		"EXPLAIN "+searchSQL,
		"semantic query",
		25,
		0,
		vectorLiteral(queryEmbedding),
		5, // Small K makes the ANN path cost-effective in the tiny test schema.
	)
	if err != nil {
		t.Fatalf("explain semantic hybrid search: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if !strings.Contains(plan.String(), "idx_posts_embedding_hnsw") {
		t.Fatalf("semantic hybrid search does not use HNSW index:\n%s", plan.String())
	}
}

// seedEmbeddingPlannerFixture gives PostgreSQL a representative table size.
// On an empty freshly migrated database, a scan through a general posts index
// is correctly cheaper than entering an HNSW graph, which makes an EXPLAIN
// assertion about production-scale planning meaningless and flaky.
func seedEmbeddingPlannerFixture(t *testing.T, tx pgx.Tx) string {
	t.Helper()
	ctx := context.Background()
	participantID := uuid.NewString()
	communityID := uuid.NewString()
	sourceID := uuid.NewString()
	embedding := make([]float32, PostEmbeddingDimensions)
	embedding[0] = 1

	if _, err := tx.Exec(ctx, `
		INSERT INTO participants (id, type, display_name)
		VALUES ($1, 'human', 'Embedding planner fixture')`, participantID); err != nil {
		t.Fatalf("insert planner participant: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO communities (id, name, slug, created_by)
		VALUES ($1, 'Embedding planner fixture', $2, $3)`,
		communityID, "embedding-planner-"+communityID, participantID); err != nil {
		t.Fatalf("insert planner community: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO posts (
			id, community_id, author_id, author_type, title, body, embedding
		)
		SELECT
			CASE WHEN n = 0 THEN $1::uuid ELSE uuid_generate_v4() END,
			$2::uuid,
			$3::uuid,
			'human',
			'Embedding planner post ' || n,
			'Planner fixture',
			$4::vector(3072)
		FROM generate_series(0, 511) AS fixture(n)`,
		sourceID, communityID, participantID, vectorLiteral(embedding)); err != nil {
		t.Fatalf("insert planner posts: %v", err)
	}
	if _, err := tx.Exec(ctx, "ANALYZE posts"); err != nil {
		t.Fatalf("analyze planner posts: %v", err)
	}
	return sourceID
}
