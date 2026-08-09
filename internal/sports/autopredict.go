package sports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/RoamXAI/loomfeed/internal/loom"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

const (
	// autoPredictInterval is how often the auto-predictor sweeps for
	// upcoming matches that still need agent predictions.
	autoPredictInterval = 6 * time.Hour
	// autoPredictWindow is how far ahead of kickoff a match becomes a
	// candidate for auto-predictions.
	autoPredictWindow = 36 * time.Hour
	// autoPredictMaxPerMatch caps agent predictions per match so the
	// prediction strip stays a curated panel, not a wall.
	autoPredictMaxPerMatch = 8
	// autoPredictMaxDaily caps LLM calls per UTC day across all matches.
	autoPredictMaxDaily = 30
	// autoPredictAgentPool is how many top-trust agents are considered.
	autoPredictAgentPool = 50
	// autoPredictCompetition matches the slug the poller stamps on
	// upserted matches (see client.go).
	autoPredictCompetition = "wc2026"
)

// llmProvider is the minimal LLM surface the auto-predictor needs. It
// matches loom.Client's Complete method, so *loom.AzureOpenAIClient — the
// platform's own cfg.LLM-backed client (also used by the @loom path) —
// satisfies it directly.
type llmProvider interface {
	Complete(ctx context.Context, req loom.CompletionRequest) (*loom.CompletionResponse, error)
}

// AutoPredictor has in-house agents publish World Cup predictions so the
// sports pages have content from day one. Every 6h it finds matches kicking
// off within 36h that have fewer than 8 agent predictions and asks the
// platform LLM for one calibrated prediction per missing agent, within a
// daily call budget.
type AutoPredictor struct {
	repo     *repository.SportsRepo
	llm      llmProvider
	model    string // Azure deployment name passed in CompletionRequest.Model
	maxDaily int

	// In-memory daily LLM budget. Resets on process restart — acceptable:
	// the worst case is a redeploy re-granting up to maxDaily cheap calls
	// for that day. Single-goroutine by design: Run ticks sequentially,
	// so spend/refund need no mutex.
	budgetDay  string // UTC date "2006-01-02" the counter belongs to
	budgetUsed int
}

// NewAutoPredictor creates an AutoPredictor with the default daily budget.
// model is the chat deployment name (cfg.LLM.DeploymentName).
func NewAutoPredictor(repo *repository.SportsRepo, llm llmProvider, model string) *AutoPredictor {
	return &AutoPredictor{repo: repo, llm: llm, model: model, maxDaily: autoPredictMaxDaily}
}

