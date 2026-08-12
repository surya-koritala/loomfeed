package sports

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/loom"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// fakeLLM is a scripted llmProvider: call i returns responses[i] (the last
// entry repeats when calls exceed the script). When errs[i] is non-nil, call
// i fails with that error instead, simulating a transport failure. It counts
// invocations so tests can assert how many LLM calls were spent.
type fakeLLM struct {
	mu        sync.Mutex
	calls     int
	responses []string
	errs      []error
}

func (f *fakeLLM) Complete(_ context.Context, _ loom.CompletionRequest) (*loom.CompletionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i >= len(f.responses) {
		i = len(f.responses) - 1
	}
	return &loom.CompletionResponse{Text: f.responses[i]}, nil
}

func (f *fakeLLM) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func autoPredictCleanup(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	database.CleanupTables(t, pool,
		"predictions", "prediction_stats", "sports_matches",
		"agent_identities", "human_users", "participants")
}

// createInHouseAgents seeds one human owner plus n agents with descending
// trust scores, returning the agent participant ids in trust order.
func createInHouseAgents(t *testing.T, pRepo *repository.ParticipantRepo, ctx context.Context, suffix string, n int) []string {
	t.Helper()
	owner, err := pRepo.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: "Owner " + suffix},
		Email:             fmt.Sprintf("autopredict-%s@example.com", suffix),
		PasswordHash:      "hashed_password",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("CreateHuman (%s): %v", suffix, err)
	}

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		agent, err := pRepo.CreateAgent(ctx, &models.AgentIdentity{
			Participant: models.Participant{
				DisplayName: fmt.Sprintf("Agent %s %d", suffix, i),
				TrustScore:  0.9 - float64(i)*0.01,
			},
			OwnerID:           owner.ID,
			ModelProvider:     "openai",
			ModelName:         "gpt-4",
			MaxRPM:            60,
			ProtocolType:      models.ProtocolREST,
			HeartbeatInterval: 300,
			Capabilities:      []string{"read"},
		})
		if err != nil {
			t.Fatalf("CreateAgent (%s %d): %v", suffix, i, err)
		}
		ids = append(ids, agent.ID)
	}
	return ids
}

// autoPredictMatch builds an upcoming wc2026 match the auto-predictor must
// consider (TIMED, kickoff inside the 36h window).
func autoPredictMatch(extID int64, kickoff time.Time) *models.SportsMatch {
	return &models.SportsMatch{
		ExtID:       extID,
		Competition: "wc2026",
		Stage:       "GROUP_STAGE",
		GroupName:   "Group B",
		HomeTeam:    "Spain",
		HomeCode:    "ESP",
		AwayTeam:    "Japan",
		AwayCode:    "JPN",
		KickoffUTC:  kickoff,
		Status:      "TIMED",
		Venue:       "MetLife Stadium",
	}
}

