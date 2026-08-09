package handlers_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api/handlers"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/auth"
	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

// setupSportsTest builds the handler against the real test DB and returns the
// repos needed for fixtures.
func setupSportsTest(t *testing.T) (*handlers.SportsHandler, *repository.SportsRepo, *repository.ParticipantRepo) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"sports_predictions", "sports_prediction_stats", "sports_matches",
		"agent_identities", "human_users", "participants")
	sportsRepo := repository.NewSportsRepo(pool)
	participants := repository.NewParticipantRepo(pool)
	return handlers.NewSportsHandler(sportsRepo), sportsRepo, participants
}

// sportsHandlerMatch builds a minimal wc2026 match fixture.
func sportsHandlerMatch(extID int64, kickoff time.Time) *models.SportsMatch {
	return &models.SportsMatch{
		ExtID:       extID,
		Competition: "wc2026",
		Stage:       "GROUP_STAGE",
		GroupName:   "A",
		HomeTeam:    "Mexico",
		HomeCode:    "MEX",
		AwayTeam:    "Canada",
		AwayCode:    "CAN",
		KickoffUTC:  kickoff,
		Status:      "SCHEDULED",
		Venue:       "Estadio Azteca",
	}
}

func createSportsMatch(t *testing.T, repo *repository.SportsRepo, extID int64, kickoff time.Time) string {
	t.Helper()
	id, err := repo.UpsertMatch(context.Background(), sportsHandlerMatch(extID, kickoff))
	if err != nil {
		t.Fatalf("UpsertMatch: %v", err)
	}
	return id
}

// createSportsHuman creates a human participant for prediction tests.
func createSportsHuman(t *testing.T, participants *repository.ParticipantRepo, suffix string) *models.Participant {
	t.Helper()
	h := &models.HumanUser{
		Participant: models.Participant{
			DisplayName: "Sports Human " + suffix,
		},
		Email:             "sports-" + suffix + "@example.com",
		PasswordHash:      "hashed_password",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	}
	p, err := participants.CreateHuman(context.Background(), h)
	if err != nil {
		t.Fatalf("CreateHuman (%s): %v", suffix, err)
	}
	return p
}

// createSportsAgent creates an agent participant (owned by a fresh human).
func createSportsAgent(t *testing.T, participants *repository.ParticipantRepo, suffix string) *models.AgentIdentity {
	t.Helper()
	owner := createSportsHuman(t, participants, "owner-"+suffix)
	agent := &models.AgentIdentity{
		Participant: models.Participant{
			DisplayName: "Sports Agent " + suffix,
		},
		OwnerID:           owner.ID,
		ModelProvider:     "openai",
		ModelName:         "gpt-4",
		MaxRPM:            60,
		ProtocolType:      models.ProtocolREST,
		HeartbeatInterval: 300,
		Capabilities:      []string{"read"},
	}
	created, err := participants.CreateAgent(context.Background(), agent)
	if err != nil {
		t.Fatalf("CreateAgent (%s): %v", suffix, err)
	}
	return created
}

// withSportsClaims injects auth claims into the request context the same way
// middleware.APIKeyAuth (agents) and middleware.Auth (humans) do.
func withSportsClaims(req *http.Request, participantID, participantType string) *http.Request {
	claims := &auth.Claims{
		ParticipantID:   participantID,
		ParticipantType: participantType,
	}
	return req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
}

// postPrediction sends a POST /api/v1/sports/matches/{id}/predictions to the
// handler with claims injected and returns the recorder.
func postPrediction(t *testing.T, h *handlers.SportsHandler, matchID string, body any, participantID, participantType string) *httptest.ResponseRecorder {
	t.Helper()
	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/sports/matches/"+matchID+"/predictions", body)
	req.SetPathValue("id", matchID)
	req = withSportsClaims(req, participantID, participantType)
	rec := httptest.NewRecorder()
	h.CreatePrediction(rec, req)
	return rec
}

