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

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/activitypub"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

type staticRemoteActorResolver struct {
	actor *activitypub.RemoteActor
}

func (r staticRemoteActorResolver) Resolve(context.Context, string) (*activitypub.RemoteActor, error) {
	return r.actor, nil
}

func TestOutboundFollowAcceptFollowingCollectionAndUndo(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"ap_outbound_follows", "ap_remote_actor_cache", "ap_followers", "votes",
		"ap_remote_actors", "ap_remote_trust", "comments", "posts", "communities",
		"api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	now := time.Now().UnixNano()
	local, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant: models.Participant{DisplayName: fmt.Sprintf("Federation Follow %d", now)},
		Email:       fmt.Sprintf("federation-follow-%d@example.com", now), PasswordHash: "test-hash",
	})
	if err != nil {
		t.Fatalf("create local participant: %v", err)
	}
	store := activitypub.NewStore(pool)
	localActor, err := store.EnsureHandleAndKey(ctx, local.ID)
	if err != nil {
		t.Fatalf("materialize local actor: %v", err)
	}
	remoteURI := "https://remote.example/users/alice"
	remote := &activitypub.RemoteActor{
		ID: remoteURI, Type: "Person", Name: "Alice", PreferredUsername: "alice",
		Inbox: remoteURI + "/inbox",
	}
	cfg := &config.Config{Email: config.EmailConfig{SiteURL: "https://loomfeed.example"}}
	repo := activitypub.NewOutboundFollowRepo(pool)
	handler := NewFederationFollowHandler(repo, store, staticRemoteActorResolver{actor: remote}, cfg)
	delivered := []map[string]any{}
	handler.deliver = func(_ context.Context, inbox, _, _ string, activity map[string]any) error {
		if inbox != remote.Inbox {
			return fmt.Errorf("unexpected inbox %s", inbox)
		}
		delivered = append(delivered, activity)
		return nil
	}

	authedRequest := func(method, target string, body any) *http.Request {
		t.Helper()
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		}
		req := httptest.NewRequest(method, target, reader)
		claims := &auth.Claims{ParticipantID: local.ID, ParticipantType: string(models.ParticipantHuman)}
		return req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
	}

	rec := httptest.NewRecorder()
	handler.Follow(rec, authedRequest(http.MethodPost, "/api/v1/federation/follows", map[string]string{"actor": "@alice@remote.example"}))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("outbound Follow status=%d body=%s", rec.Code, rec.Body.String())
	}
	follows, err := repo.ListByLocal(ctx, local.ID)
	if err != nil || len(follows) != 1 || follows[0].Status != activitypub.OutboundFollowPending || len(delivered) != 1 {
		t.Fatalf("pending follow=%#v deliveries=%d err=%v", follows, len(delivered), err)
	}
	followActivityID, _ := delivered[0]["id"].(string)
	if followActivityID == "" || followActivityID != follows[0].ActivityID || delivered[0]["type"] != "Follow" {
		t.Fatalf("delivered Follow=%#v persisted=%#v", delivered[0], follows[0])
	}

	// A validly signed Accept from a different actor must not claim Alice's
	// pending relationship.
	apHandler := NewActivityPubHandler(store, pool, cfg)
	inbox := NewInboxHandler(
		store, activitypub.NewFollowersRepo(pool), &fakeInboxRemoteTrust{score: 5},
		activitypub.NewInboundRepo(pool), apHandler,
	)
	inbox.WithOutboundFollows(repo)
	signingActor := "https://attacker.example/users/mallory"
	inbox.verify = func(_ *http.Request, _ []byte) (string, error) { return signingActor + "#main-key", nil }
	inbox.fetchRemote = func(_ context.Context, uri string) (*activitypub.RemoteActor, error) {
		return &activitypub.RemoteActor{ID: uri, Inbox: uri + "/inbox"}, nil
	}
	sendAccept := func(actorURI string) *httptest.ResponseRecorder {
		t.Helper()
		accept := map[string]any{
			"id": actorURI + "/accept/1", "type": "Accept", "actor": actorURI,
			"object": delivered[0],
		}
		body, _ := json.Marshal(accept)
		req := httptest.NewRequest(http.MethodPost, "/users/"+localActor.Handle+"/inbox", bytes.NewReader(body))
		req.SetPathValue("handle", localActor.Handle)
		recorder := httptest.NewRecorder()
		inbox.Inbox(recorder, req)
		return recorder
	}
	if rec := sendAccept(signingActor); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-actor Accept status=%d body=%s", rec.Code, rec.Body.String())
	}
	pending, _ := repo.GetOwned(ctx, local.ID, follows[0].ID)
	if pending.Status != activitypub.OutboundFollowPending {
		t.Fatalf("cross-actor Accept changed status to %s", pending.Status)
	}

	signingActor = remoteURI
	if rec := sendAccept(remoteURI); rec.Code != http.StatusAccepted {
		t.Fatalf("valid Accept status=%d body=%s", rec.Code, rec.Body.String())
	}
	accepted, _ := repo.GetOwned(ctx, local.ID, follows[0].ID)
	if accepted.Status != activitypub.OutboundFollowAccepted || accepted.AcceptedAt == nil {
		t.Fatalf("valid Accept did not settle follow: %#v", accepted)
	}

	followingReq := httptest.NewRequest(http.MethodGet, "/users/"+localActor.Handle+"/following", nil)
	followingReq.SetPathValue("handle", localActor.Handle)
	followingRec := httptest.NewRecorder()
	handler.Following(followingRec, followingReq)
	if followingRec.Code != http.StatusOK || !bytes.Contains(followingRec.Body.Bytes(), []byte(remoteURI)) {
		t.Fatalf("following collection status=%d body=%s", followingRec.Code, followingRec.Body.String())
	}

	undoReq := authedRequest(http.MethodDelete, "/api/v1/federation/follows/"+accepted.ID, nil)
	undoReq.SetPathValue("id", accepted.ID)
	undoRec := httptest.NewRecorder()
	handler.Unfollow(undoRec, undoReq)
	if undoRec.Code != http.StatusNoContent || len(delivered) != 2 || delivered[1]["type"] != "Undo" {
		t.Fatalf("Undo status=%d body=%s delivered=%#v", undoRec.Code, undoRec.Body.String(), delivered)
	}
	if _, err := repo.GetOwned(ctx, local.ID, accepted.ID); err != pgx.ErrNoRows {
		t.Fatalf("outbound follow survived successful Undo: %v", err)
	}
}
