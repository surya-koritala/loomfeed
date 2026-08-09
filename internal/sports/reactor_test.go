package sports

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/loom"
	"github.com/RoamXAI/loomfeed/internal/models"
)

// fakeReactorLLM is a scripted llmProvider that also records every user
// prompt, so tests can assert what context each take was generated from.
// Call i returns responses[i] (the last entry repeats past the script);
// a non-nil errs[i] fails call i instead, simulating a transport error.
type fakeReactorLLM struct {
	calls     int
	prompts   []string
	responses []string
	errs      []error
}

func (f *fakeReactorLLM) Complete(_ context.Context, req loom.CompletionRequest) (*loom.CompletionResponse, error) {
	i := f.calls
	f.calls++
	f.prompts = append(f.prompts, req.UserPrompt)
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i >= len(f.responses) {
		i = len(f.responses) - 1
	}
	return &loom.CompletionResponse{Text: f.responses[i]}, nil
}

// eventsSinceCall records one EventsSince invocation for assertion.
type eventsSinceCall struct {
	matchID  string
	afterSeq int
}

// fakeReactorRepo is an in-memory reactorRepo. EventsSince mirrors the real
// query's semantics (seq > afterSeq, kind != "play", ascending) so the
// high-water-mark behaviour is exercised end to end.
type fakeReactorRepo struct {
	matches    []models.SportsMatch
	events     map[string][]models.SportsMatchEvent
	maxTakeSeq map[string]int
	preds      map[string][]models.SportsPrediction
	topAgents  []string

	inserted         []models.SportsAgentTake
	eventsSinceCalls []eventsSinceCall
	topAgentCalls    int
}

func (f *fakeReactorRepo) MatchesToEnrich(context.Context) ([]models.SportsMatch, error) {
	return f.matches, nil
}

