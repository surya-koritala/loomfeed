package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/activitypub"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
)

type remoteActorResolver interface {
	Resolve(context.Context, string) (*activitypub.RemoteActor, error)
}

type activityDeliverer func(context.Context, string, string, string, map[string]any) error

// FederationFollowHandler owns the local-user side of ActivityPub follows.
// The relationship is durable before delivery, so an HTTP retry reuses the
// same Follow activity ID and a later inbox Accept can be correlated.
type FederationFollowHandler struct {
	follows  *activitypub.OutboundFollowRepo
	store    *activitypub.Store
	resolver remoteActorResolver
	cfg      *config.Config
	deliver  activityDeliverer
}

func NewFederationFollowHandler(follows *activitypub.OutboundFollowRepo, store *activitypub.Store, resolver remoteActorResolver, cfg *config.Config) *FederationFollowHandler {
	return &FederationFollowHandler{
		follows: follows, store: store, resolver: resolver, cfg: cfg,
		deliver: activitypub.Deliver,
	}
}

type outboundFollowRequest struct {
	Actor string `json:"actor"`
}

func (h *FederationFollowHandler) originURL() string {
	return strings.TrimRight(h.cfg.Email.SiteURL, "/")
}

func (h *FederationFollowHandler) Follow(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var request outboundFollowRequest
	if err := api.Decode(r, &request); err != nil || strings.TrimSpace(request.Actor) == "" {
		api.Error(w, http.StatusBadRequest, "actor is required")
		return
	}
	remote, err := h.resolver.Resolve(r.Context(), request.Actor)
	if err != nil {
		api.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if remote == nil || remote.ID == "" || remote.Inbox == "" {
		api.Error(w, http.StatusBadRequest, "remote actor is missing id or inbox")
		return
	}
	if sameOrigin(h.originURL(), remote.ID) {
		api.Error(w, http.StatusBadRequest, "use local follows for actors on this instance")
		return
	}
	local, err := h.store.EnsureHandleAndKey(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load local ActivityPub actor")
		return
	}
	actorURL := fmt.Sprintf("%s/users/%s", h.originURL(), local.Handle)
	candidateActivityID := actorURL + "/follows/" + uuid.NewString()
	follow, _, err := h.follows.Ensure(
		r.Context(), local.ID, remote.ID, remote.Inbox, candidateActivityID,
	)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to persist outbound Follow")
		return
	}
	if follow.Status == activitypub.OutboundFollowAccepted {
		api.JSON(w, http.StatusOK, follow)
		return
	}
	privateKey, err := h.store.PrivateKeyPEM(r.Context(), local.ID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load local ActivityPub key")
		return
	}
	activity := followActivity(follow.ActivityID, actorURL, follow.RemoteActorURI, follow.CreatedAt)
	deliveryErr := h.deliver(r.Context(), follow.RemoteInboxURI, actorURL+"#main-key", privateKey, activity)
	if err := h.follows.RecordDelivery(r.Context(), follow.ID, deliveryErr); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to record outbound Follow delivery")
		return
	}
	if deliveryErr != nil {
		api.Error(w, http.StatusBadGateway, "remote inbox rejected the Follow; retry is safe")
		return
	}
	refreshed, err := h.follows.GetOwned(r.Context(), local.ID, follow.ID)
	if err == nil {
		follow = refreshed
	}
	api.JSON(w, http.StatusAccepted, follow)
}

func (h *FederationFollowHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	follows, err := h.follows.ListByLocal(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list outbound follows")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"data": follows, "total": len(follows)})
}

func (h *FederationFollowHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	follow, err := h.follows.GetOwned(r.Context(), claims.ParticipantID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "outbound follow not found")
		} else {
			api.Error(w, http.StatusInternalServerError, "failed to load outbound follow")
		}
		return
	}
	local, err := h.store.EnsureHandleAndKey(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load local ActivityPub actor")
		return
	}
	privateKey, err := h.store.PrivateKeyPEM(r.Context(), local.ID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load local ActivityPub key")
		return
	}
	actorURL := fmt.Sprintf("%s/users/%s", h.originURL(), local.Handle)
	undo := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       actorURL + "/undo/" + uuid.NewString(),
		"type":     "Undo",
		"actor":    actorURL,
		"object":   followActivity(follow.ActivityID, actorURL, follow.RemoteActorURI, follow.CreatedAt),
		"to":       []string{follow.RemoteActorURI},
	}
	if err := h.deliver(r.Context(), follow.RemoteInboxURI, actorURL+"#main-key", privateKey, undo); err != nil {
		api.Error(w, http.StatusBadGateway, "remote inbox rejected the Undo; follow was retained")
		return
	}
	deleted, err := h.follows.DeleteOwned(r.Context(), local.ID, follow.ID)
	if err != nil || !deleted {
		api.Error(w, http.StatusInternalServerError, "failed to remove outbound follow")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Following serves the collection advertised in local actor documents.
func (h *FederationFollowHandler) Following(w http.ResponseWriter, r *http.Request) {
	local, err := h.store.ResolveHandle(r.Context(), strings.ToLower(r.PathValue("handle")))
	if err != nil || local == nil {
		api.Error(w, http.StatusNotFound, "no such actor")
		return
	}
	actorURIs, err := h.follows.ListAcceptedRemoteActorURIs(r.Context(), local.ID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list following")
		return
	}
	collectionID := fmt.Sprintf("%s/users/%s/following", h.originURL(), local.Handle)
	w.Header().Set("Content-Type", "application/activity+json")
	w.Header().Set("Cache-Control", "max-age=60")
	api.JSON(w, http.StatusOK, map[string]any{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           collectionID,
		"type":         "OrderedCollection",
		"totalItems":   len(actorURIs),
		"orderedItems": actorURIs,
	})
}

func followActivity(activityID, localActorURI, remoteActorURI string, createdAt time.Time) map[string]any {
	return map[string]any{
		"@context":  "https://www.w3.org/ns/activitystreams",
		"id":        activityID,
		"type":      "Follow",
		"actor":     localActorURI,
		"object":    remoteActorURI,
		"to":        []string{remoteActorURI},
		"published": createdAt.UTC().Format(time.RFC3339),
	}
}

func sameOrigin(origin, target string) bool {
	originURL, originErr := url.Parse(origin)
	targetURL, targetErr := url.Parse(target)
	return originErr == nil && targetErr == nil && originURL.Scheme != "" &&
		strings.EqualFold(originURL.Scheme, targetURL.Scheme) && strings.EqualFold(originURL.Host, targetURL.Host)
}
