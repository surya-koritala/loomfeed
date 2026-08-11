package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/surya-koritala/loomfeed/internal/activitypub"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/modfilter"
)

// InboxHandler exposes POST /users/{handle}/inbox plus the
// GET /users/{handle}/followers collection that Mastodon peeks at
// when validating Follow relationships.
//
// Supported activity types (everything else is Accepted-and-ignored
// so remote servers don't retry forever):
//   - Follow         → materialize actor, upsert follower, emit Accept
//   - Undo { Follow} → remove follower row
//   - Create { Note} → materialize a remote actor + local threaded comment
//   - Like           → materialize a trust-weighted remote vote
type InboxHandler struct {
	store       *activitypub.Store
	followers   *activitypub.FollowersRepo
	remoteTrust inboxRemoteTrust
	inbound     *activitypub.InboundRepo
	outbound    *activitypub.OutboundFollowRepo
	ap          *ActivityPubHandler // reuse origin/host helpers
	verify      func(*http.Request, []byte) (string, error)
	fetchRemote func(context.Context, string) (*activitypub.RemoteActor, error)
}

type inboxRemoteTrust interface {
	RecordInteraction(context.Context, string, string) error
	StoreAttestation(context.Context, string, string, float64, time.Time) error
	Get(context.Context, string) (*activitypub.RemoteTrust, error)
}

func NewInboxHandler(store *activitypub.Store, followers *activitypub.FollowersRepo, remoteTrust inboxRemoteTrust, inbound *activitypub.InboundRepo, ap *ActivityPubHandler) *InboxHandler {
	return &InboxHandler{
		store: store, followers: followers, remoteTrust: remoteTrust, inbound: inbound, ap: ap,
		verify: func(r *http.Request, body []byte) (string, error) {
			return activitypub.VerifyRequest(r, body, activitypub.ResolveKey)
		},
		fetchRemote: activitypub.FetchActorContext,
	}
}

func (h *InboxHandler) WithOutboundFollows(outbound *activitypub.OutboundFollowRepo) {
	h.outbound = outbound
}

func (h *InboxHandler) WithRemoteActorResolver(resolver remoteActorResolver) {
	if resolver != nil {
		h.fetchRemote = resolver.Resolve
	}
}

// activityEnvelope is the minimal shape we parse from the POST body.
// `Object` is a json.RawMessage because it can be a plain string
// (actor URI) or an embedded object.
type activityEnvelope struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Actor  string          `json:"actor"`
	Object json.RawMessage `json:"object"`
}

type activityNote struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	AttributedTo json.RawMessage `json:"attributedTo"`
	InReplyTo    json.RawMessage `json:"inReplyTo"`
	Content      string          `json:"content"`
}

// Inbox handles POST /users/{handle}/inbox.
func (h *InboxHandler) Inbox(w http.ResponseWriter, r *http.Request) {
	handle := strings.ToLower(r.PathValue("handle"))
	actor, err := h.store.ResolveHandle(r.Context(), handle)
	if err != nil || actor == nil {
		api.Error(w, http.StatusNotFound, "no such actor")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB cap
	if err != nil {
		api.Error(w, http.StatusBadRequest, "cannot read body")
		return
	}

	// HTTP signature verification. Failure is a hard 401 — unsigned or
	// mis-signed inbox POSTs are not processed.
	keyID, err := h.verify(r, body)
	if err != nil {
		slog.Warn("ap: signature verify failed", "handle", handle, "err", err)
		api.Error(w, http.StatusUnauthorized, "signature verification failed")
		return
	}

	var env activityEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid activity json")
		return
	}
	if env.Type == "" {
		api.Error(w, http.StatusBadRequest, "activity missing type")
		return
	}

	// Bind the key's actor to the activity's actor field — don't let a
	// signed request from one actor claim to act as another.
	signingActorURI := keyID
	if before, _, ok := strings.Cut(keyID, "#"); ok {
		signingActorURI = before
	}
	if env.Actor != "" && env.Actor != signingActorURI {
		api.Error(w, http.StatusForbidden, "actor mismatch between body and signature")
		return
	}
	if env.Actor == "" {
		env.Actor = signingActorURI
	}

	// Every verified activity from a remote actor counts as an
	// interaction for their trust row, regardless of how we handle
	// the activity below. Best-effort: a storage error here shouldn't
	// block the activity from being processed.
	if h.remoteTrust != nil {
		kind := env.Type
		if env.Type == "Create" {
			kind = "reply" // most Create{Note} in practice is a reply
		}
		if err := h.remoteTrust.RecordInteraction(r.Context(), env.Actor, strings.ToLower(kind)); err != nil {
			slog.Warn("ap: record remote trust failed", "actor", env.Actor, "err", err)
		}
		// Phase 3: if the remote actor doc carries a trust attestation,
		// verify it with the actor's public key (already fetched for
		// the HTTP signature check) and persist as advisory state.
		go h.ingestAttestation(env.Actor)
	}

	switch env.Type {
	case "Follow":
		h.handleFollow(r.Context(), w, actor, &env)
		return
	case "Undo":
		h.handleUndo(r.Context(), w, actor, &env, body)
		return
	case "Accept":
		h.handleAccept(r.Context(), w, actor, &env)
		return
	case "Create":
		h.handleCreate(r.Context(), w, &env)
		return
	case "Like":
		h.handleLike(r.Context(), w, &env)
		return
	default:
		// Ack-and-ignore everything else so retry-prone servers don't
		// hammer us. We still log for observability.
		slog.Info("ap: inbox activity ignored", "type", env.Type, "actor", env.Actor, "handle", handle)
		w.WriteHeader(http.StatusAccepted)
		return
	}
}

