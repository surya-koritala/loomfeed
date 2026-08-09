package repository_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// createTestCommunity is a helper that creates a community for use in post tests.
func createTestCommunity(t *testing.T, cRepo *repository.CommunityRepo, ctx context.Context, ownerID, suffix string) *models.Community {
	t.Helper()
	c, err := cRepo.Create(ctx, &models.Community{
		Name:      "Community " + suffix,
		Slug:      "community-" + suffix,
		CreatedBy: ownerID,
	})
	if err != nil {
		t.Fatalf("createTestCommunity (%s): %v", suffix, err)
	}
	return c
}

// createTestPost is a helper that creates a post in the given community.
func createTestPost(t *testing.T, pRepo *repository.PostRepo, ctx context.Context, communityID, authorID string, title string) *models.Post {
	t.Helper()
	post := &models.Post{
		CommunityID: communityID,
		AuthorID:    authorID,
		AuthorType:  models.ParticipantHuman,
		Title:       title,
		Body:        "Test body for " + title,
	}
	created, err := pRepo.Create(ctx, post)
	if err != nil {
		t.Fatalf("createTestPost (%s): %v", title, err)
	}
	return created
}

func TestPostRepo_CreateAndGetByID(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, fmt.Sprintf("postget-%d", 1))
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "postget-1")

	post := &models.Post{
		CommunityID: community.ID,
		AuthorID:    owner.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Hello World",
		Body:        "This is the post body",
		URL:         "https://example.com",
		PostType: models.PostTypeLink,
	}

	created, err := postRepo.Create(ctx, post)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.ID == "" {
		t.Error("expected non-empty ID")
	}
	if created.Title != "Hello World" {
		t.Errorf("expected title 'Hello World', got %q", created.Title)
	}
	if created.PostType != models.PostTypeLink {
		t.Errorf("expected post_type 'link', got %q", created.PostType)
	}
	if created.URL != "https://example.com" {
		t.Errorf("expected url 'https://example.com', got %q", created.URL)
	}
	if created.VoteScore != 0 {
		t.Errorf("expected vote_score 0, got %d", created.VoteScore)
	}

	got, err := postRepo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetByID returned ID %q, want %q", got.ID, created.ID)
	}
	if got.Title != "Hello World" {
		t.Errorf("GetByID returned Title %q, want 'Hello World'", got.Title)
	}
	if got.Author.ID != owner.ID {
		t.Errorf("GetByID returned Author.ID %q, want %q", got.Author.ID, owner.ID)
	}
	if got.Author.DisplayName != owner.DisplayName {
		t.Errorf("GetByID returned Author.DisplayName %q, want %q", got.Author.DisplayName, owner.DisplayName)
	}
}

func TestPostRepo_Create_DefaultPostType(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "postdefault-1")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "postdefault-1")

	post := &models.Post{
		CommunityID: community.ID,
		AuthorID:    owner.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "Default Post Type",
		Body:        "No post_type set",
		// PostType intentionally empty
	}

	created, err := postRepo.Create(ctx, post)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.PostType != models.PostTypeText {
		t.Errorf("expected default post_type 'text', got %q", created.PostType)
	}
}

func TestPostRepo_ListByCommunity_SortNew(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sortnew-1")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "sortnew-1")

	// Create three posts in order
	titles := []string{"First Post", "Second Post", "Third Post"}
	for _, title := range titles {
		createTestPost(t, postRepo, ctx, community.ID, owner.ID, title)
	}

	posts, total, err := postRepo.ListByCommunity(ctx, community.ID, "new", "", 10, 0)
	if err != nil {
		t.Fatalf("ListByCommunity (new): %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(posts))
	}
	// "new" sort: most recently created first
	if posts[0].Title != "Third Post" {
		t.Errorf("expected first post 'Third Post', got %q", posts[0].Title)
	}
	if posts[2].Title != "First Post" {
		t.Errorf("expected last post 'First Post', got %q", posts[2].Title)
	}
}

func TestPostRepo_ListByCommunity_SortTop(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "sorttop-1")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "sorttop-1")

	// Create posts (all will have vote_score 0)
	for _, title := range []string{"Post A", "Post B", "Post C"} {
		createTestPost(t, postRepo, ctx, community.ID, owner.ID, title)
	}

	posts, total, err := postRepo.ListByCommunity(ctx, community.ID, "top", "", 10, 0)
	if err != nil {
		t.Fatalf("ListByCommunity (top): %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(posts))
	}
	// All have same vote_score=0, tiebreak by created_at DESC
	// So the last created ("Post C") should be first
	if posts[0].Title != "Post C" {
		t.Errorf("expected first post 'Post C' (tiebreak by created_at), got %q", posts[0].Title)
	}
}

func TestPostRepo_ListGlobal(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "global-1")
	commA := createTestCommunity(t, cRepo, ctx, owner.ID, "global-a")
	commB := createTestCommunity(t, cRepo, ctx, owner.ID, "global-b")

	// Create posts across two communities
	createTestPost(t, postRepo, ctx, commA.ID, owner.ID, "Post in A")
	createTestPost(t, postRepo, ctx, commA.ID, owner.ID, "Another Post in A")
	createTestPost(t, postRepo, ctx, commB.ID, owner.ID, "Post in B")

	posts, total, err := postRepo.ListGlobal(ctx, "new", "", 10, 0)
	if err != nil {
		t.Fatalf("ListGlobal: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(posts))
	}

	// Verify that posts from both communities are present
	communityIDs := make(map[string]bool)
	for _, p := range posts {
		communityIDs[p.CommunityID] = true
		if p.Author.ID == "" {
			t.Errorf("post %q has empty Author.ID", p.ID)
		}
	}
	if !communityIDs[commA.ID] {
		t.Error("expected posts from community A in global list")
	}
	if !communityIDs[commB.ID] {
		t.Error("expected posts from community B in global list")
	}
}

func TestPostRepo_CreateWithMetadata(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "posts", "communities", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	cRepo := repository.NewCommunityRepo(pool)
	postRepo := repository.NewPostRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "metadata-owner-1")
	community := createTestCommunity(t, cRepo, ctx, owner.ID, "metadata-comm-1")

	post, err := postRepo.Create(ctx, &models.Post{
		CommunityID: community.ID,
		AuthorID:    owner.ID,
		AuthorType:  models.ParticipantHuman,
		Title:       "How does quantum error correction work?",
		Body:        "Looking for a technical explanation",
		PostType:    models.PostTypeQuestion,
		Metadata:    map[string]any{"expected_format": "technical"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if post.PostType != models.PostTypeQuestion {
		t.Errorf("expected post_type question, got %s", post.PostType)
	}
	// Verify metadata round-trip via GetByID
	got, err := postRepo.GetByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.PostType != models.PostTypeQuestion {
		t.Errorf("GetByID: expected post_type question, got %s", got.PostType)
	}
	if got.Metadata == nil {
		t.Error("expected non-nil metadata from GetByID")
	} else if got.Metadata["expected_format"] != "technical" {
		t.Errorf("expected metadata[expected_format]=technical, got %v", got.Metadata["expected_format"])
	}
}
