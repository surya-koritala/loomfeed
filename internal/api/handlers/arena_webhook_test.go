package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/api/handlers"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/testutil"
)

type recordedArenaEvent struct {
	typeName string
	payload  map[string]any
}

type recordingArenaDispatcher struct {
	events []recordedArenaEvent
}

func (d *recordingArenaDispatcher) Dispatch(eventType string, payload map[string]any) {
	d.events = append(d.events, recordedArenaEvent{typeName: eventType, payload: payload})
}

func TestArenaHandlerDispatchesWebhookLifecycleEvents(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"arena_comments", "arena_votes", "arena_rounds", "arena_battles", "reputation_events",
		"human_verifications", "provenances", "quality_gates", "votes", "comments", "posts",
		"community_subscriptions", "communities", "api_keys", "agent_identities", "human_users", "participants",
	)
	participants := repository.NewParticipantRepo(pool)
	arena := repository.NewArenaRepo(pool)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "arena-webhook-test", Expiry: time.Hour}}
	human, humanToken := registerTestUser(t, participants, cfg, "arena-webhooks@example.com", "Arena Host")

	createAgent := func(name string) (*models.AgentIdentity, string) {
		t.Helper()
		agent, err := participants.CreateAgent(context.Background(), &models.AgentIdentity{
			Participant: models.Participant{DisplayName: name, ReputationScore: 10},
			OwnerID:     human.ID, ModelProvider: "test", ModelName: "test", ProtocolType: models.ProtocolREST,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		token, err := generateTokenForParticipant(cfg, agent.ID, string(models.ParticipantAgent))
		if err != nil {
			t.Fatalf("token for %s: %v", name, err)
		}
		return agent, token
	}
	agentA, tokenA := createAgent("Arena Agent A")
	agentB, tokenB := createAgent("Arena Agent B")

	dispatcher := &recordingArenaDispatcher{}
	handler := handlers.NewArenaHandler(arena, participants)
	handler.WithWebhook(dispatcher)
	negativeStakeReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/arena", humanToken, models.CreateBattleRequest{
		Topic: "Invalid negative stake", AgentAID: agentA.ID, AgentBID: agentB.ID, TrustStake: -1,
	})
	negativeStakeRec := httptest.NewRecorder()
	middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)).ServeHTTP(negativeStakeRec, negativeStakeReq)
	testutil.AssertStatus(t, negativeStakeRec, http.StatusBadRequest)
	if len(dispatcher.events) != 0 {
		t.Fatalf("invalid battle must not dispatch lifecycle events: %#v", dispatcher.events)
	}

	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/arena", humanToken, models.CreateBattleRequest{
		Topic:          "Should autonomous systems explain every consequential decision?",
		AgentAID:       agentA.ID,
		AgentBID:       agentB.ID,
		TotalRounds:    2,
		RoundTimeLimit: 3600,
		TrustStake:     3,
	})
	createRec := httptest.NewRecorder()
	middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Create)).ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)
	var battle models.ArenaBattle
	testutil.DecodeResponse(t, createRec, &battle)
	if len(battle.Rounds) != 2 {
		t.Fatalf("expected two initialized rounds, got %d", len(battle.Rounds))
	}
	assertArenaEvent(t, dispatcher.events, 0, "arena.challenge_created", battle.ID, 0)
	assertArenaEvent(t, dispatcher.events, 1, "arena.round_opened", battle.ID, 1)

	submit := func(round int, token, argument string) {
		t.Helper()
		req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/arena/"+battle.ID+"/rounds/1/submit", token, models.SubmitArgumentRequest{Argument: argument})
		req.SetPathValue("id", battle.ID)
		req.SetPathValue("n", string(rune('0'+round)))
		rec := httptest.NewRecorder()
		middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.SubmitArgument)).ServeHTTP(rec, req)
		testutil.AssertStatus(t, rec, http.StatusOK)
	}
	vote := func(round int, votedFor string) {
		t.Helper()
		req := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/arena/"+battle.ID+"/rounds/1/vote", humanToken, models.CastVoteRequest{
			VotedFor: votedFor, ArgumentScore: 5, SourceScore: 5, ClarityScore: 5,
		})
		req.SetPathValue("id", battle.ID)
		req.SetPathValue("n", string(rune('0'+round)))
		rec := httptest.NewRecorder()
		middleware.Auth(cfg.JWT.Secret)(http.HandlerFunc(handler.Vote)).ServeHTTP(rec, req)
		testutil.AssertStatus(t, rec, http.StatusOK)
	}

	submit(1, tokenA, "Opening case from A")
	submit(1, tokenB, "Opening case from B")
	assertArenaEvent(t, dispatcher.events, 2, "arena.round_opened", battle.ID, 2)
	vote(1, agentA.ID)
	if len(dispatcher.events) != 3 {
		t.Fatalf("battle must not complete after only one of two rounds; events=%#v", dispatcher.events)
	}

	submit(2, tokenA, "Closing case from A")
	submit(2, tokenB, "Closing case from B")
	if len(dispatcher.events) != 3 {
		t.Fatalf("final submission must not open a nonexistent round; events=%#v", dispatcher.events)
	}
	vote(2, agentA.ID)
	assertArenaEvent(t, dispatcher.events, 3, "arena.battle_completed", battle.ID, 0)
	if got := dispatcher.events[3].payload["winner_id"]; got != agentA.ID {
		t.Fatalf("completed payload winner_id=%v, want %s", got, agentA.ID)
	}
	if got := dispatcher.events[3].payload["settled_stake"]; got != float64(3) {
		t.Fatalf("completed payload settled_stake=%v, want 3", got)
	}
	if dispatcher.events[3].payload["stake_settled_at"] == nil {
		t.Fatal("completed payload must expose the durable stake settlement time")
	}
	assertHandlerArenaReputation(t, pool, agentA.ID, 13)
	assertHandlerArenaReputation(t, pool, agentB.ID, 7)
}

func assertHandlerArenaReputation(t *testing.T, pool *pgxpool.Pool, participantID string, want float64) {
	t.Helper()
	var got float64
	if err := pool.QueryRow(context.Background(), `SELECT reputation_score FROM participants WHERE id = $1`, participantID).Scan(&got); err != nil || got != want {
		t.Fatalf("participant %s reputation=%v err=%v, want %v", participantID, got, err, want)
	}
}

func assertArenaEvent(t *testing.T, events []recordedArenaEvent, index int, eventType, battleID string, roundNumber int) {
	t.Helper()
	if len(events) <= index {
		t.Fatalf("missing event %d (%s); got %#v", index, eventType, events)
	}
	event := events[index]
	if event.typeName != eventType {
		t.Fatalf("event %d type=%q, want %q", index, event.typeName, eventType)
	}
	if event.payload["battle_id"] != battleID {
		t.Fatalf("event %d battle_id=%v, want %s", index, event.payload["battle_id"], battleID)
	}
	if roundNumber > 0 && event.payload["round_number"] != roundNumber {
		t.Fatalf("event %d round_number=%v, want %d", index, event.payload["round_number"], roundNumber)
	}
}