func rawObjectID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var direct string
	if json.Unmarshal(raw, &direct) == nil {
		return strings.TrimSpace(direct)
	}
	var object struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return strings.TrimSpace(object.ID)
	}
	return ""
}

func (h *InboxHandler) remoteProfile(ctx context.Context, actorURI string) (activitypub.RemoteActorProfile, error) {
	remote, err := h.fetchRemote(ctx, actorURI)
	if err != nil {
		return activitypub.RemoteActorProfile{}, fmt.Errorf("fetch remote actor: %w", err)
	}
	if remote == nil {
		return activitypub.RemoteActorProfile{}, fmt.Errorf("fetch remote actor: empty actor document")
	}
	trust := 5.0
	if h.remoteTrust != nil {
		if current, trustErr := h.remoteTrust.Get(ctx, actorURI); trustErr == nil {
			trust = current.LocalScore
		}
	}
	return activitypub.RemoteActorProfile{
		URI: actorURI, PreferredUsername: remote.PreferredUsername,
		DisplayName: remote.Name, AvatarURL: remote.Icon.URL,
		ActorType: remote.Type, InboxURI: remote.Inbox, LocalTrust: trust,
	}, nil
}

func (h *InboxHandler) handleCreate(ctx context.Context, w http.ResponseWriter, env *activityEnvelope) {
	if h.inbound == nil {
		api.Error(w, http.StatusServiceUnavailable, "federated content store unavailable")
		return
	}
	var note activityNote
	if err := json.Unmarshal(env.Object, &note); err != nil || note.Type != "Note" {
		// Create can wrap many ActivityStreams object types. Unsupported ones
		// are acknowledged without being mistaken for comments.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if env.ID == "" || note.ID == "" {
		api.Error(w, http.StatusBadRequest, "Create and Note ids are required")
		return
	}
	if attributedTo := rawObjectID(note.AttributedTo); attributedTo != "" && attributedTo != env.Actor {
		api.Error(w, http.StatusForbidden, "Note attributedTo does not match activity actor")
		return
	}
	replyTo := rawObjectID(note.InReplyTo)
	if replyTo == "" {
		// Loomfeed does not ingest standalone remote posts into an arbitrary
		// community; only replies to a known local object are in scope.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	target, err := activitypub.ResolveLocalTarget(h.ap.originURL(), replyTo)
	if err != nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	body, err := activitypub.PlainTextContent(note.Content, 10000)
	if err != nil {
		api.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if result := modfilter.Check(body); result.Severity >= modfilter.SeverityFlag {
		slog.Info("ap: remote reply rejected by content filter", "actor", env.Actor, "category", result.Category)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	profile, err := h.remoteProfile(ctx, env.Actor)
	if err != nil {
		api.Error(w, http.StatusBadRequest, "cannot load remote actor")
		return
	}
	_, _, err = h.inbound.IngestReply(ctx, activitypub.InboundReply{
		ActivityID: env.ID, ObjectID: note.ID, Actor: profile,
		PostID: target.PostID, ParentCommentID: target.ParentCommentID, Body: body,
	})
	if err != nil {
		slog.Error("ap: ingest remote reply failed", "actor", env.Actor, "note", note.ID, "err", err)
		api.Error(w, http.StatusInternalServerError, "failed to store remote reply")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *InboxHandler) handleLike(ctx context.Context, w http.ResponseWriter, env *activityEnvelope) {
	if h.inbound == nil {
		api.Error(w, http.StatusServiceUnavailable, "federated content store unavailable")
		return
	}
	if env.ID == "" {
		api.Error(w, http.StatusBadRequest, "Like id is required")
		return
	}
	objectID := rawObjectID(env.Object)
	target, err := activitypub.ResolveLocalTarget(h.ap.originURL(), objectID)
	if err != nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	profile, err := h.remoteProfile(ctx, env.Actor)
	if err != nil {
		api.Error(w, http.StatusBadRequest, "cannot load remote actor")
		return
	}
	_, _, err = h.inbound.IngestLike(ctx, activitypub.InboundLike{
		ActivityID: env.ID, Actor: profile, TargetID: target.TargetID,
		TargetType: target.TargetType, Weight: activitypub.TrustWeight(profile.LocalTrust),
	})
	if err != nil {
		slog.Error("ap: ingest remote Like failed", "actor", env.Actor, "object", objectID, "err", err)
		api.Error(w, http.StatusInternalServerError, "failed to store remote Like")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *InboxHandler) handleFollow(ctx context.Context, w http.ResponseWriter, localActor *activitypub.Actor, env *activityEnvelope) {
	// `object` on a Follow is our local actor URI. We already looked
	// the handle up, so no need to cross-check; just record the follow.
	remote, err := h.fetchRemote(ctx, env.Actor)
	if err != nil || remote == nil {
		slog.Warn("ap: fetch remote actor failed", "actor", env.Actor, "err", err)
		api.Error(w, http.StatusBadRequest, "cannot fetch remote actor")
		return
	}
	if remote.Inbox == "" {
		api.Error(w, http.StatusBadRequest, "remote actor has no inbox")
		return
	}
	if err := h.followers.Upsert(ctx, localActor.ID, remote.ID, remote.Inbox, remote.Endpoints.SharedInbox); err != nil {
		slog.Error("ap: upsert follower failed", "err", err)
		api.Error(w, http.StatusInternalServerError, "failed to store follower")
		return
	}

	// Emit Accept back to the remote's inbox. Do this in a goroutine so
	// the incoming request returns quickly — the spec allows either.
	go h.sendAccept(localActor, remote, env)

	w.WriteHeader(http.StatusAccepted)
}

func (h *InboxHandler) handleAccept(ctx context.Context, w http.ResponseWriter, localActor *activitypub.Actor, env *activityEnvelope) {
	if h.outbound == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	activityID := rawObjectID(env.Object)
	var directObject string
	if json.Unmarshal(env.Object, &directObject) != nil {
		var followed activityEnvelope
		if err := json.Unmarshal(env.Object, &followed); err != nil || followed.Type != "Follow" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		localActorURI := fmt.Sprintf("%s/users/%s", h.ap.originURL(), localActor.Handle)
		if followed.Actor != localActorURI {
			api.Error(w, http.StatusForbidden, "Accept does not reference this local actor")
			return
		}
		if remoteActorURI := rawObjectID(followed.Object); remoteActorURI != env.Actor {
			api.Error(w, http.StatusForbidden, "Accept actor does not match Follow object")
			return
		}
		activityID = followed.ID
	}
	if activityID == "" {
		api.Error(w, http.StatusBadRequest, "Accept is missing Follow activity id")
		return
	}
	if _, err := h.outbound.Accept(ctx, localActor.ID, env.Actor, activityID); err != nil {
		slog.Error("ap: accept outbound Follow failed", "actor", env.Actor, "activity_id", activityID, "err", err)
		api.Error(w, http.StatusInternalServerError, "failed to accept outbound Follow")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *InboxHandler) handleUndo(ctx context.Context, w http.ResponseWriter, localActor *activitypub.Actor, env *activityEnvelope, _ []byte) {
	// Parse the inner object for a Follow we should undo.
	var inner activityEnvelope
	if len(env.Object) > 0 {
		_ = json.Unmarshal(env.Object, &inner)
	}
	if inner.Type == "Follow" {
		if err := h.followers.Remove(ctx, localActor.ID, env.Actor); err != nil {
			slog.Warn("ap: remove follower failed", "err", err)
		}
	}
	w.WriteHeader(http.StatusAccepted)
}

// sendAccept POSTs an Accept activity back to the remote actor's
// inbox, signed with the local actor's key.
func (h *InboxHandler) sendAccept(localActor *activitypub.Actor, remote *activitypub.RemoteActor, original *activityEnvelope) {
	origin := h.ap.originURL()
	localActorURL := fmt.Sprintf("%s/users/%s", origin, localActor.Handle)

	accept := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       fmt.Sprintf("%s#accepts/%s", localActorURL, uuid.New().String()),
		"type":     "Accept",
		"actor":    localActorURL,
		"object": map[string]any{
			"id":     original.ID,
			"type":   "Follow",
			"actor":  original.Actor,
			"object": localActorURL,
		},
	}

	// Load the actor's private key — same row used to publish the
	// actor document.
	a, err := h.store.EnsureHandleAndKey(context.Background(), localActor.ID)
	if err != nil {
		slog.Warn("ap: load actor key failed", "err", err)
		return
	}
	privPEM, err := h.store.PrivateKeyPEM(context.Background(), localActor.ID)
	if err != nil {
		slog.Warn("ap: load private key failed", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	keyID := fmt.Sprintf("%s#main-key", localActorURL)
	_ = a // a.PublicKey available if we ever need it
	if err := activitypub.Deliver(ctx, remote.Inbox, keyID, privPEM, accept); err != nil {
		slog.Warn("ap: send Accept failed", "inbox", remote.Inbox, "err", err)
	}
}

// ingestAttestation pulls the trust block off the actor document
// (already cached from the HTTP-signature verify step), checks:
//   - issuer matches the actor's origin
//   - issuedAt is within 30 days
//   - signature is valid under the actor's publicKey
//
// ...and stores the advisory score. Runs on a background goroutine —
// per-actor failures are logged and swallowed.
func (h *InboxHandler) ingestAttestation(remoteActorURI string) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	actor, err := h.fetchRemote(ctx, remoteActorURI)
	if err != nil || actor == nil || actor.Trust == nil {
		return
	}
	att := actor.Trust
	if att.Signature == "" || att.Issuer == "" || att.IssuedAt == "" {
		return
	}

	// Issuer must match the actor's origin (strip to scheme+host).
	u, err := url.Parse(actor.ID)
	if err != nil {
		return
	}
	origin := u.Scheme + "://" + u.Host
	if !strings.HasPrefix(att.Issuer, origin) {
		slog.Info("ap: attestation issuer mismatch, ignoring", "actor", actor.ID, "claimed_issuer", att.Issuer)
		return
	}

	// Age check — 30 days per docs/FEDIVERSE_TRUST.md.
	issuedAt, err := time.Parse(time.RFC3339, att.IssuedAt)
	if err != nil || time.Since(issuedAt) > 30*24*time.Hour {
		return
	}

	// Signature check using the actor's published public key.
	if actor.PublicKey.PublicKeyPem == "" {
		return
	}
	if err := activitypub.VerifyAttestation(att.Issuer, att.IssuedAt, att.Scale, att.Score, att.Signature, actor.PublicKey.PublicKeyPem); err != nil {
		slog.Info("ap: attestation verify failed", "actor", actor.ID, "err", err)
		return
	}

	if err := h.remoteTrust.StoreAttestation(ctx, actor.ID, att.Issuer, att.Score, issuedAt); err != nil {
		slog.Warn("ap: store attestation failed", "actor", actor.ID, "err", err)
	}
}

// TrustLookup handles GET /api/v1/remote-trust?uri=<encoded>.
// Returns the row we have for a remote actor — both our computed
// local score and any verified advisory from their home instance.
// Returns 404 with zero-values if nothing has been recorded.
func (h *InboxHandler) TrustLookup(w http.ResponseWriter, r *http.Request) {
	uri := strings.TrimSpace(r.URL.Query().Get("uri"))
	if uri == "" {
		api.Error(w, http.StatusBadRequest, "uri query param required")
		return
	}
	trust, err := h.remoteTrust.Get(r.Context(), uri)
	if err != nil {
		// Distinguish "no row" from real errors by the default row
		// shape — Get returns err on both. We want the 404 behaviour
		// so clients can render a neutral state.
		api.Error(w, http.StatusNotFound, "no trust data for that actor")
		return
	}
	api.JSON(w, http.StatusOK, trust)
}

// Followers handles GET /users/{handle}/followers — returns an
// OrderedCollection with the count plus the first page inline.
func (h *InboxHandler) Followers(w http.ResponseWriter, r *http.Request) {
	handle := strings.ToLower(r.PathValue("handle"))
	actor, err := h.store.ResolveHandle(r.Context(), handle)
	if err != nil || actor == nil {
		api.Error(w, http.StatusNotFound, "no such actor")
		return
	}
	origin := h.ap.originURL()
	collectionID := fmt.Sprintf("%s/users/%s/followers", origin, actor.Handle)

	items, total, err := h.followers.ListFollowers(r.Context(), actor.ID, 80, 0)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list followers")
		return
	}

	w.Header().Set("Content-Type", "application/activity+json")
	api.JSON(w, http.StatusOK, map[string]any{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           collectionID,
		"type":         "OrderedCollection",
		"totalItems":   total,
		"orderedItems": items,
	})
}
