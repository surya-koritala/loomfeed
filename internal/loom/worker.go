package loom

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/surya-koritala/loomfeed/internal/metrics"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// processSummon is the body of the background worker. Called from
// safego.Run; one goroutine per summon. Responsible for:
//
//   1. Resolving the dispatch (model + system prompt + max tokens).
//   2. Checking the cache before paying for an LLM call.
//   3. Calling the LLM if necessary.
//   4. Writing the response back to the summon row, AND optionally
//      posting a Loom-authored reply comment (legacy mode).
//   5. Emitting metrics.
//
// `postCard` switches output shape:
//   - true  → finalize the summon row only. The post-level Loom
//             card surfaces the response above the comments. No
//             child comment is created. Default for the @loom-in-
//             post-comment path.
//   - false → legacy: also post a Loom-authored child comment.
//             Reserved for future per-comment intents.
//
// Every terminal path emits metrics. Anything that returns early
// without finalizing the summon row is a bug — the row would stay
// 'pending' forever and bloat the partial index.
func (m *Manager) processSummon(
	ctx context.Context,
	summonID string,
	intent models.LoomIntent,
	postID string,
	parentCommentID *string,
	userPrompt string,
	postCard bool,
) {
	start := time.Now()
	slog.Info("loom: summon worker start",
		"summon_id", summonID,
		"post_id", postID,
		"intent", string(intent),
		"post_card", postCard,
	)
	defer func() {
		slog.Info("loom: summon worker exit",
			"summon_id", summonID,
			"latency_ms", int(time.Since(start)/time.Millisecond),
		)
	}()

	spec, err := Dispatch(intent)
	if err != nil {
		m.finalizeError(ctx, summonID, "dispatch_error", err, intent, "", false, start)
		return
	}

	// Cache key scope = postID. Same post + same intent + same prompt
	// hash → cache hit, regardless of which user summoned it.
	if cached, _ := GetCached(ctx, m.cache, intent, postID, userPrompt); cached != nil {
		metrics.LoomCacheHits.Inc()
		m.finalizeSuccess(ctx, summonID, intent, postID, parentCommentID, *cached, true, start, postCard)
		return
	}
	metrics.LoomCacheMisses.Inc()

	// Cache miss → real LLM call. Bounded by the 90s timeout the
	// safego wrapper enforces; nothing here should ever take longer.
	resp, err := m.client.Complete(ctx, CompletionRequest{
		Model:         m.model,
		SystemPrompt:  spec.SystemPrompt,
		UserPrompt:    userPrompt,
		MaxOutputToks: spec.MaxOutputToks,
	})
	if err != nil {
		m.finalizeError(ctx, summonID, "llm_error", err, intent, m.model, false, start)
		return
	}

	cached := CachedResponse{
		Text:         resp.Text,
		Model:        m.model,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
	}

	// Cache the fresh response before finalizing so a near-
	// simultaneous retry sees the hit. Failure to cache is non-fatal.
	SetCached(ctx, m.cache, intent, postID, userPrompt, cached)

	m.finalizeSuccess(ctx, summonID, intent, postID, parentCommentID, cached, false, start, postCard)
}

// finalizeSuccess updates the summon row to state=done with the
// response payload. For legacy comment-reply summons (postCard=false)
// it also posts a Loom-authored child comment. Metrics fire on every
// terminal path.
func (m *Manager) finalizeSuccess(
	ctx context.Context,
	summonID string,
	intent models.LoomIntent,
	postID string,
	parentCommentID *string,
	cached CachedResponse,
	wasCached bool,
	start time.Time,
	postCard bool,
) {
	var replyCommentID *string
	if !postCard {
		reply, err := m.comments.CreateLoomReply(ctx, repository.CreateLoomReplyParams{
			PostID:          postID,
			ParentCommentID: parentCommentID,
			Body:            cached.Text,
			LoomSummonID:    summonID,
			LoomIntent:      string(intent),
		})
		if err != nil {
			m.finalizeError(ctx, summonID, "reply_insert_error", err, intent, cached.Model, wasCached, start)
			return
		}
		replyCommentID = &reply.ID
	}

	var costUSD float64
	if !wasCached {
		// Only charge inference cost on the miss path. Cache hits
		// preserve the token counts in the row for traceability but
		// cost_usd is 0 because we didn't issue a billable API call.
		costUSD = Cost(cached.Model, cached.InputTokens, cached.OutputTokens)
		metrics.LoomInferenceCostUSD.WithLabelValues(cached.Model).Add(costUSD)
	}
	latencyMs := int(time.Since(start) / time.Millisecond)

	err := m.repo.FinalizeSummon(ctx, summonID, repository.FinalizeParams{
		ReplyCommentID: replyCommentID,
		Response:       cached.Text,
		Model:          cached.Model,
		InputTokens:    cached.InputTokens,
		OutputTokens:   cached.OutputTokens,
		CostUSD:        costUSD,
		Cached:         wasCached,
		LatencyMs:      latencyMs,
	})
	if err != nil {
		// If this fires, the worker computed a real response but
		// couldn't persist it — the summon row stays 'pending' and
		// the frontend spinner sits forever. Common cause: ctx
		// cancelled by safego timeout firing concurrently with the
		// DB write.
		slog.Error("loom: finalize summon failed — row stays pending",
			"summon_id", summonID,
			"err", err,
			"ctx_err", ctx.Err(),
		)
	} else {
		slog.Info("loom: summon finalised",
			"summon_id", summonID,
			"cached", wasCached,
			"latency_ms", latencyMs,
			"output_tokens", cached.OutputTokens,
		)
	}

	metrics.LoomSummonsTotal.WithLabelValues(
		string(intent), cached.Model, "done", strconv.FormatBool(wasCached),
	).Inc()
	metrics.LoomSummonLatency.WithLabelValues(
		string(intent), strconv.FormatBool(wasCached),
	).Observe(time.Since(start).Seconds())
}

// finalizeError marks the summon row errored and emits the error
// metric. errorCode is a stable short string ("dispatch_error",
// "llm_error", "reply_insert_error") so the metric label has bounded
// cardinality; the actual error.Error() lands in slog only.
func (m *Manager) finalizeError(
	ctx context.Context,
	summonID, errorCode string,
	err error,
	intent models.LoomIntent,
	model string,
	wasCached bool,
	start time.Time,
) {
	slog.Error("loom: summon failed",
		"summon_id", summonID,
		"error_code", errorCode,
		"err", err,
	)
	latencyMs := int(time.Since(start) / time.Millisecond)
	if uerr := m.repo.MarkErrored(ctx, summonID, errorCode, latencyMs); uerr != nil {
		slog.Error("loom: mark errored failed", "summon_id", summonID, "err", uerr)
	}
	metrics.LoomSummonsTotal.WithLabelValues(
		string(intent), model, "error", strconv.FormatBool(wasCached),
	).Inc()
}
