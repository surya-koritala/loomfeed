package activitypub_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/activitypub"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

func TestDeliverRejectsPrivateInboxBeforeNetworkAccess(t *testing.T) {
	err := activitypub.Deliver(
		context.Background(), "http://127.0.0.1:8080/inbox", "https://loomfeed.example/users/alice#main-key",
		"not-needed-for-a-blocked-target", map[string]any{"type": "Follow"},
	)
	if err == nil {
		t.Fatal("private inbox URL must be rejected")
	}
}

func TestRemoteActorCacheAndOutboundFollowLifecycleAreDurable(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"ap_outbound_follows", "ap_remote_actor_cache", "api_keys", "agent_identities", "human_users", "participants",
	)
	ctx := context.Background()
	participants := repository.NewParticipantRepo(pool)
	now := time.Now().UnixNano()
	local, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant: models.Participant{DisplayName: fmt.Sprintf("Outbound Follow %d", now)},
		Email:       fmt.Sprintf("outbound-follow-%d@example.com", now), PasswordHash: "test-hash",
	})
	if err != nil {
		t.Fatalf("create local actor: %v", err)
	}

	actor := &activitypub.RemoteActor{
		ID: "https://remote.example/users/alice", Type: "Person", Name: "Alice",
		PreferredUsername: "alice", Inbox: "https://remote.example/users/alice/inbox",
	}
	actor.Endpoints.SharedInbox = "https://remote.example/inbox"
	resolver := activitypub.NewActorResolver(
		pool,
		activitypub.WithWebFingerLookup(func(_ context.Context, acct string) (string, error) {
			if acct != "acct:alice@remote.example" {
				return "", fmt.Errorf("unexpected acct %s", acct)
			}
			return actor.ID, nil
		}),
		activitypub.WithRemoteActorFetcher(func(_ context.Context, actorURI string) (*activitypub.RemoteActor, error) {
			if actorURI != actor.ID {
				return nil, fmt.Errorf("unexpected actor URI %s", actorURI)
			}
			return actor, nil
		}),
	)
	discovered, err := resolver.Resolve(ctx, "@Alice@remote.example")
	if err != nil || discovered.ID != actor.ID {
		t.Fatalf("resolve remote acct: actor=%#v err=%v", discovered, err)
	}
	if _, _, err := activitypub.ParseRemoteActorReference("@alice@localhost"); err == nil {
		t.Fatal("private/local WebFinger host must be rejected")
	}
	cache := activitypub.NewRemoteActorCache(pool, time.Hour)
	if err := cache.Put(ctx, "acct:alice@remote.example", actor); err != nil {
		t.Fatalf("cache remote actor: %v", err)
	}
	// A new cache instance proves the hit is PostgreSQL-backed rather than
	// relying on the process-local cache in FetchActor.
	reloadedCache := activitypub.NewRemoteActorCache(pool, time.Hour)
	cached, found, err := reloadedCache.GetByAcct(ctx, "acct:alice@remote.example")
	if err != nil || !found || cached.ID != actor.ID || cached.Inbox != actor.Inbox {
		t.Fatalf("reload cached actor: actor=%#v found=%v err=%v", cached, found, err)
	}

	follows := activitypub.NewOutboundFollowRepo(pool)
	follow, created, err := follows.Ensure(ctx, local.ID, actor.ID, actor.Inbox, "https://loomfeed.example/follows/one")
	if err != nil || !created || follow.Status != activitypub.OutboundFollowPending {
		t.Fatalf("create outbound follow: follow=%#v created=%v err=%v", follow, created, err)
	}
	duplicate, created, err := follows.Ensure(ctx, local.ID, actor.ID, "https://remote.example/new-inbox", "https://loomfeed.example/follows/must-not-replace")
	if err != nil || created || duplicate.ID != follow.ID || duplicate.ActivityID != follow.ActivityID || duplicate.RemoteInboxURI != "https://remote.example/new-inbox" {
		t.Fatalf("idempotent outbound follow: follow=%#v created=%v err=%v", duplicate, created, err)
	}
	if accepted, err := follows.Accept(ctx, local.ID, "https://attacker.example/users/mallory", follow.ActivityID); err != nil || accepted {
		t.Fatalf("cross-actor Accept accepted=%v err=%v", accepted, err)
	}
	if accepted, err := follows.Accept(ctx, local.ID, actor.ID, follow.ActivityID); err != nil || !accepted {
		t.Fatalf("valid Accept accepted=%v err=%v", accepted, err)
	}
	accepted, err := follows.GetOwned(ctx, local.ID, follow.ID)
	if err != nil || accepted.Status != activitypub.OutboundFollowAccepted || accepted.AcceptedAt == nil {
		t.Fatalf("accepted follow state=%#v err=%v", accepted, err)
	}
	remoteURIs, err := follows.ListAcceptedRemoteActorURIs(ctx, local.ID)
	if err != nil || len(remoteURIs) != 1 || remoteURIs[0] != actor.ID {
		t.Fatalf("accepted following collection=%v err=%v", remoteURIs, err)
	}
}