func (f *fakeReactorRepo) EventsSince(_ context.Context, matchID string, afterSeq int) ([]models.SportsMatchEvent, error) {
	f.eventsSinceCalls = append(f.eventsSinceCalls, eventsSinceCall{matchID: matchID, afterSeq: afterSeq})
	var out []models.SportsMatchEvent
	for _, e := range f.events[matchID] {
		if e.Seq > afterSeq && e.Kind != "play" {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeReactorRepo) MaxTakeSeq(_ context.Context, matchID string) (int, error) {
	if seq, ok := f.maxTakeSeq[matchID]; ok {
		return seq, nil
	}
	return -1, nil
}

func (f *fakeReactorRepo) ListPredictions(_ context.Context, matchID string, _, _ int) ([]models.SportsPrediction, error) {
	return f.preds[matchID], nil
}

func (f *fakeReactorRepo) TopAgentIDs(context.Context, int, int) ([]string, error) {
	f.topAgentCalls++
	return f.topAgents, nil
}

func (f *fakeReactorRepo) InsertTake(_ context.Context, t *models.SportsAgentTake) error {
	f.inserted = append(f.inserted, *t)
	return nil
}

// reactorTestMatch is a live match with a current score, the shape the
// reactor sees mid-game.
func reactorTestMatch(id string) models.SportsMatch {
	h, a := 1, 0
	return models.SportsMatch{
		ID:        id,
		HomeTeam:  "Mexico",
		AwayTeam:  "Canada",
		Status:    "IN_PLAY",
		HomeScore: &h,
		AwayScore: &a,
	}
}

func agentPrediction(participantID, displayName, pick string, home, draw, away float64) models.SportsPrediction {
	return models.SportsPrediction{
		ParticipantID: participantID,
		PredictorKind: "agent",
		DisplayName:   displayName,
		Pick:          pick,
		HomeProb:      &home,
		DrawProb:      &draw,
		AwayProb:      &away,
	}
}

func TestReactorReactsToNewKeyEvents(t *testing.T) {
	m := reactorTestMatch("m1")
	min9, min45 := "9'", "45'"
	repo := &fakeReactorRepo{
		matches: []models.SportsMatch{m},
		events: map[string][]models.SportsMatchEvent{
			"m1": {
				{MatchID: "m1", Seq: 0, Kind: "play", Body: "Kickoff."},
				{MatchID: "m1", Seq: 7, Kind: "goal", Minute: &min9, Body: "Goal! Mexico 1, Canada 0."},
				{MatchID: "m1", Seq: 44, Kind: "ht", Minute: &min45, Body: "First Half ends."},
			},
		},
		preds: map[string][]models.SportsPrediction{
			"m1": {
				agentPrediction("agent-a", "Ada", "home", 0.5, 0.3, 0.2),
				agentPrediction("agent-b", "Bo", "away", 0.2, 0.3, 0.5),
				{ParticipantID: "human-1", PredictorKind: "human", Pick: "draw"},
			},
		},
	}
	llm := &fakeReactorLLM{responses: []string{
		`{"take": "take one"}`,
		`{"take": "take two"}`,
		`{"take": "take three"}`,
		`{"take": "take four"}`,
	}}

	x := NewReactor(repo, llm, "test-deploy")
	if err := x.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if llm.calls != 4 {
		t.Fatalf("expected 4 LLM calls (2 events x 2 takes), got %d", llm.calls)
	}
	if len(repo.inserted) != 4 {
		t.Fatalf("expected 4 takes inserted, got %d", len(repo.inserted))
	}

	wantSeqs := []int{7, 7, 44, 44}
	wantBodies := []string{"take one", "take two", "take three", "take four"}
	for i, take := range repo.inserted {
		if take.MatchID != "m1" {
			t.Errorf("take %d: expected match m1, got %q", i, take.MatchID)
		}
		if take.EventSeq == nil || *take.EventSeq != wantSeqs[i] {
			t.Errorf("take %d: expected event_seq %d, got %v", i, wantSeqs[i], take.EventSeq)
		}
		if take.Body != wantBodies[i] {
			t.Errorf("take %d: expected body %q, got %q", i, wantBodies[i], take.Body)
		}
	}

	// Deterministic selection: (seq + i) % len(agents). Event 7 → agents
	// [1, 0] = Bo then Ada; event 44 → [0, 1] = Ada then Bo.
	wantAgents := []string{"agent-b", "agent-a", "agent-a", "agent-b"}
	for i, take := range repo.inserted {
		if take.ParticipantID != wantAgents[i] {
			t.Errorf("take %d: expected participant %q, got %q", i, wantAgents[i], take.ParticipantID)
		}
	}

	// Each prompt carries the agent's own prediction line and the event body.
	wantPickLines := []string{
		"Your prediction: away (home 20%, draw 30%, away 50%).",
		"Your prediction: home (home 50%, draw 30%, away 20%).",
		"Your prediction: home (home 50%, draw 30%, away 20%).",
		"Your prediction: away (home 20%, draw 30%, away 50%).",
	}
	wantEventBodies := []string{
		"Goal! Mexico 1, Canada 0.", "Goal! Mexico 1, Canada 0.",
		"First Half ends.", "First Half ends.",
	}
	for i, p := range llm.prompts {
		if !strings.Contains(p, wantPickLines[i]) {
			t.Errorf("prompt %d: expected pick line %q, got:\n%s", i, wantPickLines[i], p)
		}
		if !strings.Contains(p, wantEventBodies[i]) {
			t.Errorf("prompt %d: expected event body %q, got:\n%s", i, wantEventBodies[i], p)
		}
		if !strings.Contains(p, "Mexico vs Canada, current score 1-0 (IN_PLAY)") {
			t.Errorf("prompt %d: expected match/score line, got:\n%s", i, p)
		}
	}
	if repo.topAgentCalls != 0 {
		t.Errorf("expected no TopAgentIDs fallback with agent predictors present, got %d calls", repo.topAgentCalls)
	}
}

func TestReactorHighWaterMark(t *testing.T) {
	m := reactorTestMatch("m1")
	repo := &fakeReactorRepo{
		matches: []models.SportsMatch{m},
		events: map[string][]models.SportsMatchEvent{
			"m1": {
				{MatchID: "m1", Seq: 7, Kind: "goal", Body: "Goal! Mexico 1, Canada 0."},
				{MatchID: "m1", Seq: 44, Kind: "ht", Body: "First Half ends."},
			},
		},
		maxTakeSeq: map[string]int{"m1": 7}, // goal already reacted to
		preds: map[string][]models.SportsPrediction{
			"m1": {
				agentPrediction("agent-a", "Ada", "home", 0.5, 0.3, 0.2),
				agentPrediction("agent-b", "Bo", "away", 0.2, 0.3, 0.5),
			},
		},
	}
	llm := &fakeReactorLLM{responses: []string{`{"take": "ht verdict"}`}}

	x := NewReactor(repo, llm, "test-deploy")
	if err := x.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(repo.eventsSinceCalls) != 1 {
		t.Fatalf("expected 1 EventsSince call, got %d", len(repo.eventsSinceCalls))
	}
	if got := repo.eventsSinceCalls[0]; got.matchID != "m1" || got.afterSeq != 7 {
		t.Errorf("expected EventsSince(m1, 7), got EventsSince(%s, %d)", got.matchID, got.afterSeq)
	}
	if llm.calls != 2 {
		t.Errorf("expected 2 LLM calls (only ht 44 is new), got %d", llm.calls)
	}
	if len(repo.inserted) != 2 {
		t.Fatalf("expected 2 takes, got %d", len(repo.inserted))
	}
	for i, take := range repo.inserted {
		if take.EventSeq == nil || *take.EventSeq != 44 {
			t.Errorf("take %d: expected event_seq 44, got %v", i, take.EventSeq)
		}
	}
}

func TestReactorBudgetExhausted(t *testing.T) {
	m := reactorTestMatch("m1")
	repo := &fakeReactorRepo{
		matches: []models.SportsMatch{m},
		events: map[string][]models.SportsMatchEvent{
			"m1": {
				{MatchID: "m1", Seq: 7, Kind: "goal", Body: "Goal!"},
				{MatchID: "m1", Seq: 44, Kind: "ht", Body: "First Half ends."},
			},
		},
		preds: map[string][]models.SportsPrediction{
			"m1": {
				agentPrediction("agent-a", "Ada", "home", 0.5, 0.3, 0.2),
				agentPrediction("agent-b", "Bo", "away", 0.2, 0.3, 0.5),
			},
		},
	}
	llm := &fakeReactorLLM{responses: []string{`{"take": "only take"}`}}

	x := NewReactor(repo, llm, "test-deploy")
	x.maxDaily = 1
	if err := x.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if llm.calls != 1 {
		t.Errorf("expected exactly 1 LLM call under maxDaily=1, got %d", llm.calls)
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("expected exactly 1 take under maxDaily=1, got %d", len(repo.inserted))
	}
	if repo.inserted[0].Body != "only take" {
		t.Errorf("expected body 'only take', got %q", repo.inserted[0].Body)
	}
}

func TestReactorYellowSkippedUnderPressure(t *testing.T) {
	m := reactorTestMatch("m1")
	repo := &fakeReactorRepo{
		matches: []models.SportsMatch{m},
		events: map[string][]models.SportsMatchEvent{
			"m1": {
				{MatchID: "m1", Seq: 10, Kind: "card", Body: "Yellow card for Johnson."},
				{MatchID: "m1", Seq: 12, Kind: "card", Body: "He is sent off with a red card."},
			},
		},
		preds: map[string][]models.SportsPrediction{
			"m1": {agentPrediction("agent-a", "Ada", "home", 0.5, 0.3, 0.2)},
		},
	}
	llm := &fakeReactorLLM{responses: []string{`{"take": "down to ten men"}`}}

	x := NewReactor(repo, llm, "test-deploy")
	// Past the half-budget mark (150/2 = 75): yellows are no longer worth a
	// call, red cards still are.
	x.budgetDay = time.Now().UTC().Format("2006-01-02")
	x.budgetUsed = 80
	if err := x.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if llm.calls != 1 {
		t.Fatalf("expected 1 LLM call (yellow skipped, red reacted), got %d", llm.calls)
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("expected 1 take (red card only), got %d", len(repo.inserted))
	}
	if repo.inserted[0].EventSeq == nil || *repo.inserted[0].EventSeq != 12 {
		t.Errorf("expected take on red-card seq 12, got %v", repo.inserted[0].EventSeq)
	}
	if !strings.Contains(llm.prompts[0], "red card") {
		t.Errorf("expected red-card event in the prompt, got:\n%s", llm.prompts[0])
	}
}

func TestReactorNoPredictors(t *testing.T) {
	m := reactorTestMatch("m1")
	repo := &fakeReactorRepo{
		matches: []models.SportsMatch{m},
		events: map[string][]models.SportsMatchEvent{
			"m1": {{MatchID: "m1", Seq: 7, Kind: "goal", Body: "Goal! Mexico 1, Canada 0."}},
		},
		preds: map[string][]models.SportsPrediction{
			"m1": {
				{ParticipantID: "human-1", PredictorKind: "human", Pick: "home"},
				{ParticipantID: "human-2", PredictorKind: "human", Pick: "away"},
			},
		},
		topAgents: []string{"agent-x", "agent-y"},
	}
	llm := &fakeReactorLLM{responses: []string{
		`{"take": "fallback one"}`,
		`{"take": "fallback two"}`,
	}}

	x := NewReactor(repo, llm, "test-deploy")
	if err := x.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if repo.topAgentCalls != 1 {
		t.Fatalf("expected TopAgentIDs fallback once, got %d calls", repo.topAgentCalls)
	}
	if len(repo.inserted) != 2 {
		t.Fatalf("expected 2 takes from top agents, got %d", len(repo.inserted))
	}
	got := map[string]bool{}
	for _, take := range repo.inserted {
		got[take.ParticipantID] = true
	}
	if !got["agent-x"] || !got["agent-y"] {
		t.Errorf("expected takes from agent-x and agent-y, got %v", got)
	}
	for i, p := range llm.prompts {
		if strings.Contains(p, "Your prediction") {
			t.Errorf("prompt %d: must not contain a prediction line without one, got:\n%s", i, p)
		}
		if !strings.Contains(p, "an agent on loomfeed") {
			t.Errorf("prompt %d: expected the anonymous persona line, got:\n%s", i, p)
		}
	}
}

func TestReactorParseFailureCharged(t *testing.T) {
	m := reactorTestMatch("m1")
	repo := &fakeReactorRepo{
		matches: []models.SportsMatch{m},
		events: map[string][]models.SportsMatchEvent{
			"m1": {{MatchID: "m1", Seq: 7, Kind: "goal", Body: "Goal!"}},
		},
		preds: map[string][]models.SportsPrediction{
			"m1": {agentPrediction("agent-a", "Ada", "home", 0.5, 0.3, 0.2)},
		},
	}
	llm := &fakeReactorLLM{responses: []string{`what a goal, no JSON here`}}

	x := NewReactor(repo, llm, "test-deploy")
	if err := x.tick(context.Background()); err != nil {
		t.Fatalf("tick must swallow parse failures, got: %v", err)
	}

	if llm.calls != 1 {
		t.Fatalf("expected 1 LLM call, got %d", llm.calls)
	}
	if x.budgetUsed != 1 {
		t.Errorf("parse failure must stay charged: expected budgetUsed 1, got %d", x.budgetUsed)
	}
	if len(repo.inserted) != 0 {
		t.Errorf("expected no takes from unparseable output, got %d", len(repo.inserted))
	}
}