// listPredictions fetches the stored predictions for a match via the handler.
func listPredictions(t *testing.T, h *handlers.SportsHandler, matchID string) []models.SportsPrediction {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/matches/"+matchID+"/predictions", nil)
	req.SetPathValue("id", matchID)
	rec := httptest.NewRecorder()
	h.ListPredictions(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)
	var resp struct {
		Data []models.SportsPrediction `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	return resp.Data
}

// --- POST predictions: agent validation matrix ---

func TestSportsCreatePrediction_AgentValidProbs(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	agent := createSportsAgent(t, participants, "valid")
	matchID := createSportsMatch(t, sportsRepo, 4001, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{
		"home_prob": 0.55,
		"draw_prob": 0.25,
		"away_prob": 0.20,
		"reasoning": "home advantage at altitude",
		"pick":      "away", // client-sent pick must be ignored for agents
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Data models.SportsPrediction `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if resp.Data.Pick != "home" {
		t.Errorf("expected derived pick home (argmax), got %q", resp.Data.Pick)
	}
	if resp.Data.Reasoning != "home advantage at altitude" {
		t.Errorf("expected reasoning stored, got %q", resp.Data.Reasoning)
	}
	if resp.Data.PredictorKind != "agent" {
		t.Errorf("expected predictor_kind agent, got %q", resp.Data.PredictorKind)
	}
	// Columns are float4 (real); compare with tolerance.
	if resp.Data.HomeProb == nil || math.Abs(*resp.Data.HomeProb-0.55) > 1e-6 {
		t.Errorf("expected home_prob ~0.55, got %v", resp.Data.HomeProb)
	}

	preds := listPredictions(t, h, matchID)
	if len(preds) != 1 {
		t.Fatalf("expected 1 stored prediction, got %d", len(preds))
	}
	if preds[0].Pick != "home" {
		t.Errorf("expected stored pick home, got %q", preds[0].Pick)
	}
}

func TestSportsCreatePrediction_AgentMissingProb(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	agent := createSportsAgent(t, participants, "missing")
	matchID := createSportsMatch(t, sportsRepo, 4002, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{
		"home_prob": 0.6,
		"draw_prob": 0.4,
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSportsCreatePrediction_AgentBadSum(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	agent := createSportsAgent(t, participants, "badsum")
	matchID := createSportsMatch(t, sportsRepo, 4003, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{
		"home_prob": 0.4,
		"draw_prob": 0.2,
		"away_prob": 0.2, // sums to 0.8
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSportsCreatePrediction_AgentProbOutOfRange(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	agent := createSportsAgent(t, participants, "range")
	matchID := createSportsMatch(t, sportsRepo, 4004, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{
		"home_prob": -0.1,
		"draw_prob": 0.9,
		"away_prob": 0.2,
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusBadRequest)

	rec = postPrediction(t, h, matchID, map[string]any{
		"home_prob": 1.2,
		"draw_prob": -0.1,
		"away_prob": -0.1,
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSportsCreatePrediction_AgentReasoningTooLong(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	agent := createSportsAgent(t, participants, "longreason")
	matchID := createSportsMatch(t, sportsRepo, 4005, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{
		"home_prob": 0.5,
		"draw_prob": 0.3,
		"away_prob": 0.2,
		"reasoning": strings.Repeat("x", 1001),
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSportsCreatePrediction_BodyTooLarge(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	agent := createSportsAgent(t, participants, "bigbody")
	matchID := createSportsMatch(t, sportsRepo, 4017, time.Now().Add(24*time.Hour).UTC())

	// >8KB body trips the MaxBytesReader cap before decode completes.
	rec := postPrediction(t, h, matchID, map[string]any{
		"home_prob": 0.5,
		"draw_prob": 0.3,
		"away_prob": 0.2,
		"reasoning": strings.Repeat("x", 9*1024),
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSportsCreatePrediction_AgentArgmaxTiebreak(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	agent := createSportsAgent(t, participants, "tie")
	matchID := createSportsMatch(t, sportsRepo, 4006, time.Now().Add(24*time.Hour).UTC())

	// home and draw tie at 0.4 — tiebreak order home > draw > away.
	rec := postPrediction(t, h, matchID, map[string]any{
		"home_prob": 0.4,
		"draw_prob": 0.4,
		"away_prob": 0.2,
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Data models.SportsPrediction `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if resp.Data.Pick != "home" {
		t.Errorf("expected tiebreak pick home, got %q", resp.Data.Pick)
	}
}

// --- POST predictions: human path ---

func TestSportsCreatePrediction_HumanPick(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	human := createSportsHuman(t, participants, "pick")
	matchID := createSportsMatch(t, sportsRepo, 4007, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{"pick": "home"}, human.ID, "human")
	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Data models.SportsPrediction `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if resp.Data.Pick != "home" {
		t.Errorf("expected pick home, got %q", resp.Data.Pick)
	}
	if resp.Data.PredictorKind != "human" {
		t.Errorf("expected predictor_kind human, got %q", resp.Data.PredictorKind)
	}
}

func TestSportsCreatePrediction_HumanInvalidPick(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	human := createSportsHuman(t, participants, "banana")
	matchID := createSportsMatch(t, sportsRepo, 4008, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{"pick": "banana"}, human.ID, "human")
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSportsCreatePrediction_HumanProbsDiscarded(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	human := createSportsHuman(t, participants, "probs")
	matchID := createSportsMatch(t, sportsRepo, 4009, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{
		"pick":      "draw",
		"home_prob": 0.5,
		"draw_prob": 0.3,
		"away_prob": 0.2,
	}, human.ID, "human")
	testutil.AssertStatus(t, rec, http.StatusOK)

	preds := listPredictions(t, h, matchID)
	if len(preds) != 1 {
		t.Fatalf("expected 1 stored prediction, got %d", len(preds))
	}
	if preds[0].HomeProb != nil || preds[0].DrawProb != nil || preds[0].AwayProb != nil {
		t.Errorf("expected human probs stored NULL, got %v/%v/%v",
			preds[0].HomeProb, preds[0].DrawProb, preds[0].AwayProb)
	}
	if preds[0].Pick != "draw" {
		t.Errorf("expected pick draw, got %q", preds[0].Pick)
	}
}

// --- POST predictions: lock, not-found, upsert ---

func TestSportsCreatePrediction_MatchKickedOff(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	human := createSportsHuman(t, participants, "locked")
	matchID := createSportsMatch(t, sportsRepo, 4010, time.Now().Add(-1*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{"pick": "home"}, human.ID, "human")
	testutil.AssertStatus(t, rec, http.StatusConflict)

	var resp map[string]string
	testutil.DecodeResponse(t, rec, &resp)
	if resp["error"] != "prediction window closed" {
		t.Errorf("expected error %q, got %q", "prediction window closed", resp["error"])
	}
}

func TestSportsCreatePrediction_UnknownMatch(t *testing.T) {
	h, _, participants := setupSportsTest(t)
	human := createSportsHuman(t, participants, "unknown")

	rec := postPrediction(t, h, "00000000-0000-0000-0000-000000000001",
		map[string]any{"pick": "home"}, human.ID, "human")
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestSportsCreatePrediction_MalformedUUID(t *testing.T) {
	h, _, participants := setupSportsTest(t)
	human := createSportsHuman(t, participants, "malformed")

	rec := postPrediction(t, h, "not-a-uuid", map[string]any{"pick": "home"}, human.ID, "human")
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

func TestSportsCreatePrediction_RepeatUpdatesPreKickoff(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	agent := createSportsAgent(t, participants, "repeat")
	matchID := createSportsMatch(t, sportsRepo, 4011, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{
		"home_prob": 0.5, "draw_prob": 0.3, "away_prob": 0.2,
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusOK)

	rec = postPrediction(t, h, matchID, map[string]any{
		"home_prob": 0.1, "draw_prob": 0.2, "away_prob": 0.7,
		"reasoning": "injury news flipped it",
	}, agent.ID, "agent")
	testutil.AssertStatus(t, rec, http.StatusOK)

	preds := listPredictions(t, h, matchID)
	if len(preds) != 1 {
		t.Fatalf("expected 1 prediction after repeat post, got %d", len(preds))
	}
	if preds[0].Pick != "away" {
		t.Errorf("expected updated pick away, got %q", preds[0].Pick)
	}
	if preds[0].AwayProb == nil || math.Abs(*preds[0].AwayProb-0.7) > 1e-6 {
		t.Errorf("expected updated away_prob ~0.7, got %v", preds[0].AwayProb)
	}
	if preds[0].Reasoning != "injury news flipped it" {
		t.Errorf("expected updated reasoning, got %q", preds[0].Reasoning)
	}
}

func TestSportsCreatePrediction_Unauthenticated(t *testing.T) {
	h, sportsRepo, _ := setupSportsTest(t)
	matchID := createSportsMatch(t, sportsRepo, 4012, time.Now().Add(24*time.Hour).UTC())

	req := testutil.JSONRequest(t, http.MethodPost, "/api/v1/sports/matches/"+matchID+"/predictions",
		map[string]any{"pick": "home"})
	req.SetPathValue("id", matchID)
	rec := httptest.NewRecorder()
	h.CreatePrediction(rec, req)
	testutil.AssertStatus(t, rec, http.StatusUnauthorized)
}

// --- GET matches list ---

func TestSportsListMatches_DateFilter(t *testing.T) {
	h, sportsRepo, _ := setupSportsTest(t)

	day1 := time.Date(2026, 6, 15, 18, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 6, 16, 18, 0, 0, 0, time.UTC)
	id1 := createSportsMatch(t, sportsRepo, 4013, day1)
	createSportsMatch(t, sportsRepo, 4014, day2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/worldcup/matches?date=2026-06-15", nil)
	rec := httptest.NewRecorder()
	h.ListMatches(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Data []models.SportsMatch `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 match on 2026-06-15, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != id1 {
		t.Errorf("expected match %s, got %s", id1, resp.Data[0].ID)
	}
}

func TestSportsListMatches_InvalidDate(t *testing.T) {
	h, _, _ := setupSportsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/worldcup/matches?date=not-a-date", nil)
	rec := httptest.NewRecorder()
	h.ListMatches(rec, req)
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

// --- GET match detail ---

func TestSportsGetMatch_AnonWithAggregates(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	human := createSportsHuman(t, participants, "agg")
	matchID := createSportsMatch(t, sportsRepo, 4015, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{"pick": "home"}, human.ID, "human")
	testutil.AssertStatus(t, rec, http.StatusOK)

	// Anonymous GET — no claims in context.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/matches/"+matchID, nil)
	req.SetPathValue("id", matchID)
	rec = httptest.NewRecorder()
	h.GetMatch(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Data struct {
			Match      models.SportsMatch `json:"match"`
			Aggregates struct {
				Home   int             `json:"home"`
				Total  int             `json:"total"`
				Viewer json.RawMessage `json:"viewer"`
			} `json:"aggregates"`
		} `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if resp.Data.Match.ID != matchID {
		t.Errorf("expected match id %s, got %s", matchID, resp.Data.Match.ID)
	}
	if resp.Data.Aggregates.Home != 1 || resp.Data.Aggregates.Total != 1 {
		t.Errorf("expected aggregates home=1 total=1, got home=%d total=%d",
			resp.Data.Aggregates.Home, resp.Data.Aggregates.Total)
	}
	if string(resp.Data.Aggregates.Viewer) != "null" {
		t.Errorf("expected null viewer for anon, got %s", resp.Data.Aggregates.Viewer)
	}
}

func TestSportsGetMatch_ViewerSeesOwnPrediction(t *testing.T) {
	h, sportsRepo, participants := setupSportsTest(t)
	human := createSportsHuman(t, participants, "viewer")
	matchID := createSportsMatch(t, sportsRepo, 4016, time.Now().Add(24*time.Hour).UTC())

	rec := postPrediction(t, h, matchID, map[string]any{"pick": "away"}, human.ID, "human")
	testutil.AssertStatus(t, rec, http.StatusOK)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/matches/"+matchID, nil)
	req.SetPathValue("id", matchID)
	req = withSportsClaims(req, human.ID, "human")
	rec = httptest.NewRecorder()
	h.GetMatch(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Data struct {
			Aggregates struct {
				Viewer *models.SportsPrediction `json:"viewer"`
			} `json:"aggregates"`
		} `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if resp.Data.Aggregates.Viewer == nil {
		t.Fatal("expected viewer prediction for authenticated viewer")
	}
	if resp.Data.Aggregates.Viewer.Pick != "away" {
		t.Errorf("expected viewer pick away, got %q", resp.Data.Aggregates.Viewer.Pick)
	}
}

func TestSportsGetMatch_NotFoundAndMalformed(t *testing.T) {
	h, _, _ := setupSportsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/matches/00000000-0000-0000-0000-000000000001", nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000001")
	rec := httptest.NewRecorder()
	h.GetMatch(rec, req)
	testutil.AssertStatus(t, rec, http.StatusNotFound)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/sports/matches/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	rec = httptest.NewRecorder()
	h.GetMatch(rec, req)
	testutil.AssertStatus(t, rec, http.StatusNotFound)
}

// --- GET leaderboard ---

func TestSportsLeaderboard_InvalidKind(t *testing.T) {
	h, _, _ := setupSportsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/leaderboard?kind=bogus", nil)
	rec := httptest.NewRecorder()
	h.Leaderboard(rec, req)
	testutil.AssertStatus(t, rec, http.StatusBadRequest)
}

func TestSportsLeaderboard_DefaultsToAgent(t *testing.T) {
	h, _, _ := setupSportsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/leaderboard", nil)
	rec := httptest.NewRecorder()
	h.Leaderboard(rec, req)
	testutil.AssertStatus(t, rec, http.StatusOK)

	var resp struct {
		Data struct {
			Rows           []models.SportsLeaderboardRow `json:"rows"`
			HumansVsAgents map[string]any                `json:"humans_vs_agents"`
		} `json:"data"`
	}
	testutil.DecodeResponse(t, rec, &resp)
	if resp.Data.Rows == nil {
		t.Error("expected non-null rows array")
	}
	if resp.Data.HumansVsAgents == nil {
		t.Error("expected humans_vs_agents object")
	}
}
