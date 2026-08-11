package activitypub_test

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/activitypub"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestResolveLocalTargetAndPlainTextContent(t *testing.T) {
	postID := "11111111-1111-4111-8111-111111111111"
	commentID := "22222222-2222-4222-8222-222222222222"
	postTarget, err := activitypub.ResolveLocalTarget("https://loomfeed.example", "https://loomfeed.example/post/"+postID+"/a-title")
	if err != nil || postTarget.PostID != postID || postTarget.TargetID != postID || postTarget.TargetType != models.TargetPost || postTarget.ParentCommentID != nil {
		t.Fatalf("resolve local post: target=%#v err=%v", postTarget, err)
	}
	commentTarget, err := activitypub.ResolveLocalTarget("https://loomfeed.example", "https://loomfeed.example/post/"+postID+"/comment/"+commentID)
	if err != nil || commentTarget.PostID != postID || commentTarget.TargetID != commentID || commentTarget.TargetType != models.TargetComment || commentTarget.ParentCommentID == nil || *commentTarget.ParentCommentID != commentID {
		t.Fatalf("resolve local comment: target=%#v err=%v", commentTarget, err)
	}
	if _, err := activitypub.ResolveLocalTarget("https://loomfeed.example", "https://attacker.example/post/"+postID); err == nil {
		t.Fatal("foreign-origin target must be rejected")
	}

	plain, err := activitypub.PlainTextContent(`<p>Hello &amp; <strong>world</strong></p><script>alert(1)</script><p>Second<br>line</p>`, 10000)
	if err != nil || plain != "Hello & world\n\nSecond\nline" {
		t.Fatalf("safe HTML conversion=%q err=%v", plain, err)
	}
	if _, err := activitypub.PlainTextContent("<p></p>", 10000); err == nil {
		t.Fatal("empty remote content must be rejected")
	}
}

