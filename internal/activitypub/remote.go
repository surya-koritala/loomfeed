package activitypub

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/RoamXAI/loomfeed/internal/safehttp"
)

// actorClient is SSRF-hardened. FetchActor runs on attacker-controlled URIs
// (the inbox endpoint resolves a signature keyId before the signature is
// verified), so it must refuse private/internal/metadata targets at dial
// time and across redirects.
var actorClient = safehttp.NewClient(safehttp.Options{Timeout: 10 * time.Second, MaxRedirects: 3})

// RemoteActor is the minimal shape we parse from a fetched actor doc.
// Anything else the remote server sends is ignored.
type RemoteActor struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	PreferredUsername string `json:"preferredUsername"`
	Inbox             string `json:"inbox"`
	Endpoints         struct {
		SharedInbox string `json:"sharedInbox"`
	} `json:"endpoints"`
	PublicKey struct {
		ID           string `json:"id"`
		Owner        string `json:"owner"`
		PublicKeyPem string `json:"publicKeyPem"`
	} `json:"publicKey"`
	// Loomfeed-specific trust attestation block. Other instances
	// ignore the field. See docs/FEDIVERSE_TRUST.md.
	Trust *TrustAttestation `json:"trust,omitempty"`
}

// TrustAttestation is the signed trust-score claim a home instance
// may emit on its actors. All four top fields plus the signature are
// required to verify.
type TrustAttestation struct {
	Score     float64 `json:"score"`
	Scale     string  `json:"scale"`
	Issuer    string  `json:"issuer"`
	IssuedAt  string  `json:"issuedAt"`
	Signature string  `json:"signature"`
}

// actorCache is a tiny in-memory cache keyed by actor URI. We store a
// short TTL (10 min) — long enough to avoid re-fetching for every
// incoming Create, short enough that a rotated key eventually takes
// effect. Concurrent-safe via RWMutex.
type cacheEntry struct {
	actor   *RemoteActor
	fetched time.Time
}

var (
	actorCacheMu sync.RWMutex
	actorCache   = map[string]cacheEntry{}
)

const actorCacheTTL = 10 * time.Minute

// FetchActor GETs the actor document. Sets the ActivityPub Accept
// header so servers give us JSON-LD instead of HTML.
func FetchActor(actorURI string) (*RemoteActor, error) {
	actorCacheMu.RLock()
	if entry, ok := actorCache[actorURI]; ok && time.Since(entry.fetched) < actorCacheTTL {
		actorCacheMu.RUnlock()
		return entry.actor, nil
	}
	actorCacheMu.RUnlock()

	if err := safehttp.ValidateURL(actorURI); err != nil {
		return nil, fmt.Errorf("fetch actor: %w", err)
	}
	req, err := http.NewRequest(http.MethodGet, actorURI, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/activity+json, application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"")
	req.Header.Set("User-Agent", "loomfeed/1.0 (+https://www.loomfeed.com)")

	resp, err := actorClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch actor: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch actor: http %d", resp.StatusCode)
	}

	var actor RemoteActor
	if err := json.NewDecoder(resp.Body).Decode(&actor); err != nil {
		return nil, fmt.Errorf("decode actor: %w", err)
	}
	if actor.ID == "" {
		return nil, fmt.Errorf("actor doc missing id")
	}

	actorCacheMu.Lock()
	actorCache[actorURI] = cacheEntry{actor: &actor, fetched: time.Now()}
	actorCacheMu.Unlock()

	return &actor, nil
}

// ResolveKey is the callback passed to VerifyRequest. Extracts the
// actor URI from the keyId (strip the #fragment) and fetches the
// corresponding actor document to get its public key.
func ResolveKey(keyID string) (*rsa.PublicKey, error) {
	// keyId is typically "<actor-uri>#main-key". Strip the fragment.
	actorURI := keyID
	if before, _, ok := strings.Cut(keyID, "#"); ok {
		actorURI = before
	}
	actor, err := FetchActor(actorURI)
	if err != nil {
		return nil, err
	}
	if actor.PublicKey.PublicKeyPem == "" {
		return nil, fmt.Errorf("actor has no publicKey")
	}
	return parsePublicKey(actor.PublicKey.PublicKeyPem)
}
