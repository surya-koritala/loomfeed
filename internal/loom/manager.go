package loom

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RoamXAI/loomfeed/internal/cache"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/safego"
)

// ErrRateLimited is returned by Summon when the requester has already
// hit their daily summon quota. The handler maps this to HTTP 429.
var ErrRateLimited = errors.New("loom: daily rate limit reached")

// ErrNoContent is returned when the requested post has no body to
// summarize / fact-check / etc. Better than spending tokens on a
// no-op, and gives the frontend a chance to render a friendly "this
// thread is empty" message.
var ErrNoContent = errors.New("loom: no content to summon against")

// LimitDailyAnon is the per-IP daily summon cap for logged-out
// callers. Generous enough nobody hits it on a normal "ask the looms"
// flow; cheap enough that one bot can't drain inference budget.
const LimitDailyAnon = 3

// LimitDailySignedIn is the per-participant daily cap for humans.
// Earned boosts (e.g. +5/day for an upvoted post) can stack on top,
// but they aren't implemented in v1.
const LimitDailySignedIn = 20

// LimitDailyAgent is lower than humans because agents tend to script
// retries; cap them tighter until we have signal that 10 isn't enough.
const LimitDailyAgent = 10

// Manager is the seam between the API layer and the Loom inference
// pipeline. It owns the persistence, cache, LLM client, the
// operator-chosen model (Azure deployment name), and the background
// worker that turns a pending summon row into a posted reply. One
// instance per process; lifetime tied to the API server.
type Manager struct {
	repo     *repository.LoomRepo
	comments *repository.CommentRepo
	posts    *repository.PostRepo
	cache    *cache.RedisCache
	client   Client
	// model is the Azure OpenAI deployment name passed through to the
	// client and recorded on loom_summons rows. Per-intent model
	// routing lands in v2 when fact-check / counter want different
	// tiers; v1 uses one model for everything.
	model string
}

// NewManager wires the Manager. Any of the dependencies may be nil at
// construction time; degraded paths (no cache, no LLM) are handled at
// the call sites where they apply. model is the Azure deployment
// name (e.g. "gpt-4o-mini").
func NewManager(
	loomRepo *repository.LoomRepo,
	comments *repository.CommentRepo,
	posts *repository.PostRepo,
	redis *cache.RedisCache,
	client Client,
	model string,
) *Manager {
	return &Manager{
		repo:     loomRepo,
		comments: comments,
		posts:    posts,
		cache:    redis,
		client:   client,
		model:    model,
	}
}

// SummonParams is everything the manager needs to start one summon.
// PostID is required (the content being asked about). CommentID is
// the comment that triggered the summon (the user's @loom-mentioning
// reply), kept for audit; it is NOT used as the parent of a Loom
// reply when PostCard is true.
//
// ParticipantID is nil for anonymous summons (the future logged-out
// "ask the looms" landing page). UserMessage is the body of the
// comment that @-mentioned Loom; we feed it to the classifier so the
// user can steer intent ("@loom tldr" vs "@loom what's this about").
//
// PostCard switches the worker output mode:
//   - true  → write the response to the summon row only; the post-
//             level Loom card surfaces it above the comments. No
//             child reply comment is created. THIS IS THE NEW DEFAULT
//             for v1.2+, used by the @loom-in-comment-body path.
//   - false → legacy "Loom is a thread participant" mode: the worker
//             posts a child reply comment authored by the Loom
//             participant. Kept for future intents where a direct
//             conversational reply is the right shape (e.g. @loom
//             on a specific comment to fact-check that comment, not
//             the post).
type SummonParams struct {
	ParticipantID *string
	PostID        string
	CommentID     *string
	UserMessage   string
	PostCard      bool
}

// Summon is the entry point invoked by the mention parser and (in v2)
// the explicit POST /api/v1/loom/summons handler. It does the
// synchronous bookkeeping — rate-limit check, content fetch, summon
// insert — then kicks off a goroutine that performs the LLM call.
//
// Returns the summon_id the caller can hand back to the frontend for
// polling. Errors flagged here are the ones the user can act on
// (rate-limited, empty content, missing dependencies); transport /
// LLM errors surface later via the summon row's state='error'.
func (m *Manager) Summon(ctx context.Context, p SummonParams) (string, error) {
	if m == nil || m.repo == nil || m.client == nil {
		return "", fmt.Errorf("loom: manager not initialised")
	}
	if p.PostID == "" {
		return "", fmt.Errorf("loom: PostID is required")
	}

	// Rate-limit check uses the summons table itself as the counter.
	// One source of truth (no Redis fan-out, no per-process state),
	// and the count survives deploys and restarts. The window is the
	// last 24h; the index in 000079 keeps this O(today's-summons).
	if p.ParticipantID != nil && *p.ParticipantID != "" {
		count, err := m.repo.CountRecentByParticipant(ctx, *p.ParticipantID, time.Now().Add(-24*time.Hour))
		if err != nil {
			// A failing rate-limit check is not a reason to drop the
			// summon. Worst case: a single user exceeds their quota by
			// one until the next request.
			count = 0
		}
		if count >= LimitDailySignedIn {
			return "", ErrRateLimited
		}
	}

	// Look up the post body — that's the content we're asking the LLM
	// about. If the post has no body, refuse early rather than spend
	// tokens on whitespace.
	post, err := m.posts.GetByID(ctx, p.PostID)
	if err != nil || post == nil {
		return "", fmt.Errorf("loom: post lookup: %w", err)
	}
	if post.Body == "" {
		return "", ErrNoContent
	}

	intent := Classify(p.UserMessage)

	// Build the prompt we'll feed to the LLM. We store the exact
	// string in loom_summons.prompt so the eval pipeline can later
	// replay summons deterministically.
	userPrompt := buildUserPrompt(intent, post.Title, post.Body)

	summonID, err := m.repo.CreateSummon(ctx, repository.CreateSummonParams{
		ParticipantID: p.ParticipantID,
		PostID:        &p.PostID,
		CommentID:     p.CommentID,
		Intent:        intent,
		Prompt:        userPrompt,
	})
	if err != nil {
		return "", fmt.Errorf("loom: create summon: %w", err)
	}

	// Hand off to the worker. 60s timeout covers the LLM call + the
	// reply-comment insert with margin. Errors inside the goroutine
	// land on the summon row, not the original request.
	postCard := p.PostCard
	parentForReply := p.CommentID
	safego.Run(ctx, "loom-summon", 90*time.Second, func(ctx context.Context) {
		m.processSummon(ctx, summonID, intent, p.PostID, parentForReply, userPrompt, postCard)
	})

	return summonID, nil
}

// buildUserPrompt assembles the message string the LLM sees as the
// user turn. The system prompt (set by dispatch) frames the role; the
// user prompt is just "here's the content, here's the ask."
func buildUserPrompt(intent models.LoomIntent, title, body string) string {
	switch intent {
	case models.LoomIntentSummarize:
		return fmt.Sprintf("Summarize this discussion post titled %q:\n\n%s", title, body)
	default:
		return body
	}
}
