package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/activitypub"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/config"
)

// ActivityPubHandler exposes the outbound ActivityPub surface:
// webfinger, actor, outbox. Read-only — no Follow handling here (see
// #19 inbound for that).
type ActivityPubHandler struct {
	store *activitypub.Store
	pool  *pgxpool.Pool
	cfg   *config.Config
}

func NewActivityPubHandler(store *activitypub.Store, pool *pgxpool.Pool, cfg *config.Config) *ActivityPubHandler {
	return &ActivityPubHandler{store: store, pool: pool, cfg: cfg}
}

// originURL is the canonical instance URL used in id / URL fields on
// ActivityPub documents. Pulled from email.SiteURL since that's the
// existing "public base" setting.
func (h *ActivityPubHandler) originURL() string {
	return strings.TrimRight(h.cfg.Email.SiteURL, "/")
}

func (h *ActivityPubHandler) host() string {
	// Strip scheme to get the domain for acct: subjects.
	u := h.originURL()
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	return u
}

// Webfinger handles GET /.well-known/webfinger?resource=acct:NAME@HOST.
// RFC 7033. Looks up the handle and returns link rels pointing at the
// actor document.
func (h *ActivityPubHandler) Webfinger(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		api.Error(w, http.StatusBadRequest, "resource parameter required")
		return
	}
	// acct:handle@host — we accept host mismatches (many remotes send
	// bare names) and just extract whatever looks like a handle.
	s := strings.TrimPrefix(resource, "acct:")
	parts := strings.SplitN(s, "@", 2)
	handle := strings.ToLower(parts[0])

	actor, err := h.store.ResolveHandle(r.Context(), handle)
	if err != nil || actor == nil {
		api.Error(w, http.StatusNotFound, "no such actor")
		return
	}

	origin := h.originURL()
	actorURL := fmt.Sprintf("%s/users/%s", origin, actor.Handle)
	profileURL := fmt.Sprintf("%s/profile/%s", origin, actor.ID)

	w.Header().Set("Content-Type", "application/jrd+json; charset=utf-8")
	w.Header().Set("Cache-Control", "max-age=600")
	api.JSON(w, http.StatusOK, map[string]any{
		"subject": fmt.Sprintf("acct:%s@%s", actor.Handle, h.host()),
		"aliases": []string{actorURL, profileURL},
		"links": []map[string]any{
			{"rel": "self", "type": "application/activity+json", "href": actorURL},
			{"rel": "http://webfinger.net/rel/profile-page", "type": "text/html", "href": profileURL},
		},
	})
}

// Actor handles GET /users/{handle}. Returns an ActivityStreams v2
// Person actor document with a Mastodon-compatible publicKey block.
func (h *ActivityPubHandler) Actor(w http.ResponseWriter, r *http.Request) {
	handle := strings.ToLower(r.PathValue("handle"))
	actor, err := h.store.ResolveHandle(r.Context(), handle)
	if err != nil || actor == nil {
		api.Error(w, http.StatusNotFound, "no such actor")
		return
	}

	origin := h.originURL()
	actorURL := fmt.Sprintf("%s/users/%s", origin, actor.Handle)
	profileURL := fmt.Sprintf("%s/profile/%s", origin, actor.ID)

	doc := map[string]any{
		"@context": []any{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/v1",
			map[string]any{
				"lf":    "https://www.loomfeed.com/ns#",
				"trust": "lf:trust",
			},
		},
		"id":                actorURL,
		"type":              "Person",
		"preferredUsername": actor.Handle,
		"name":              actor.DisplayName,
		"summary":           actor.Bio,
		"url":               profileURL,
		"inbox":             actorURL + "/inbox",
		"outbox":            actorURL + "/outbox",
		"followers":         actorURL + "/followers",
		"following":         actorURL + "/following",
		"publicKey": map[string]any{
			"id":           actorURL + "#main-key",
			"owner":        actorURL,
			"publicKeyPem": actor.PublicKey,
		},
	}
	if actor.AvatarURL != "" {
		avatar := actor.AvatarURL
		if strings.HasPrefix(avatar, "/") {
			avatar = origin + avatar
		}
		doc["icon"] = map[string]any{"type": "Image", "url": avatar}
	}

	// Signed trust attestation — see docs/FEDIVERSE_TRUST.md.
	// Only emit when the score is meaningful (>0). Private key is
	// fetched via the store so we can sign per-actor.
	if actor.TrustScore > 0 {
		priv, perr := h.store.PrivateKeyPEM(r.Context(), actor.ID)
		if perr == nil {
			issuedAt := time.Now().UTC().Format(time.RFC3339)
			sig, serr := activitypub.SignAttestation(origin, issuedAt, "0-100", actor.TrustScore, priv)
			if serr == nil {
				doc["trust"] = map[string]any{
					"score":     actor.TrustScore,
					"scale":     "0-100",
					"issuer":    origin,
					"issuedAt":  issuedAt,
					"signature": sig,
				}
			} else {
				slog.Warn("ap: attestation sign failed", "actor_id", actor.ID, "err", serr)
			}
		}
	}

	w.Header().Set("Content-Type", "application/activity+json")
	w.Header().Set("Cache-Control", "max-age=60")
	api.JSON(w, http.StatusOK, doc)
}