func TestAutoPredictTick_InsertsPredictions(t *testing.T) {
	pool := database.TestPool(t)
	autoPredictCleanup(t, pool)

	ctx := context.Background()
	repo := repository.NewSportsRepo(pool)
	pRepo := repository.NewParticipantRepo(pool)

	agentIDs := createInHouseAgents(t, pRepo, ctx, "insert", 3)
	matchID, err := repo.UpsertMatch(ctx, autoPredictMatch(7001, time.Now().Add(24*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	// One response per call; each carries a unique reasoning marker so we
	// can pair stored rows back to the response that produced them
	// regardless of the daily agent rotation.
	want := map[string]struct {
		home, draw, away float64
		pick             string
	}{
		"r0 strong home form":  {0.5, 0.3, 0.2, "home"},
		"r1 evenly matched":    {0.2, 0.5, 0.3, "draw"},
		"r2 away side quality": {0.1, 0.2, 0.7, "away"},
	}
	fake := &fakeLLM{responses: []string{
		`{"home_prob":0.5,"draw_prob":0.3,"away_prob":0.2,"reasoning":"r0 strong home form"}`,
		`{"home_prob":0.2,"draw_prob":0.5,"away_prob":0.3,"reasoning":"r1 evenly matched"}`,
		`{"home_prob":0.1,"draw_prob":0.2,"away_prob":0.7,"reasoning":"r2 away side quality"}`,
	}}

	ap := NewAutoPredictor(repo, fake, "test-deploy")
	if err := ap.tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	preds, err := repo.ListPredictions(ctx, matchID, 50, 0)
	if err != nil {
		t.Fatalf("ListPredictions: %v", err)
	}
	if len(preds) != 3 {
		t.Fatalf("expected 3 predictions, got %d", len(preds))
	}

	seenAgents := map[string]bool{}
	for _, p := range preds {
		if p.PredictorKind != "agent" {
			t.Errorf("expected predictor_kind 'agent', got %q", p.PredictorKind)
		}
		seenAgents[p.ParticipantID] = true

		w, ok := want[p.Reasoning]
		if !ok {
			t.Errorf("unexpected reasoning %q", p.Reasoning)
			continue
		}
		if p.HomeProb == nil || math.Abs(*p.HomeProb-w.home) > 1e-6 {
			t.Errorf("reasoning %q: expected home_prob %v, got %v", p.Reasoning, w.home, p.HomeProb)
		}
		if p.DrawProb == nil || math.Abs(*p.DrawProb-w.draw) > 1e-6 {
			t.Errorf("reasoning %q: expected draw_prob %v, got %v", p.Reasoning, w.draw, p.DrawProb)
		}
		if p.AwayProb == nil || math.Abs(*p.AwayProb-w.away) > 1e-6 {
			t.Errorf("reasoning %q: expected away_prob %v, got %v", p.Reasoning, w.away, p.AwayProb)
		}
		if p.Pick != w.pick {
			t.Errorf("reasoning %q: expected pick %q, got %q", p.Reasoning, w.pick, p.Pick)
		}
	}
	for _, id := range agentIDs {
		if !seenAgents[id] {
			t.Errorf("agent %s has no prediction", id)
		}
	}
	if got := fake.callCount(); got != 3 {
		t.Errorf("expected 3 LLM calls, got %d", got)
	}
}

func TestAutoPredictTick_RespectsDailyBudget(t *testing.T) {
	pool := database.TestPool(t)
	autoPredictCleanup(t, pool)

	ctx := context.Background()
	repo := repository.NewSportsRepo(pool)
	pRepo := repository.NewParticipantRepo(pool)

	createInHouseAgents(t, pRepo, ctx, "budget", 3)
	matchID, err := repo.UpsertMatch(ctx, autoPredictMatch(7002, time.Now().Add(12*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	fake := &fakeLLM{responses: []string{
		`{"home_prob":0.4,"draw_prob":0.35,"away_prob":0.25,"reasoning":"budget call"}`,
	}}
	ap := NewAutoPredictor(repo, fake, "test-deploy")
	ap.maxDaily = 2

	if err := ap.tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	preds, err := repo.ListPredictions(ctx, matchID, 50, 0)
	if err != nil {
		t.Fatalf("ListPredictions: %v", err)
	}
	if len(preds) != 2 {
		t.Errorf("expected exactly 2 predictions under maxDaily=2, got %d", len(preds))
	}
	if got := fake.callCount(); got != 2 {
		t.Errorf("expected exactly 2 LLM calls under maxDaily=2, got %d", got)
	}
}

func TestAutoPredictTick_SkipsMalformedLLMOutput(t *testing.T) {
	pool := database.TestPool(t)
	autoPredictCleanup(t, pool)

	ctx := context.Background()
	repo := repository.NewSportsRepo(pool)
	pRepo := repository.NewParticipantRepo(pool)

	createInHouseAgents(t, pRepo, ctx, "malformed", 2)
	matchID, err := repo.UpsertMatch(ctx, autoPredictMatch(7003, time.Now().Add(6*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	fake := &fakeLLM{responses: []string{
		`I think the home team will probably win this one!`,
		`{"home_prob":0.6,"draw_prob":0.25,"away_prob":0.15,"reasoning":"clean output"}`,
	}}
	ap := NewAutoPredictor(repo, fake, "test-deploy")

	if err := ap.tick(ctx); err != nil {
		t.Fatalf("tick must not fail on malformed LLM output: %v", err)
	}

	preds, err := repo.ListPredictions(ctx, matchID, 50, 0)
	if err != nil {
		t.Fatalf("ListPredictions: %v", err)
	}
	if len(preds) != 1 {
		t.Fatalf("expected 1 prediction (malformed skipped), got %d", len(preds))
	}
	if preds[0].Reasoning != "clean output" {
		t.Errorf("expected the valid response stored, got reasoning %q", preds[0].Reasoning)
	}
	if preds[0].Pick != "home" {
		t.Errorf("expected pick 'home', got %q", preds[0].Pick)
	}
}

// TestAutoPredictTick_AcceptsFencedJSON covers the Azure quirk where the
// model wraps its JSON in markdown fences despite the "no markdown"
// instruction — the prediction must still be parsed and inserted.
func TestAutoPredictTick_AcceptsFencedJSON(t *testing.T) {
	pool := database.TestPool(t)
	autoPredictCleanup(t, pool)

	ctx := context.Background()
	repo := repository.NewSportsRepo(pool)
	pRepo := repository.NewParticipantRepo(pool)

	createInHouseAgents(t, pRepo, ctx, "fenced", 1)
	matchID, err := repo.UpsertMatch(ctx, autoPredictMatch(7005, time.Now().Add(6*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	fake := &fakeLLM{responses: []string{
		"```json\n{\"home_prob\":0.55,\"draw_prob\":0.25,\"away_prob\":0.20,\"reasoning\":\"fenced output\"}\n```",
	}}
	ap := NewAutoPredictor(repo, fake, "test-deploy")

	if err := ap.tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	preds, err := repo.ListPredictions(ctx, matchID, 50, 0)
	if err != nil {
		t.Fatalf("ListPredictions: %v", err)
	}
	if len(preds) != 1 {
		t.Fatalf("expected 1 prediction from fenced JSON, got %d", len(preds))
	}
	if preds[0].Reasoning != "fenced output" {
		t.Errorf("expected reasoning 'fenced output', got %q", preds[0].Reasoning)
	}
	if preds[0].Pick != "home" {
		t.Errorf("expected pick 'home', got %q", preds[0].Pick)
	}
	if preds[0].HomeProb == nil || math.Abs(*preds[0].HomeProb-0.55) > 1e-6 {
		t.Errorf("expected home_prob 0.55, got %v", preds[0].HomeProb)
	}
}

// TestAutoPredictTick_RefundsBudgetOnTransportError verifies that a Complete
// transport error refunds its budget unit. With maxDaily=2 and the first
// call failing in transport, the refund leaves room for two more calls — so
// 3 agents yield 3 LLM calls and 2 stored predictions. Without the refund,
// the third call would be blocked and only 1 prediction would land.
func TestAutoPredictTick_RefundsBudgetOnTransportError(t *testing.T) {
	pool := database.TestPool(t)
	autoPredictCleanup(t, pool)

	ctx := context.Background()
	repo := repository.NewSportsRepo(pool)
	pRepo := repository.NewParticipantRepo(pool)

	createInHouseAgents(t, pRepo, ctx, "refund", 3)
	matchID, err := repo.UpsertMatch(ctx, autoPredictMatch(7006, time.Now().Add(10*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}

	fake := &fakeLLM{
		errs: []error{errors.New("transport blip"), nil, nil},
		responses: []string{
			`{"home_prob":0.45,"draw_prob":0.30,"away_prob":0.25,"reasoning":"after refund"}`,
		},
	}
	ap := NewAutoPredictor(repo, fake, "test-deploy")
	ap.maxDaily = 2

	if err := ap.tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if got := fake.callCount(); got != 3 {
		t.Errorf("expected 3 LLM calls (failed call refunded), got %d", got)
	}
	preds, err := repo.ListPredictions(ctx, matchID, 50, 0)
	if err != nil {
		t.Fatalf("ListPredictions: %v", err)
	}
	if len(preds) != 2 {
		t.Fatalf("expected 2 predictions after a refunded transport error, got %d", len(preds))
	}
	for _, p := range preds {
		if p.Reasoning != "after refund" {
			t.Errorf("expected reasoning 'after refund', got %q", p.Reasoning)
		}
	}
}

func TestAutoPredictTick_SkipsMatchesWithEnoughPredictions(t *testing.T) {
	pool := database.TestPool(t)
	autoPredictCleanup(t, pool)

	ctx := context.Background()
	repo := repository.NewSportsRepo(pool)
	pRepo := repository.NewParticipantRepo(pool)

	// 9 agents; the first 8 already predicted, so the match is at the
	// per-match cap and the 9th agent must not trigger an LLM call.
	agentIDs := createInHouseAgents(t, pRepo, ctx, "full", 9)
	matchID, err := repo.UpsertMatch(ctx, autoPredictMatch(7004, time.Now().Add(20*time.Hour).UTC()))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}
	h, d, a := 0.4, 0.3, 0.3
	for i := 0; i < 8; i++ {
		err := repo.UpsertPrediction(ctx, &models.SportsPrediction{
			MatchID:       matchID,
			ParticipantID: agentIDs[i],
			PredictorKind: "agent",
			HomeProb:      &h,
			DrawProb:      &d,
			AwayProb:      &a,
			Pick:          "home",
			Reasoning:     "pre-existing",
		})
		if err != nil {
			t.Fatalf("seed prediction %d: %v", i, err)
		}
	}

	fake := &fakeLLM{responses: []string{
		`{"home_prob":0.4,"draw_prob":0.3,"away_prob":0.3,"reasoning":"should never be called"}`,
	}}
	ap := NewAutoPredictor(repo, fake, "test-deploy")

	if err := ap.tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if got := fake.callCount(); got != 0 {
		t.Errorf("expected 0 LLM calls for a match at the cap, got %d", got)
	}
	preds, err := repo.ListPredictions(ctx, matchID, 50, 0)
	if err != nil {
		t.Fatalf("ListPredictions: %v", err)
	}
	if len(preds) != 8 {
		t.Errorf("expected predictions to stay at 8, got %d", len(preds))
	}
}
