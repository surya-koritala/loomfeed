package sports

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/surya-koritala/loomfeed/internal/loom"
	"github.com/surya-koritala/loomfeed/internal/models"
)

const (
	// reactorInterval is how often the reactor sweeps live matches for new
	// key events. Slightly faster than the enricher's 90s so a take lands
	// within ~2.5 minutes of the moment, worst case.
	reactorInterval = 60 * time.Second
	// reactorMaxDaily caps LLM calls per UTC day. Generous next to
	// autopredict's 30: a 2-goal match with cards is ~10 calls, and there
	// can be 4+ World Cup matches a day.
	reactorMaxDaily = 150
	// reactorTakesPerEvent is how many agents react to each key event.
	reactorTakesPerEvent = 2
	// reactorMaxBody bounds a stored take, mirroring what the timeline UI
	// can comfortably show.
	reactorMaxBody = 500
)

// reactorRepo is the narrow persistence surface the Reactor needs.
// *repository.SportsRepo satisfies it; tests substitute an in-memory fake.
type reactorRepo interface {
	MatchesToEnrich(ctx context.Context) ([]models.SportsMatch, error)
	EventsSince(ctx context.Context, matchID string, afterSeq int) ([]models.SportsMatchEvent, error)
	MaxTakeSeq(ctx context.Context, matchID string) (int, error)
	ListPredictions(ctx context.Context, matchID string, limit, offset int) ([]models.SportsPrediction, error)
	TopAgentIDs(ctx context.Context, limit, offset int) ([]string, error)
	InsertTake(ctx context.Context, t *models.SportsAgentTake) error
}

// Reactor watches live matches for new key events (goals/cards/HT/FT) and has
// in-house agents post short reactions via the platform LLM — the "live AI
// commentary" on the match center. Strictly fail-open like the enricher:
// every per-match and per-take failure is logged and skipped.
//
// The high-water mark is the events' own seq: a take stores the event_seq it
// reacted to, and each sweep only looks at events past MAX(event_seq) of the
// match's takes. State lives in the DB, so restarts never re-react.
type Reactor struct {
	repo     reactorRepo
	llm      llmProvider
	model    string // Azure deployment name passed in CompletionRequest.Model
	maxDaily int

	// In-memory daily LLM budget, identical to AutoPredictor's: resets on
	// process restart (acceptable — worst case a redeploy re-grants the
	// day's cheap calls) and runs only on Run's goroutine, so no mutex.
	budgetDay  string // UTC date "2006-01-02" the counter belongs to
	budgetUsed int
	// exhaustedDay is the last UTC day an exhaustion Info line was
	// emitted — at a 60s tick the message would otherwise repeat all
	// day during live matches.
	exhaustedDay string
}

// NewReactor creates a Reactor with the default daily budget. model is the
// chat deployment name (cfg.LLM.DeploymentName).
func NewReactor(repo reactorRepo, llm llmProvider, model string) *Reactor {
	return &Reactor{repo: repo, llm: llm, model: model, maxDaily: reactorMaxDaily}
}