// Run ticks immediately, then every autoPredictInterval until ctx is done.
// Mirrors the poller's timer pattern.
func (a *AutoPredictor) Run(ctx context.Context) {
	for {
		if err := a.tick(ctx); err != nil {
			slog.Warn("sports autopredict: tick failed", "error", err)
		}

		timer := time.NewTimer(autoPredictInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// tick finds candidate matches (kicking off within autoPredictWindow, status
// SCHEDULED/TIMED, under the per-match agent cap) and fills them with agent
// predictions. Per-prediction failures (bad LLM output, kickoff race) are
// logged and skipped; only infrastructure failures bubble up.
func (a *AutoPredictor) tick(ctx context.Context) error {
	if a.llm == nil {
		return nil
	}

	agents, err := a.repo.TopAgentIDs(ctx, autoPredictAgentPool, 0)
	if err != nil {
		return fmt.Errorf("autopredict: list agents: %w", err)
	}
	if len(agents) == 0 {
		return nil
	}

	// Deterministic daily rotation: shift the trust-ordered list by the
	// day-of-year so different personas lead on different days.
	off := time.Now().UTC().YearDay() % len(agents)
	rotated := append(append([]string{}, agents[off:]...), agents[:off]...)

	matches, err := a.repo.ListMatches(ctx, autoPredictCompetition, "", "", "")
	if err != nil {
		return fmt.Errorf("autopredict: list matches: %w", err)
	}

	now := time.Now().UTC()
	inserted := 0
	for i := range matches {
		m := &matches[i]
		if m.Status != "SCHEDULED" && m.Status != "TIMED" {
			continue
		}
		if !m.KickoffUTC.After(now) || m.KickoffUTC.After(now.Add(autoPredictWindow)) {
			continue
		}

		preds, err := a.repo.ListPredictions(ctx, m.ID, 500, 0)
		if err != nil {
			slog.Warn("sports autopredict: list predictions failed", "match_id", m.ID, "error", err)
			continue
		}
		agentCount := 0
		predicted := make(map[string]bool, len(preds))
		for _, p := range preds {
			if p.PredictorKind == "agent" {
				agentCount++
				predicted[p.ParticipantID] = true
			}
		}
		if agentCount >= autoPredictMaxPerMatch {
			continue
		}

		for _, agentID := range rotated {
			if agentCount >= autoPredictMaxPerMatch {
				break
			}
			if predicted[agentID] {
				continue
			}
			if !a.spendBudget() {
				if inserted == 0 {
					// The whole budget was burned without a single
					// prediction landing — worth a louder signal than
					// the routine exhaustion notice.
					slog.Warn("sports autopredict: budget exhausted with zero successful inserts",
						"max_daily", a.maxDaily)
				} else {
					slog.Info("sports autopredict: daily LLM budget exhausted",
						"max_daily", a.maxDaily, "inserted", inserted)
				}
				return nil
			}
			if a.predictOne(ctx, m, agentID) {
				agentCount++
				inserted++
			}
		}
	}
	return nil
}

// spendBudget consumes one unit of the daily LLM budget, rolling the counter
// over when the UTC day changes. Returns false once the budget is spent.
func (a *AutoPredictor) spendBudget() bool {
	today := time.Now().UTC().Format("2006-01-02")
	if a.budgetDay != today {
		a.budgetDay = today
		a.budgetUsed = 0
	}
	if a.budgetUsed >= a.maxDaily {
		return false
	}
	a.budgetUsed++
	return true
}

// refundBudget returns one unit to the daily budget, used when an LLM call
// fails in transport (the model never produced output, so nothing was really
// spent). Like spendBudget it runs only on Run's single goroutine, so no
// mutex is needed.
func (a *AutoPredictor) refundBudget() {
	if a.budgetUsed > 0 {
		a.budgetUsed--
	}
}

// clampRunes truncates s to at most n runes (UTF-8 safe). Match fields come
// from an external API; clamping before prompt interpolation keeps both the
// prompt-injection surface and the prompt size bounded.
func clampRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// autoPredictOutput is the strict JSON shape the LLM is instructed to emit.
// Pointers distinguish "missing" from a literal 0.
type autoPredictOutput struct {
	HomeProb  *float64 `json:"home_prob"`
	DrawProb  *float64 `json:"draw_prob"`
	AwayProb  *float64 `json:"away_prob"`
	Reasoning string   `json:"reasoning"`
}

// predictOne makes one LLM call for (match, agent) and stores the result.
// Returns true only when a prediction row was written; all failures are
// logged at debug/warn and swallowed so one bad response can't stop the
// sweep. The LLM call has already been counted against the budget; a
// transport error (Complete itself failing) refunds the unit, while
// parse/validation rejections keep the charge.
func (a *AutoPredictor) predictOne(ctx context.Context, m *models.SportsMatch, agentID string) bool {
	system := "You are a football analyst producing calibrated win probabilities for FIFA World Cup matches. " +
		"Reply with a single strict JSON object and nothing else — no markdown, no prose."
	user := fmt.Sprintf(`Predict the outcome of this FIFA World Cup match.

Home team: %s
Away team: %s
Stage: %s
Group: %s
Kickoff (UTC): %s

Respond with EXACTLY this JSON shape and nothing else:
{"home_prob":0.45,"draw_prob":0.30,"away_prob":0.25,"reasoning":"<at most 2 sentences>"}

Rules:
- home_prob, draw_prob and away_prob are each between 0 and 1 and sum to 1.
- reasoning is at most 2 sentences and under 1000 characters.`,
		clampRunes(m.HomeTeam, 50), clampRunes(m.AwayTeam, 50),
		clampRunes(m.Stage, 50), clampRunes(m.GroupName, 50),
		m.KickoffUTC.UTC().Format(time.RFC3339))

	resp, err := a.llm.Complete(ctx, loom.CompletionRequest{
		Model:         a.model,
		SystemPrompt:  system,
		UserPrompt:    user,
		MaxOutputToks: 300,
	})
	if err != nil {
		// Transport error: the model never responded, so refund the
		// budget unit — an Azure blip shouldn't burn the daily allowance.
		// Parse/validation failures below still charge: the model did
		// respond, we just rejected its output.
		a.refundBudget()
		slog.Warn("sports autopredict: llm call failed", "match_id", m.ID, "agent_id", agentID, "error", err)
		return false
	}

	out, err := parseAutoPrediction(resp.Text)
	if err != nil {
		slog.Debug("sports autopredict: rejected llm output",
			"match_id", m.ID, "agent_id", agentID, "error", err)
		return false
	}

	pred := &models.SportsPrediction{
		MatchID:       m.ID,
		ParticipantID: agentID,
		PredictorKind: "agent",
		HomeProb:      out.HomeProb,
		DrawProb:      out.DrawProb,
		AwayProb:      out.AwayProb,
		Pick:          models.DeriveSportsPick(*out.HomeProb, *out.DrawProb, *out.AwayProb),
		Reasoning:     strings.TrimSpace(out.Reasoning),
	}
	if err := a.repo.UpsertPrediction(ctx, pred); err != nil {
		if errors.Is(err, repository.ErrPredictionLocked) {
			// Kickoff raced the sweep — expected near the window edge.
			slog.Debug("sports autopredict: kickoff passed", "match_id", m.ID, "agent_id", agentID)
		} else {
			slog.Warn("sports autopredict: upsert failed", "match_id", m.ID, "agent_id", agentID, "error", err)
		}
		return false
	}
	return true
}

// parseAutoPrediction strictly parses and validates the LLM's JSON output:
// all three probabilities present and in [0,1], summing to 1±0.01, with a
// non-empty reasoning of at most 1000 characters. Mirrors the validation the
// public prediction API applies to BYOK agents (handlers/sports.go).
func parseAutoPrediction(text string) (*autoPredictOutput, error) {
	text = strings.TrimSpace(text)
	// Azure intermittently wraps the JSON in markdown fences despite the
	// "no markdown" instruction. Extract the outermost {...} span before
	// parsing (same lenient approach as curatedshorts' parseDecision).
	if idx := strings.Index(text, "{"); idx != -1 {
		if end := strings.LastIndex(text, "}"); end >= idx {
			text = text[idx : end+1]
		}
	}
	var out autoPredictOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if out.HomeProb == nil || out.DrawProb == nil || out.AwayProb == nil {
		return nil, fmt.Errorf("missing probability field")
	}
	for _, p := range []float64{*out.HomeProb, *out.DrawProb, *out.AwayProb} {
		if p < 0 || p > 1 {
			return nil, fmt.Errorf("probability %v out of [0,1]", p)
		}
	}
	if sum := *out.HomeProb + *out.DrawProb + *out.AwayProb; sum < 0.99 || sum > 1.01 {
		return nil, fmt.Errorf("probabilities sum to %v, want 1±0.01", sum)
	}
	reasoning := strings.TrimSpace(out.Reasoning)
	if reasoning == "" {
		return nil, fmt.Errorf("empty reasoning")
	}
	if utf8.RuneCountInString(reasoning) > 1000 {
		return nil, fmt.Errorf("reasoning over 1000 characters")
	}
	return &out, nil
}
