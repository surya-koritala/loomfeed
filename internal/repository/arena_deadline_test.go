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

func TestArenaRepoProcessExpiredRoundsAdvancesCompletesAndIsIdempotent(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"arena_comments", "arena_votes", "arena_rounds", "arena_battles",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	arena := repository.NewArenaRepo(pool)
	host := createTestOwner(t, participants, ctx, "arena-deadline-host")
	voter := createTestOwner(t, participants, ctx, "arena-deadline-voter")
	createAgent := func(name string) *models.AgentIdentity {
		t.Helper()
		agent, err := participants.CreateAgent(ctx, &models.AgentIdentity{
			Participant: models.Participant{DisplayName: name},
			OwnerID:     host.ID, ModelProvider: "test", ModelName: "test", ProtocolType: models.ProtocolREST,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return agent
	}
	agentA := createAgent("Deadline Agent A")
	agentB := createAgent("Deadline Agent B")

	battle, err := arena.CreateBattle(ctx, &models.ArenaBattle{
		Topic: "Deadline test", AgentAID: agentA.ID, AgentBID: agentB.ID,
		Format: models.ArenaFormatPointCounterpoint, TotalRounds: 2,
		RoundTimeLimit: 60, WordLimit: 500, CreatedBy: host.ID,
	})
	if err != nil {
		t.Fatalf("create battle: %v", err)
	}
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	for number, deadline := range []time.Time{expired, future} {
		if err := arena.CreateRound(ctx, &models.ArenaRound{
			BattleID: battle.ID, RoundNumber: number + 1, RoundType: "argument", Deadline: &deadline,
		}); err != nil {
			t.Fatalf("create round %d: %v", number+1, err)
		}
	}
	round1, err := arena.GetRoundByBattleAndNumber(ctx, battle.ID, 1)
	if err != nil {
		t.Fatalf("get round 1: %v", err)
	}
	insertArenaVote(t, pool, battle.ID, round1.ID, voter.ID, agentA.ID)

	transitions, err := arena.ProcessExpiredRounds(ctx, now, 10)
	if err != nil {
		t.Fatalf("process first deadline: %v", err)
	}
	if len(transitions) != 1 || transitions[0].ClosedRound != 1 || transitions[0].OpenedRound != 2 || transitions[0].Completed {
		t.Fatalf("unexpected first transition: %#v", transitions)
	}
	round1, _ = arena.GetRoundByBattleAndNumber(ctx, battle.ID, 1)
	if round1.ClosedAt == nil || round1.ClosureReason != "deadline" || round1.RoundWinner == nil || *round1.RoundWinner != agentA.ID {
		t.Fatalf("expired round was not closed for vote winner A: %#v", round1)
	}
	advanced, _ := arena.GetBattle(ctx, battle.ID)
	if advanced.Status != models.ArenaStatusActive || advanced.CurrentRound != 2 {
		t.Fatalf("battle did not advance to active round 2: %#v", advanced)
	}

	if _, err := pool.Exec(ctx, `UPDATE arena_rounds SET deadline = $1 WHERE battle_id = $2 AND round_number = 2`, expired, battle.ID); err != nil {
		t.Fatalf("expire round 2: %v", err)
	}
	round2, _ := arena.GetRoundByBattleAndNumber(ctx, battle.ID, 2)
	insertArenaVote(t, pool, battle.ID, round2.ID, voter.ID, agentA.ID)
	transitions, err = arena.ProcessExpiredRounds(ctx, now, 10)
	if err != nil {
		t.Fatalf("process final deadline: %v", err)
	}
	if len(transitions) != 1 || transitions[0].ClosedRound != 2 || transitions[0].OpenedRound != 0 || !transitions[0].Completed {
		t.Fatalf("unexpected completion transition: %#v", transitions)
	}
	completed, _ := arena.GetBattle(ctx, battle.ID)
	if completed.Status != models.ArenaStatusCompleted || completed.WinnerID == nil || *completed.WinnerID != agentA.ID || completed.CompletedAt == nil {
		t.Fatalf("battle did not complete for A: %#v", completed)
	}

	again, err := arena.ProcessExpiredRounds(ctx, now.Add(time.Hour), 10)
	if err != nil || len(again) != 0 {
		t.Fatalf("deadline processing must be idempotent, transitions=%#v err=%v", again, err)
	}
}

func insertArenaVote(t *testing.T, pool *pgxpool.Pool, battleID, roundID, voterID, votedFor string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO arena_votes (battle_id, round_id, voter_id, voted_for, argument_score, source_score, clarity_score)
		VALUES ($1, $2, $3, $4, 5, 5, 5)`, battleID, roundID, voterID, votedFor); err != nil {
		t.Fatalf("insert arena vote: %v", err)
	}
}
