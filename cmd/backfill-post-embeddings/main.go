// backfill-post-embeddings walks posts that don't have an embedding
// yet and fills the column via Azure OpenAI's embeddings endpoint.
//
// Idempotent and resumable — `WHERE embedding IS NULL` is the only
// progress marker. Interrupt + re-run and it picks up where it left
// off. Skips deleted / retracted / quarantined posts (no point
// embedding content that won't surface in recommendations).
//
// Usage:
//
//	DATABASE_URL=postgres://...                  \
//	LLM_ENDPOINT=https://roamx-resource...       \
//	LLM_API_KEY=...                              \
//	LLM_EMBED_DEPLOYMENT=text-embedding-3-large  \
//	go run ./cmd/backfill-post-embeddings
//
// Flags:
//
//	--batch=32           how many posts to fetch + process per loop
//	--max-input-chars=8000  truncate body if longer (keeps tokens bounded)
//	--sleep-ms=50        small delay between embed calls (politeness +
//	                     Azure rate-limit headroom)
//	--limit=0            stop after this many posts (0 = no limit)
//	--dry-run            log what would happen, don't write embeddings
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/loom"
)

func main() {
	batch := flag.Int("batch", 32, "posts fetched per loop")
	maxInputChars := flag.Int("max-input-chars", 8000, "truncate post body to this many chars (rough token bound)")
	sleepMs := flag.Int("sleep-ms", 50, "delay between embed calls")
	limit := flag.Int("limit", 0, "stop after this many posts (0 = no limit)")
	dryRun := flag.Bool("dry-run", false, "log only, don't write embeddings")
	flag.Parse()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dbURL := must("DATABASE_URL")
	endpoint := must("LLM_ENDPOINT")
	apiKey := must("LLM_API_KEY")
	deployment := envWithDefault("LLM_EMBED_DEPLOYMENT", "text-embedding-3-large")

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		fail("connect db", err)
	}
	defer pool.Close()

	embedClient := loom.NewAzureEmbedClient(endpoint, apiKey, deployment)

	// Sanity check: count how many posts still need embedding.
	var todo int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM posts
		WHERE embedding IS NULL
		  AND deleted_at IS NULL
		  AND NOT is_retracted
		  AND NOT quarantined`,
	).Scan(&todo); err != nil {
		fail("count posts", err)
	}
	slog.Info("backfill plan",
		"posts_remaining", todo,
		"deployment", deployment,
		"batch", *batch,
		"sleep_ms", *sleepMs,
		"dry_run", *dryRun,
	)
	if todo == 0 {
		slog.Info("nothing to do")
		return
	}

	var processed, ok, failed int
	start := time.Now()

	for {
		if *limit > 0 && processed >= *limit {
			slog.Info("hit --limit, stopping", "processed", processed)
			break
		}

		// Pull a batch. Order by id for deterministic resumption.
		rows, err := pool.Query(ctx, `
			SELECT id, title, body
			FROM posts
			WHERE embedding IS NULL
			  AND deleted_at IS NULL
			  AND NOT is_retracted
			  AND NOT quarantined
			ORDER BY id
			LIMIT $1`, *batch,
		)
		if err != nil {
			fail("fetch batch", err)
		}
		type row struct {
			ID, Title, Body string
		}
		var batchRows []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.ID, &r.Title, &r.Body); err != nil {
				rows.Close()
				fail("scan row", err)
			}
			batchRows = append(batchRows, r)
		}
		rows.Close()

		if len(batchRows) == 0 {
			slog.Info("no more rows", "processed", processed, "ok", ok, "failed", failed)
			break
		}

		// Build the batch input array — title + body, truncated to
		// the per-input char cap so a single huge post can't blow up
		// the request.
		inputs := make([]string, len(batchRows))
		for i, r := range batchRows {
			input := r.Title + "\n\n" + r.Body
			if len(input) > *maxInputChars {
				input = input[:*maxInputChars]
			}
			inputs[i] = input
		}

		// One HTTP roundtrip per batch — the throughput lever that
		// makes the backfill complete in hours instead of a day.
		vecs, err := embedClient.EmbedBatch(ctx, inputs)
		if err != nil {
			slog.Warn("batch embed failed — skipping batch",
				"size", len(batchRows), "err", err)
			failed += len(batchRows)
			processed += len(batchRows)
			if *sleepMs > 0 {
				time.Sleep(time.Duration(*sleepMs*4) * time.Millisecond)
			}
			continue
		}

		for i, r := range batchRows {
			if !*dryRun {
				if err := writeEmbedding(ctx, pool, r.ID, vecs[i]); err != nil {
					slog.Warn("write failed",
						"post_id", r.ID, "err", err)
					failed++
					processed++
					continue
				}
			}
			ok++
			processed++
		}

		if *sleepMs > 0 {
			time.Sleep(time.Duration(*sleepMs) * time.Millisecond)
		}

		// Progress line every batch
		elapsed := time.Since(start)
		rate := float64(processed) / elapsed.Seconds()
		var eta time.Duration
		if rate > 0 {
			eta = time.Duration(float64(todo-int64(processed))/rate) * time.Second
		}
		slog.Info("progress",
			"processed", processed,
			"ok", ok,
			"failed", failed,
			"per_sec", fmt.Sprintf("%.1f", rate),
			"eta", eta.Truncate(time.Second),
		)
	}

	slog.Info("done",
		"processed", processed,
		"ok", ok,
		"failed", failed,
		"duration", time.Since(start).Truncate(time.Second),
	)
}

// writeEmbedding stores the vector by formatting it as pgvector's
// text representation: '[v1,v2,...]'::vector. Avoids pulling in
// pgvector-go for a one-call use site. The column is vector(3072);
// pgvector validates dimensionality on cast.
func writeEmbedding(ctx context.Context, pool *pgxpool.Pool, postID string, vec []float32) error {
	_, err := pool.Exec(ctx,
		`UPDATE posts SET embedding = $1::vector WHERE id = $2`,
		vectorText(vec), postID,
	)
	return err
}

// vectorText formats a slice as pgvector's text input: "[1.2,3.4,...]"
// with no spaces (pgvector accepts spaces but trimming them halves
// the wire size on a 3072-dim vector).
func vectorText(vec []float32) string {
	var b strings.Builder
	b.Grow(len(vec) * 12) // rough preallocation
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		// 'g' format gives the shortest faithful representation; 32-bit
		// precision matches what the model returned, no rounding.
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func must(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", name)
		os.Exit(1)
	}
	return v
}

func envWithDefault(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	return v
}

func fail(stage string, err error) {
	slog.Error(stage, "error", err)
	os.Exit(1)
}

