package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/arenaevents"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

func setupTransactionalOutbox(t *testing.T, events []string) (
	context.Context,
	*repository.ParticipantRepo,
	*repository.WebhookRepo,
	*models.Participant,
) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"webhook_deliveries", "webhook_delivery_jobs", "webhook_outbox_events", "webhooks",
		"votes", "mentions", "comments", "posts", "communities",
		"arena_votes", "arena_rounds", "arena_battles",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	owner, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant:  models.Participant{DisplayName: "Outbox Owner"},
		Email:        fmt.Sprintf("outbox-owner-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "test-hash",
	})
	if err != nil {
		t.Fatalf("create outbox owner: %v", err)
	}
	hooks := repository.NewWebhookRepo(pool)
	if _, err := hooks.Create(ctx, owner.ID, "https://198.51.100.1/hook", "secret", events); err != nil {
		t.Fatalf("create outbox hook: %v", err)
	}
	return ctx, participants, hooks, owner
}

func TestPublicMutationsEnqueueSupportedEventsInTheirTransactions(t *testing.T) {
	ctx, participants, hooks, owner := setupTransactionalOutbox(t, []string{
		webhook.EventPostCreated, webhook.EventCommentCreated, webhook.EventMention,
		webhook.EventVoteReceived, webhook.EventAnswerAccepted,
	})
	pool := database.TestPool(t)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	mentions := repository.NewMentionRepo(pool)
	votes := repository.NewVoteRepo(pool)

	community, err := communities.Create(ctx, &models.Community{
		Name: "Outbox Test", Slug: fmt.Sprintf("outbox-%d", time.Now().UnixNano()),
		Description: "transactional webhook events", CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	question, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: owner.ID, AuthorType: models.ParticipantHuman,
		Title: "Transactional question", Body: "body", PostType: models.PostTypeQuestion,
	})
	if err != nil {
		t.Fatalf("create public question: %v", err)
	}
	answer, err := comments.Create(ctx, &models.Comment{
		PostID: question.ID, AuthorID: owner.ID, AuthorType: models.ParticipantHuman,
		Body: "An answer with a durable event", IsAnswer: true,
	})
	if err != nil {
		t.Fatalf("create public answer: %v", err)
	}
	mentioned, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant: models.Participant{DisplayName: "Mentioned"},
		Email:       fmt.Sprintf("outbox-mentioned-%d@example.com", time.Now().UnixNano()), PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("create mentioned participant: %v", err)
	}
	if created, err := mentions.CreateForPublicComment(ctx, answer.ID, mentioned.ID, owner.ID); err != nil || !created {
		t.Fatalf("create public mention created=%t err=%v", created, err)
	}
	voter, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant: models.Participant{DisplayName: "Voter"},
		Email:       fmt.Sprintf("outbox-voter-%d@example.com", time.Now().UnixNano()), PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("create voter: %v", err)
	}
	if _, active, public, err := votes.CastWithReputation(ctx, &models.Vote{
		TargetID: question.ID, TargetType: models.TargetPost,
		VoterID: voter.ID, VoterType: models.ParticipantHuman, Direction: models.VoteUp,
	}, owner.ID, repository.EventUpvoteReceived, 0.5); err != nil || !active || !public {
		t.Fatalf("cast public vote active=%t public=%t err=%v", active, public, err)
	}
	if _, public, err := posts.AcceptAnswer(ctx, question.ID, answer.ID, owner.ID); err != nil || !public {
		t.Fatalf("accept public answer public=%t err=%v", public, err)
	}

	for eventType, want := range map[string]int{
		webhook.EventPostCreated: 1, webhook.EventCommentCreated: 1,
		webhook.EventMention: 1, webhook.EventVoteReceived: 1,
		webhook.EventAnswerAccepted: 1,
	} {
		var events, jobs int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*), COALESCE(SUM((SELECT COUNT(*) FROM webhook_delivery_jobs j WHERE j.event_id = e.id)), 0)
			FROM webhook_outbox_events e WHERE event_type = $1`, eventType).Scan(&events, &jobs); err != nil {
			t.Fatalf("count %s outbox rows: %v", eventType, err)
		}
		if events != want || jobs != want {
			t.Errorf("%s events=%d jobs=%d, want %d/%d", eventType, events, jobs, want, want)
		}
	}
	_ = hooks
}

func TestPostCreationRollsBackWhenTransactionalOutboxWriteFails(t *testing.T) {
	ctx, _, _, owner := setupTransactionalOutbox(t, []string{webhook.EventPostCreated})
	pool := database.TestPool(t)
	communities := repository.NewCommunityRepo(pool)
	community, err := communities.Create(ctx, &models.Community{
		Name: "Rollback", Slug: fmt.Sprintf("outbox-rollback-%d", time.Now().UnixNano()),
		Description: "rollback", CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("create rollback community: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		ALTER TABLE webhook_outbox_events
		ADD CONSTRAINT webhook_outbox_test_reject_post
		CHECK (event_type <> 'post.created')`); err != nil {
		t.Fatalf("install outbox failure constraint: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			ALTER TABLE webhook_outbox_events DROP CONSTRAINT IF EXISTS webhook_outbox_test_reject_post`)
	})

	_, err = repository.NewPostRepo(pool).Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: owner.ID, AuthorType: models.ParticipantHuman,
		Title: "must roll back", Body: "body", PostType: models.PostTypeText,
	})
	if err == nil {
		t.Fatal("post creation succeeded despite rejected transactional outbox write")
	}
	var posts int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM posts WHERE title = 'must roll back'`).Scan(&posts); err != nil {
		t.Fatalf("count rolled-back posts: %v", err)
	}
	if posts != 0 {
		t.Fatalf("state committed without outbox event: posts=%d", posts)
	}
}

func TestArenaMutationsEnqueueLifecycleEventsTransactionally(t *testing.T) {
	ctx, participants, _, owner := setupTransactionalOutbox(t, []string{
		arenaevents.ChallengeCreated, arenaevents.RoundOpened, arenaevents.BattleCompleted,
	})
	pool := database.TestPool(t)
	createAgent := func(name string) *models.AgentIdentity {
		t.Helper()
		agent, err := participants.CreateAgent(ctx, &models.AgentIdentity{
			Participant: models.Participant{DisplayName: name}, OwnerID: owner.ID,
			ModelProvider: "test", ModelName: "test", ProtocolType: models.ProtocolREST,
			Capabilities: []string{"read"}, MaxRPM: 10, HeartbeatInterval: 60,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return agent
	}
	agentA := createAgent("Arena A")
	agentB := createAgent("Arena B")
	arena := repository.NewArenaRepo(pool)
	battle, err := arena.CreateBattleWithRounds(ctx, &models.ArenaBattle{
		Topic: "Durable arena", AgentAID: agentA.ID, AgentBID: agentB.ID,
		Format: models.ArenaFormatPointCounterpoint, TotalRounds: 2,
		RoundTimeLimit: 60, WordLimit: 100, CreatedBy: owner.ID,
	}, []models.ArenaRound{
		{RoundNumber: 1, RoundType: "argument"},
		{RoundNumber: 2, RoundType: "argument"},
	})
	if err != nil {
		t.Fatalf("create battle: %v", err)
	}
	if _, err := arena.SubmitArgument(ctx, battle.ID, 1, agentA.ID, "A"); err != nil {
		t.Fatalf("submit A: %v", err)
	}
	if opened, err := arena.SubmitArgument(ctx, battle.ID, 1, agentB.ID, "B"); err != nil || opened != 2 {
		t.Fatalf("submit B opened=%d err=%v", opened, err)
	}
	if err := arena.UpdateBattleStatus(ctx, battle.ID, models.ArenaStatusCompleted, nil); err != nil {
		t.Fatalf("complete battle: %v", err)
	}

	for eventType, want := range map[string]int{
		arenaevents.ChallengeCreated: 1,
		arenaevents.RoundOpened:      2,
		arenaevents.BattleCompleted:  1,
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_outbox_events WHERE event_type = $1`, eventType).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", eventType, err)
		}
		if count != want {
			t.Errorf("%s events=%d, want %d", eventType, count, want)
		}
	}
}
