package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/activitypub"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

type fakeInboxRemoteTrust struct {
	score float64
}

func (f *fakeInboxRemoteTrust) RecordInteraction(context.Context, string, string) error { return nil }
func (f *fakeInboxRemoteTrust) StoreAttestation(context.Context, string, string, float64, time.Time) error {
	return nil
}
func (f *fakeInboxRemoteTrust) Get(_ context.Context, uri string) (*activitypub.RemoteTrust, error) {
	return &activitypub.RemoteTrust{RemoteActorURI: uri, LocalScore: f.score}, nil
}

func TestInboxIngestsSignedRemoteReplyAndTrustWeightedLikeIdempotently(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"votes", "ap_remote_actors", "ap_remote_trust", "comments", "posts", "communities",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	now := time.Now().UnixNano()
	owner, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant: models.Participant{DisplayName: fmt.Sprintf("Inbox Owner %d", now)},
		Email:       fmt.Sprintf("inbox-owner-%d@example.com", now), PasswordHash: "test-hash",
	})
	if err != nil {
		t.Fatalf("create local owner: %v", err)
	}
	community, err := repository.NewCommunityRepo(pool).Create(ctx, &models.Community{
		Name: "Inbox federation", Slug: fmt.Sprintf("inbox-federation-%d", now), CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	posts := repository.NewPostRepo(pool)
	post, err := posts.Create(ctx, &models.Post{
		CommunityID: community.ID, AuthorID: owner.ID, AuthorType: models.ParticipantHuman,
		Title: "Inbox target", Body: "Local body", PostType: models.PostTypeText,
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	cfg := &config.Config{Email: config.EmailConfig{SiteURL: "https://loomfeed.example"}}
	store := activitypub.NewStore(pool)
	localActor, err := store.EnsureHandleAndKey(ctx, owner.ID)
	if err != nil {
		t.Fatalf("materialize local actor: %v", err)
	}
	remoteURI := "https://remote.example/users/alice"
	remote := &activitypub.RemoteActor{ID: remoteURI, Type: "Person", Name: "Alice", PreferredUsername: "alice", Inbox: remoteURI + "/inbox"}
	remote.Icon.URL = "https://remote.example/alice.png"
	inbox := NewInboxHandler(
		store, activitypub.NewFollowersRepo(pool), &fakeInboxRemoteTrust{score: 80},
		activitypub.NewInboundRepo(pool), NewActivityPubHandler(store, pool, cfg),
	)
	inbox.verify = func(_ *http.Request, _ []byte) (string, error) { return remoteURI + "#main-key", nil }
	inbox.fetchRemote = func(_ context.Context, uri string) (*activitypub.RemoteActor, error) {
		if uri != remoteURI {
			return nil, fmt.Errorf("unexpected remote URI %s", uri)
		}
		return remote, nil
	}

	postURL := "https://loomfeed.example/post/" + post.ID
	send := func(activity map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(activity)
		req := httptest.NewRequest(http.MethodPost, "/users/"+localActor.Handle+"/inbox", bytesReader(body))
		req.SetPathValue("handle", localActor.Handle)
		rec := httptest.NewRecorder()
		inbox.Inbox(rec, req)
		return rec
	}
	create := map[string]any{
		"id": "https://remote.example/activities/create-1", "type": "Create", "actor": remoteURI,
		"object": map[string]any{
			"id": "https://remote.example/notes/1", "type": "Note", "attributedTo": remoteURI,
			"inReplyTo": postURL, "content": "<p>Hello &amp; <strong>Loomfeed</strong></p><script>alert(1)</script>",
		},
	}
	if rec := send(create); rec.Code != http.StatusAccepted {
		t.Fatalf("Create Note status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := send(create); rec.Code != http.StatusAccepted {
		t.Fatalf("duplicate Create Note status=%d body=%s", rec.Code, rec.Body.String())
	}
	var commentID, body, authorType string
	if err := pool.QueryRow(ctx, `SELECT id, body, author_type::text FROM comments WHERE federated_object_id = $1`, "https://remote.example/notes/1").Scan(&commentID, &body, &authorType); err != nil {
		t.Fatalf("load remote comment: %v", err)
	}
	if body != "Hello & Loomfeed" || authorType != "remote" {
		t.Fatalf("remote comment body=%q author_type=%q", body, authorType)
	}
	reloadedPost, _ := posts.GetByID(ctx, post.ID)
	if reloadedPost.CommentCount != 1 {
		t.Fatalf("duplicate inbox delivery changed comment_count to %d", reloadedPost.CommentCount)
	}

	like := map[string]any{
		"id": "https://remote.example/activities/like-1", "type": "Like", "actor": remoteURI, "object": postURL,
	}
	if rec := send(like); rec.Code != http.StatusAccepted {
		t.Fatalf("Like status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := send(like); rec.Code != http.StatusAccepted {
		t.Fatalf("duplicate Like status=%d body=%s", rec.Code, rec.Body.String())
	}
	var weight float64
	if err := pool.QueryRow(ctx, `SELECT weight FROM votes WHERE federated_activity_id = $1`, "https://remote.example/activities/like-1").Scan(&weight); err != nil || weight != 0.8 {
		t.Fatalf("stored Like weight=%v err=%v, want 0.8", weight, err)
	}
	reloadedPost, _ = posts.GetByID(ctx, post.ID)
	if reloadedPost.VoteScore != 1 {
		t.Fatalf("weighted remote Like score=%d, want rounded score 1", reloadedPost.VoteScore)
	}
}

// bytesReader keeps request construction readable without sharing mutable
// buffers across duplicate-delivery calls.
func bytesReader(body []byte) *bytes.Reader { return bytes.NewReader(body) }