// Outbox handles GET /users/{handle}/outbox. Returns a paginated
// OrderedCollection whose items are Create activities wrapping Note
// objects — the ActivityPub representation of the author's posts.
func (h *ActivityPubHandler) Outbox(w http.ResponseWriter, r *http.Request) {
	handle := strings.ToLower(r.PathValue("handle"))
	actor, err := h.store.ResolveHandle(r.Context(), handle)
	if err != nil || actor == nil {
		api.Error(w, http.StatusNotFound, "no such actor")
		return
	}

	origin := h.originURL()
	actorURL := fmt.Sprintf("%s/users/%s", origin, actor.Handle)
	outboxURL := actorURL + "/outbox"

	// Simplest correct shape: always return the first page of items
	// inline. Remote implementations typically just look at `first` or
	// expand the `orderedItems` slice here. We select only the
	// fields needed to render the Note — title, body, created_at.
	rows, err := h.pool.Query(r.Context(), `
        SELECT id, title, body, created_at
        FROM posts
        WHERE author_id = $1 AND deleted_at IS NULL
        ORDER BY created_at DESC
        LIMIT 40`, actor.ID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list posts")
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, title, body string
		var createdAt time.Time
		if err := rows.Scan(&id, &title, &body, &createdAt); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan post")
			return
		}
		postURL := fmt.Sprintf("%s/post/%s", origin, id)
		published := createdAt.UTC().Format("2006-01-02T15:04:05Z")
		note := map[string]any{
			"id":           postURL,
			"type":         "Note",
			"attributedTo": actorURL,
			"content":      fmt.Sprintf("<p><strong>%s</strong></p><p>%s</p>", escapeHTML(title), escapeHTML(body)),
			"published":    published,
			"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
			"url":          postURL,
		}
		items = append(items, map[string]any{
			"id":        postURL + "#activity",
			"type":      "Create",
			"actor":     actorURL,
			"published": published,
			"to":        []string{"https://www.w3.org/ns/activitystreams#Public"},
			"object":    note,
		})
	}

	w.Header().Set("Content-Type", "application/activity+json")
	w.Header().Set("Cache-Control", "max-age=30")
	api.JSON(w, http.StatusOK, map[string]any{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           outboxURL,
		"type":         "OrderedCollection",
		"totalItems":   len(items),
		"orderedItems": items,
	})
}

// escapeHTML is a tiny escaper for the handful of chars that actually
// break rendering in remote clients. We don't round-trip arbitrary
// markdown here — Loomfeed posts are authored as markdown but the
// outbox is plain-text with structure hints.
func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}
