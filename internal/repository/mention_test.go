package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestCreateForPublicCommentRechecksConcurrentQuarantine(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "mentions", "comments", "posts", "communities", "human_users", "participants")
	ctx := context.Background()

	participants := repository.NewParticipantRepo(pool)
	communities := repository.NewCommunityRepo(pool)
	posts := repository.NewPostRepo(pool)
	comments := repository.NewCommentRepo(pool)
	mentions := repository.NewMentionRepo(pool)
	author := createTestOwner(t, participants, ctx, "mention-author")
	mentioned := createTestOwner(t, participants, ctx, "mention-recipient")
	community := createTestCommunity(t, communities, ctx, author.ID, "mention-publicity")
	post := createTestPost(t, posts, ctx, community.ID, author.ID, "Mention publicity")
	comment, err := comments.Create(ctx, &models.Comment{
		PostID: post.ID, AuthorID: author.ID, AuthorType: models.ParticipantHuman, Body: "@mention-recipient",
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}

	quarantineTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin quarantine: %v", err)
	}
	defer func() { _ = quarantineTx.Rollback(ctx) }()
	if _, err := quarantineTx.Exec(ctx, `UPDATE posts SET quarantined = TRUE WHERE id = $1`, post.ID); err != nil {
		t.Fatalf("stage quarantine: %v", err)
	}

	type outcome struct {
		created bool
		err     error
	}
	result := make(chan outcome, 1)
	go func() {
		created, err := mentions.CreateForPublicComment(ctx, comment.ID, mentioned.ID, author.ID)
		result <- outcome{created: created, err: err}
	}()
	select {
	case got := <-result:
		t.Fatalf("mention persistence did not wait for concurrent quarantine: %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
	if err := quarantineTx.Commit(ctx); err != nil {
		t.Fatalf("commit quarantine: %v", err)
	}
	got := <-result
	if got.err != nil || got.created {
		t.Fatalf("mention created after concurrent quarantine: %+v", got)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM mentions WHERE content_id = $1`, comment.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("persisted mentions=%d err=%v", count, err)
	}
}