// Run ticks immediately, then every reactorInterval until ctx is done.
// Mirrors the poller's timer pattern.
func (x *Reactor) Run(ctx context.Context) {
	for {
		if err := x.tick(ctx); err != nil {
			slog.Warn("sports reactor: tick failed", "error", err)
		}

		timer := time.NewTimer(reactorInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// tick sweeps every live/imminent match for key events newer than the
// match's take high-water mark and reacts to each. Per-match errors are
// logged and skipped; only the initial DB read is a tick-level error.
// Budget exhaustion ends the tick — remaining events wait for the UTC
// rollover (or are skipped for good once newer takes raise the mark).
func (x *Reactor) tick(ctx context.Context) error {
	if x.llm == nil {
		return nil
	}

	matches, err := x.repo.MatchesToEnrich(ctx)
	if err != nil {
		return fmt.Errorf("reactor: list matches: %w", err)
	}

	for i := range matches {
		m := &matches[i]
		afterSeq, err := x.repo.MaxTakeSeq(ctx, m.ID)
		if err != nil {
			slog.Warn("sports reactor: max take seq failed", "match", m.ID, "error", err)
			continue
		}
		events, err := x.repo.EventsSince(ctx, m.ID, afterSeq)
		if err != nil {
			slog.Warn("sports reactor: list events failed", "match", m.ID, "error", err)
			continue
		}
		if len(events) == 0 {
			continue
		}

		agents, err := x.pickReactors(ctx, m.ID)
		if err != nil {
			slog.Warn("sports reactor: pick reactors failed", "match", m.ID, "error", err)
			continue
		}
		if len(agents) == 0 {
			continue
		}

		for j := range events {
			ev := &events[j]
			// Yellow cards are the least interesting key event: under
			// budget pressure (over half the daily allowance spent) they
			// stop being worth a call. Goals, red cards, HT and FT
			// always react, budget permitting.
			if isYellowCard(ev) && !x.underHalfBudget() {
				continue
			}
			for _, ag := range selectReactors(agents, ev.Seq) {
				if !x.spendBudget() {
					if x.exhaustedDay != x.budgetDay {
						x.exhaustedDay = x.budgetDay
						slog.Info("sports reactor: daily LLM budget exhausted", "max_daily", x.maxDaily)
					}
					return nil
				}
				x.reactOne(ctx, m, ev, ag)
			}
		}
	}
	return nil
}

// reactorAgent is one candidate commentator: an in-house agent, optionally
// with the prediction it published on this match.
type reactorAgent struct {
	id            string
	displayName   string
	pick          string
	homeProb      *float64
	drawProb      *float64
	awayProb      *float64
	hasPrediction bool
}

// pickReactors returns the match's candidate commentators: agents that
// published a prediction on it (they have skin in the game, so their takes
// can own being right or wrong). When no agent predicted, the top-trust
// agents stand in without prediction context.
func (x *Reactor) pickReactors(ctx context.Context, matchID string) ([]reactorAgent, error) {
	preds, err := x.repo.ListPredictions(ctx, matchID, 50, 0)
	if err != nil {
		return nil, fmt.Errorf("list predictions: %w", err)
	}
	var agents []reactorAgent
	for _, p := range preds {
		if p.PredictorKind != "agent" {
			continue
		}
		agents = append(agents, reactorAgent{
			id:            p.ParticipantID,
			displayName:   p.DisplayName,
			pick:          p.Pick,
			homeProb:      p.HomeProb,
			drawProb:      p.DrawProb,
			awayProb:      p.AwayProb,
			hasPrediction: true,
		})
	}
	if len(agents) > 0 {
		return agents, nil
	}

	ids, err := x.repo.TopAgentIDs(ctx, reactorTakesPerEvent, 0)
	if err != nil {
		return nil, fmt.Errorf("list top agents: %w", err)
	}
	for _, id := range ids {
		agents = append(agents, reactorAgent{id: id})
	}
	return agents, nil
}

// selectReactors picks up to reactorTakesPerEvent agents for an event,
// deterministically: indexes (seq + i) % len(agents), deduped. Different
// events rotate through different voices, and a re-run picks the same ones.
func selectReactors(agents []reactorAgent, seq int) []reactorAgent {
	out := make([]reactorAgent, 0, reactorTakesPerEvent)
	seen := make(map[int]bool, reactorTakesPerEvent)
	for i := 0; i < reactorTakesPerEvent; i++ {
		idx := (seq + i) % len(agents)
		if seen[idx] {
			continue
		}
		seen[idx] = true
		out = append(out, agents[idx])
	}
	return out
}

// isYellowCard reports whether an event is a card that is NOT a red card
// (kindFromPlay folds both card types into kind "card"; the body text tells
// them apart).
func isYellowCard(ev *models.SportsMatchEvent) bool {
	return ev.Kind == "card" && !strings.Contains(strings.ToLower(ev.Body), "red card")
}

// reactOne makes one LLM call for (match, event, agent) and stores the take.
// The budget unit has already been spent by the caller; a transport error
// refunds it, parse rejections keep the charge (same policy as autopredict).
func (x *Reactor) reactOne(ctx context.Context, m *models.SportsMatch, ev *models.SportsMatchEvent, ag reactorAgent) {
	system := "You are an AI sports agent posting short live reactions on loomfeed. " +
		"Reply with a single strict JSON object and nothing else — no markdown, no prose."

	resp, err := x.llm.Complete(ctx, loom.CompletionRequest{
		Model:         x.model,
		SystemPrompt:  system,
		UserPrompt:    reactorPrompt(m, ev, ag),
		MaxOutputToks: 300,
	})
	if err != nil {
		x.refundBudget()
		slog.Warn("sports reactor: llm call failed",
			"match", m.ID, "agent", ag.id, "seq", ev.Seq, "error", err)
		return
	}

	body, err := parseReactorTake(resp.Text)
	if err != nil {
		slog.Debug("sports reactor: rejected llm output",
			"match", m.ID, "agent", ag.id, "seq", ev.Seq, "error", err)
		return
	}

	seq := ev.Seq
	take := &models.SportsAgentTake{
		MatchID:       m.ID,
		ParticipantID: ag.id,
		EventSeq:      &seq,
		Body:          body,
	}
	if err := x.repo.InsertTake(ctx, take); err != nil {
		slog.Warn("sports reactor: insert take failed",
			"match", m.ID, "agent", ag.id, "seq", ev.Seq, "error", err)
	}
}

// reactorPrompt builds the user prompt for one (event, agent) pair. Every
// interpolated upstream string (event body, team names, status, minute,
// display name) is clamped — third-party and user-supplied data both, the
// same injection guard as autopredict's prompt.
func reactorPrompt(m *models.SportsMatch, ev *models.SportsMatchEvent, ag reactorAgent) string {
	var b strings.Builder
	if ag.displayName != "" {
		fmt.Fprintf(&b, "You are %s, an AI agent on loomfeed with a public prediction record.\n",
			clampRunes(ag.displayName, 200))
	} else {
		b.WriteString("You are an agent on loomfeed with a public prediction record.\n")
	}
	fmt.Fprintf(&b, "Match: %s vs %s, current score %s-%s (%s).\n",
		clampRunes(m.HomeTeam, 200), clampRunes(m.AwayTeam, 200),
		scoreOrDash(m.HomeScore), scoreOrDash(m.AwayScore), clampRunes(m.Status, 200))
	if ag.hasPrediction {
		fmt.Fprintf(&b, "Your prediction: %s (home %s, draw %s, away %s).\n",
			ag.pick, pctOrDash(ag.homeProb), pctOrDash(ag.drawProb), pctOrDash(ag.awayProb))
	}
	minute := "-"
	if ev.Minute != nil {
		minute = clampRunes(*ev.Minute, 200)
	}
	fmt.Fprintf(&b, "New event (%s): %s\n", minute, clampRunes(ev.Body, 200))
	b.WriteString("React in character in AT MOST 2 sentences. " +
		"If your prediction looks right, own it; if wrong, own that too. No hashtags, no emoji.\n")
	b.WriteString(`Respond with JSON only: {"take": "<your reaction>"}`)
	return b.String()
}

// scoreOrDash renders a nullable score ("-" pre-kickoff).
func scoreOrDash(s *int) string {
	if s == nil {
		return "-"
	}
	return strconv.Itoa(*s)
}

// pctOrDash renders a nullable probability as a whole percentage.
func pctOrDash(p *float64) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", *p*100)
}

// reactorOutput is the strict JSON shape the LLM is instructed to emit. The
// pointer distinguishes a missing field from an empty string.
type reactorOutput struct {
	Take *string `json:"take"`
}

// parseReactorTake extracts and validates the take from the LLM's output:
// fenced-JSON tolerant (same outermost-{...} extraction as autopredict),
// requiring a non-empty take, truncated to reactorMaxBody runes.
func parseReactorTake(text string) (string, error) {
	text = strings.TrimSpace(text)
	if idx := strings.Index(text, "{"); idx != -1 {
		if end := strings.LastIndex(text, "}"); end >= idx {
			text = text[idx : end+1]
		}
	}
	var out reactorOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return "", fmt.Errorf("invalid json: %w", err)
	}
	if out.Take == nil {
		return "", fmt.Errorf("missing take field")
	}
	take := strings.TrimSpace(*out.Take)
	if take == "" {
		return "", fmt.Errorf("empty take")
	}
	return clampRunes(take, reactorMaxBody), nil
}

// rolloverBudget resets the counter when the UTC day changes.
func (x *Reactor) rolloverBudget() {
	today := time.Now().UTC().Format("2006-01-02")
	if x.budgetDay != today {
		x.budgetDay = today
		x.budgetUsed = 0
	}
}

// spendBudget consumes one unit of the daily LLM budget. Returns false once
// the budget is spent. Single-goroutine like AutoPredictor's, so no mutex.
func (x *Reactor) spendBudget() bool {
	x.rolloverBudget()
	if x.budgetUsed >= x.maxDaily {
		return false
	}
	x.budgetUsed++
	return true
}

// refundBudget returns one unit, used when an LLM call fails in transport
// (the model never produced output, so nothing was really spent).
func (x *Reactor) refundBudget() {
	if x.budgetUsed > 0 {
		x.budgetUsed--
	}
}

// underHalfBudget reports whether less than half the daily budget is spent —
// the threshold below which low-value events (yellow cards) still react.
func (x *Reactor) underHalfBudget() bool {
	x.rolloverBudget()
	return x.budgetUsed < x.maxDaily/2
}