func TestInboundRepoPersistsRemoteReplyAndWeightedLikeIdempotently(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"votes", "reputation_events", "ap_remote_actors", "ap_remote_trust", "comments", "posts", "communities",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	owner := createAPTestHuman(t, participants, ctx, "ap-inbound-owner")
	voter := createAPTestHuman(t, participants, ctx, "ap-inbound-voter")
	communities := repository.NewCommunityRepo(pool)
	community, err := communities.Create(ctx, &models.Community{
		Name: "Federation test", Slug: "federation-test", Description: "test", CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	posts := repository.NewPostRepo(pool)
	post, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: owner.ID, AuthorType: models.ParticipantHuman,
		Title: "Local post", Body: "Body", PostType: models.PostTypeText,
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	inbound := activitypub.NewInboundRepo(pool)
	actor := activitypub.RemoteActorProfile{
		URI: "https://remote.example/users/alice", PreferredUsername: "alice",
		DisplayName: "Alice Remote", AvatarURL: "https://remote.example/alice.png", LocalTrust: 80,
	}
	comment, created, err := inbound.IngestReply(ctx, activitypub.InboundReply{
		ActivityID: "https://remote.example/activities/create-1",
		ObjectID:   "https://remote.example/notes/reply-1",
		Actor:      actor, PostID: post.ID, Body: "A safe remote reply",
	})
	if err != nil || !created || comment.AuthorType != models.ParticipantRemote || comment.Body != "A safe remote reply" {
		t.Fatalf("ingest reply: comment=%#v created=%v err=%v", comment, created, err)
	}
	duplicate, created, err := inbound.IngestReply(ctx, activitypub.InboundReply{
		ActivityID: "https://remote.example/activities/create-duplicate",
		ObjectID:   "https://remote.example/notes/reply-1",
		Actor:      actor, PostID: post.ID, Body: "must not overwrite",
	})
	if err != nil || created || duplicate.ID != comment.ID || duplicate.Body != "A safe remote reply" {
		t.Fatalf("duplicate reply: comment=%#v created=%v err=%v", duplicate, created, err)
	}
	reloadedPost, _ := posts.GetByID(ctx, post.ID)
	if reloadedPost.CommentCount != 1 {
		t.Fatalf("duplicate reply changed comment_count to %d", reloadedPost.CommentCount)
	}

	weight := activitypub.TrustWeight(actor.LocalTrust)
	if math.Abs(weight-0.8) > 1e-9 {
		t.Fatalf("trust weight=%v, want 0.8", weight)
	}
	weightedScore, created, err := inbound.IngestLike(ctx, activitypub.InboundLike{
		ActivityID: "https://remote.example/activities/like-1", Actor: actor,
		TargetID: post.ID, TargetType: models.TargetPost, Weight: weight,
	})
	if err != nil || !created || weightedScore != 1 {
		t.Fatalf("ingest weighted like: score=%d created=%v err=%v", weightedScore, created, err)
	}
	weightedScore, created, err = inbound.IngestLike(ctx, activitypub.InboundLike{
		ActivityID: "https://remote.example/activities/like-duplicate", Actor: actor,
		TargetID: post.ID, TargetType: models.TargetPost, Weight: weight,
	})
	if err != nil || created || weightedScore != 1 {
		t.Fatalf("duplicate actor like: score=%d created=%v err=%v", weightedScore, created, err)
	}

	var storedWeight float64
	var voterType string
	if err := pool.QueryRow(ctx, `SELECT weight, voter_type::text FROM votes WHERE target_id = $1`, post.ID).Scan(&storedWeight, &voterType); err != nil || math.Abs(storedWeight-0.8) > 1e-9 || voterType != "remote" {
		t.Fatalf("stored remote vote weight=%v voter_type=%q err=%v", storedWeight, voterType, err)
	}
	var remoteType string
	var participantTrust float64
	if err := pool.QueryRow(ctx, `
		SELECT p.type::text, p.trust_score
		FROM participants p JOIN ap_remote_actors ra ON ra.participant_id = p.id
		WHERE ra.actor_uri = $1`, actor.URI).Scan(&remoteType, &participantTrust); err != nil || remoteType != "remote" || participantTrust != 80 {
		t.Fatalf("remote participant type=%q trust=%v err=%v", remoteType, participantTrust, err)
	}
	if _, err := activitypub.NewStore(pool).EnsureHandleAndKey(ctx, comment.AuthorID); err == nil {
		t.Fatal("materialized remote participant must never receive a local ActivityPub handle/key")
	}

	// A later local vote must include—not erase—the stored remote weight.
	votes := repository.NewVoteRepo(pool)
	remoteTrustRepo := activitypub.NewRemoteTrustRepo(pool)
	if err := remoteTrustRepo.RecordInteraction(ctx, actor.URI, "reply"); err != nil {
		t.Fatalf("seed remote trust row: %v", err)
	}
	if score, err := votes.CastWithReputation(ctx, &models.Vote{
		TargetID: comment.ID, TargetType: models.TargetComment, VoterID: voter.ID,
		VoterType: models.ParticipantHuman, Direction: models.VoteUp,
	}, comment.AuthorID, repository.EventUpvoteReceived, 0.3); err != nil || score != 1 {
		t.Fatalf("vote on remote reply score=%d err=%v", score, err)
	}
	remoteTrust, err := remoteTrustRepo.Get(ctx, actor.URI)
	if err != nil || remoteTrust.ReplyVoteSum != 1 || remoteTrust.LocalScore != 5.5 {
		t.Fatalf("remote reply vote did not update trust: trust=%#v err=%v", remoteTrust, err)
	}
	var remoteReputation, remoteParticipantTrust float64
	var reputationEvents int
	if err := pool.QueryRow(ctx, `SELECT reputation_score, trust_score FROM participants WHERE id = $1`, comment.AuthorID).Scan(&remoteReputation, &remoteParticipantTrust); err != nil || remoteReputation != 5.5 || remoteParticipantTrust != 5.5 {
		t.Fatalf("remote participant trust=%v reputation=%v err=%v", remoteParticipantTrust, remoteReputation, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM reputation_events WHERE participant_id = $1`, comment.AuthorID).Scan(&reputationEvents); err != nil || reputationEvents != 0 {
		t.Fatalf("remote reply vote created %d local reputation events err=%v", reputationEvents, err)
	}
	if score, err := votes.CastVote(ctx, &models.Vote{
		TargetID: post.ID, TargetType: models.TargetPost, VoterID: voter.ID,
		VoterType: models.ParticipantHuman, Direction: models.VoteUp,
	}); err != nil || score != 2 {
		t.Fatalf("local + weighted remote score=%d err=%v, want 2", score, err)
	}

	// Different actors liking the same object concurrently must not lose one
	// another's weighted contribution during aggregate score recalculation.
	concurrentPost, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: owner.ID, AuthorType: models.ParticipantHuman,
		Title: "Concurrent Like target", Body: "Body", PostType: models.PostTypeText,
	})
	if err != nil {
		t.Fatalf("create concurrent Like target: %v", err)
	}
	concurrentActors := []activitypub.RemoteActorProfile{
		{URI: "https://remote.example/users/bob", PreferredUsername: "bob", LocalTrust: 80},
		{URI: "https://remote.example/users/carol", PreferredUsername: "carol", LocalTrust: 80},
	}
	start := make(chan struct{})
	results := make(chan error, len(concurrentActors))
	var wg sync.WaitGroup
	for i, concurrentActor := range concurrentActors {
		wg.Add(1)
		go func(index int, concurrentActor activitypub.RemoteActorProfile) {
			defer wg.Done()
			<-start
			_, _, likeErr := inbound.IngestLike(ctx, activitypub.InboundLike{
				ActivityID: fmt.Sprintf("https://remote.example/activities/concurrent-like-%d", index),
				Actor:      concurrentActor, TargetID: concurrentPost.ID, TargetType: models.TargetPost,
				Weight: activitypub.TrustWeight(concurrentActor.LocalTrust),
			})
			results <- likeErr
		}(i, concurrentActor)
	}
	close(start)
	wg.Wait()
	close(results)
	for likeErr := range results {
		if likeErr != nil {
			t.Fatalf("concurrent federated Like: %v", likeErr)
		}
	}
	loadedConcurrentPost, err := posts.GetByID(ctx, concurrentPost.ID)
	if err != nil {
		t.Fatalf("load concurrent Like target: %v", err)
	}
	if loadedConcurrentPost.VoteScore != 2 {
		t.Fatalf("concurrent weighted Like score=%d err=%v, want 2", loadedConcurrentPost.VoteScore, err)
	}
}

func createAPTestHuman(t *testing.T, participants *repository.ParticipantRepo, ctx context.Context, prefix string) *models.Participant {
	t.Helper()
	now := time.Now().UnixNano()
	human, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant: models.Participant{DisplayName: fmt.Sprintf("%s-%d", prefix, now)},
		Email:       fmt.Sprintf("%s-%d@example.com", prefix, now), PasswordHash: "test-hash",
	})
	if err != nil {
		t.Fatalf("create human: %v", err)
	}
	return human
}
