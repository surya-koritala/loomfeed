package scorecard_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/scorecard"
)

func TestCompute_CorrectionRateUsesOrderedContestAndRetractionHistory(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"scorecard_history", "agent_scorecards", "epistemic_votes",
		"reputation_events", "votes", "posts", "communities", "participants",
	)
	ctx := context.Background()

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	epistemic := repository.NewEpistemicRepo(pool)

	author := createScorecardHuman(t, ctx, participants, "correction-author")
	voter := createScorecardHuman(t, ctx, participants, "correction-voter")
	community, err := communities.Create(ctx, &models.Community{
		Name:      "Correction Test",
		Slug:      "correction-test",
		CreatedBy: author.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}

	acknowledged := createScorecardPost(t, ctx, posts, community.ID, author.ID, "Acknowledged")
	ignored := createScorecardPost(t, ctx, posts, community.ID, author.ID, "Ignored")
	retractedFirst := createScorecardPost(t, ctx, posts, community.ID, author.ID, "Retracted first")

	if err := epistemic.Vote(ctx, acknowledged.ID, voter.ID, "contested"); err != nil {
		t.Fatalf("contest acknowledged post: %v", err)
	}
	if err := posts.Retract(ctx, acknowledged.ID, "The criticism was correct."); err != nil {
		t.Fatalf("retract acknowledged post: %v", err)
	}

	if err := epistemic.Vote(ctx, ignored.ID, voter.ID, "contested"); err != nil {
		t.Fatalf("contest ignored post: %v", err)
	}

	if err := posts.Retract(ctx, retractedFirst.ID, "Retracted before any contest."); err != nil {
		t.Fatalf("retract pre-contest post: %v", err)
	}
	time.Sleep(time.Millisecond)
	if err := epistemic.Vote(ctx, retractedFirst.ID, voter.ID, "contested"); err != nil {
		t.Fatalf("contest pre-retracted post: %v", err)
	}

	sc, err := scorecard.Compute(ctx, pool, author.ID)
	if err != nil {
		t.Fatalf("compute scorecard: %v", err)
	}

	want := 1.0 / 3.0
	if !sc.Signals.CorrectionRate.HasData {
		t.Fatal("correction rate should have data when a post was contested")
	}
	if math.Abs(sc.Signals.CorrectionRate.Raw-want) > 1e-9 {
		t.Fatalf("correction raw rate = %v, want %v", sc.Signals.CorrectionRate.Raw, want)
	}
	if math.Abs(sc.Signals.CorrectionRate.Normalized-want) > 1e-9 {
		t.Fatalf("correction normalized rate = %v, want %v", sc.Signals.CorrectionRate.Normalized, want)
	}

	quiet := createScorecardHuman(t, ctx, participants, "no-corrections-warranted")
	quietScorecard, err := scorecard.Compute(ctx, pool, quiet.ID)
	if err != nil {
		t.Fatalf("compute scorecard without corrections: %v", err)
	}
	if quietScorecard.Signals.CorrectionRate.HasData {
		t.Fatal("correction rate should be missing, not zero, when no correction was warranted")
	}
}

func createScorecardHuman(t *testing.T, ctx context.Context, repo *repository.ParticipantRepo, suffix string) *models.Participant {
	t.Helper()
	p, err := repo.CreateHuman(ctx, &models.HumanUser{
		Participant:       models.Participant{DisplayName: suffix},
		Email:             suffix + "@example.com",
		PasswordHash:      "test-password-hash",
		PreferredLanguage: "en",
		NotificationPrefs: "{}",
	})
	if err != nil {
		t.Fatalf("create participant %s: %v", suffix, err)
	}
	return p
}

func createScorecardPost(t *testing.T, ctx context.Context, repo *repository.PostRepo, communityID, authorID, title string) *models.Post {
	t.Helper()
	p, err := repo.Create(ctx, &models.Post{
		CommunityID: communityID,
		AuthorID:    authorID,
		AuthorType:  models.ParticipantHuman,
		Title:       title,
		Body:        "Scorecard correction test body.",
	})
	if err != nil {
		t.Fatalf("create post %s: %v", title, err)
	}
	return p
}
