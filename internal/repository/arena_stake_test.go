package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestArenaDeadlineCompletionSettlesCappedStakeExactlyOnce(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"arena_comments", "arena_votes", "arena_rounds", "arena_battles", "reputation_events",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	arena := repository.NewArenaRepo(pool)
	host := createTestOwner(t, participants, ctx, "arena-stake-host")
	voter := createTestOwner(t, participants, ctx, "arena-stake-voter")
	createAgent := func(name string, reputation float64) *models.AgentIdentity {
		t.Helper()
		agent, err := participants.CreateAgent(ctx, &models.AgentIdentity{
			Participant: models.Participant{DisplayName: name, TrustScore: reputation, ReputationScore: reputation},
			OwnerID:     host.ID, ModelProvider: "test", ModelName: "test", ProtocolType: models.ProtocolREST,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return agent
	}
	agentA := createAgent("Stake Agent A", 50)
	agentB := createAgent("Stake Agent B", 7)
	battle, err := arena.CreateBattle(ctx, &models.ArenaBattle{
		Topic: "Stake settlement", AgentAID: agentA.ID, AgentBID: agentB.ID,
		Format: models.ArenaFormatPointCounterpoint, TotalRounds: 1,
		RoundTimeLimit: 60, WordLimit: 500, TrustStake: 20, CreatedBy: host.ID,
	})
	if err != nil {
		t.Fatalf("create battle: %v", err)
	}
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	if err := arena.CreateRound(ctx, &models.ArenaRound{BattleID: battle.ID, RoundNumber: 1, RoundType: "closing", Deadline: &expired}); err != nil {
		t.Fatalf("create round: %v", err)
	}
	round, _ := arena.GetRoundByBattleAndNumber(ctx, battle.ID, 1)
	insertArenaVote(t, pool, battle.ID, round.ID, voter.ID, agentA.ID)

	transitions, err := arena.ProcessExpiredRounds(ctx, now, 10)
	if err != nil || len(transitions) != 1 || !transitions[0].Completed {
		t.Fatalf("complete battle: transitions=%#v err=%v", transitions, err)
	}
	settled, _ := arena.GetBattle(ctx, battle.ID)
	if settled.StakeSettledAt == nil || settled.SettledStake != 7 {
		t.Fatalf("stake should cap to loser balance 7: %#v", settled)
	}

	assertParticipantReputation(t, pool, agentA.ID, 57)
	assertParticipantReputation(t, pool, agentB.ID, 0)
	assertArenaStakeEvent(t, pool, agentA.ID, repository.EventArenaStakeWon, 7)
	assertArenaStakeEvent(t, pool, agentB.ID, repository.EventArenaStakeLost, -7)

	again, err := arena.ProcessExpiredRounds(ctx, now.Add(time.Hour), 10)
	if err != nil || len(again) != 0 {
		t.Fatalf("second settlement sweep: transitions=%#v err=%v", again, err)
	}
	assertParticipantReputation(t, pool, agentA.ID, 57)
	assertParticipantReputation(t, pool, agentB.ID, 0)
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM reputation_events WHERE event_type IN ('arena_stake_won', 'arena_stake_lost')`).Scan(&eventCount); err != nil || eventCount != 2 {
		t.Fatalf("stake events count=%d err=%v, want 2", eventCount, err)
	}
}

func TestArenaDrawReturnsStakeAndRecordsSettlementExactlyOnce(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"arena_comments", "arena_votes", "arena_rounds", "arena_battles", "reputation_events",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	arena := repository.NewArenaRepo(pool)
	host := createTestOwner(t, participants, ctx, "arena-draw-host")
	createAgent := func(name string, reputation float64) *models.AgentIdentity {
		t.Helper()
		agent, err := participants.CreateAgent(ctx, &models.AgentIdentity{
			Participant: models.Participant{DisplayName: name, TrustScore: reputation, ReputationScore: reputation},
			OwnerID:     host.ID, ModelProvider: "test", ModelName: "test", ProtocolType: models.ProtocolREST,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return agent
	}
	agentA := createAgent("Draw Agent A", 12)
	agentB := createAgent("Draw Agent B", 9)
	battle, err := arena.CreateBattle(ctx, &models.ArenaBattle{
		Topic: "Draw settlement", AgentAID: agentA.ID, AgentBID: agentB.ID,
		Format: models.ArenaFormatPointCounterpoint, TotalRounds: 1,
		RoundTimeLimit: 60, WordLimit: 500, TrustStake: 8, CreatedBy: host.ID,
	})
	if err != nil {
		t.Fatalf("create battle: %v", err)
	}

	if err := arena.UpdateBattleStatus(ctx, battle.ID, models.ArenaStatusCompleted, nil); err != nil {
		t.Fatalf("complete drawn battle: %v", err)
	}
	settled, err := arena.GetBattle(ctx, battle.ID)
	if err != nil {
		t.Fatalf("get settled draw: %v", err)
	}
	if settled.StakeSettledAt == nil || settled.SettledStake != 0 {
		t.Fatalf("draw should durably return rather than transfer stake: %#v", settled)
	}
	assertParticipantReputation(t, pool, agentA.ID, 12)
	assertParticipantReputation(t, pool, agentB.ID, 9)
	assertArenaStakeEvent(t, pool, agentA.ID, repository.EventArenaStakeReturn, 0)
	assertArenaStakeEvent(t, pool, agentB.ID, repository.EventArenaStakeReturn, 0)

	if err := arena.UpdateBattleStatus(ctx, battle.ID, models.ArenaStatusCompleted, nil); err != nil {
		t.Fatalf("repeat drawn completion: %v", err)
	}
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM reputation_events WHERE event_type = 'arena_stake_returned'`).Scan(&eventCount); err != nil || eventCount != 2 {
		t.Fatalf("draw return events count=%d err=%v, want 2", eventCount, err)
	}
}

func assertParticipantReputation(t *testing.T, pool *pgxpool.Pool, participantID string, want float64) {
	t.Helper()
	var got float64
	if err := pool.QueryRow(context.Background(), `SELECT reputation_score FROM participants WHERE id = $1`, participantID).Scan(&got); err != nil || got != want {
		t.Fatalf("participant %s reputation=%v err=%v, want %v", participantID, got, err, want)
	}
}

func assertArenaStakeEvent(t *testing.T, pool *pgxpool.Pool, participantID, eventType string, wantDelta float64) {
	t.Helper()
	var delta float64
	if err := pool.QueryRow(context.Background(), `SELECT score_delta FROM reputation_events WHERE participant_id = $1 AND event_type = $2`, participantID, eventType).Scan(&delta); err != nil || delta != wantDelta {
		t.Fatalf("event %s for %s delta=%v err=%v, want %v", eventType, participantID, delta, err, wantDelta)
	}
}
