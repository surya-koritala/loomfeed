// Package activitypub provides Loomfeed's in-process bridge to the fediverse:
// actor discovery, signed inbox/outbox traffic, remote replies and Likes, and
// durable inbound/outbound follow relationships.
package activitypub

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store owns persistence of actor handles and keypairs. Backed by
// participants columns added in migration 000049.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type Actor struct {
	ID          string
	DisplayName string
	Type        string // human | agent
	Bio         string
	AvatarURL   string
	Handle      string
	PublicKey   string
	TrustScore  float64
}

// GetByHandle returns the actor with ap_handle == handle. Handles are
// stored lowercased.
func (s *Store) GetByHandle(ctx context.Context, handle string) (*Actor, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return nil, fmt.Errorf("handle required")
	}
	var a Actor
	var bio, avatar, pub *string
	err := s.pool.QueryRow(ctx, `
        SELECT id, display_name, type::text,
               COALESCE(bio, ''), COALESCE(avatar_url, ''),
               COALESCE(ap_handle, ''), ap_public_key,
               COALESCE(trust_score, 0)
        FROM participants
        WHERE ap_handle = $1 AND type <> 'remote'`, handle).
		Scan(&a.ID, &a.DisplayName, &a.Type, &bio, &avatar, &a.Handle, &pub, &a.TrustScore)
	if err != nil {
		return nil, err
	}
	if bio != nil {
		a.Bio = *bio
	}
	if avatar != nil {
		a.AvatarURL = *avatar
	}
	if pub != nil {
		a.PublicKey = *pub
	}
	return &a, nil
}

// EnsureHandleAndKey returns an actor after lazily materializing any
// missing handle or keypair. Callers (the ActivityPub handlers) use
// this so participants don't need a proactive backfill pass.
func (s *Store) EnsureHandleAndKey(ctx context.Context, participantID string) (*Actor, error) {
	var (
		id, displayName, pType, bio, avatar string
		handle                              *string
		pubKey, privKey                     *string
	)
	var trust float64
	err := s.pool.QueryRow(ctx, `
        SELECT id, display_name, type::text,
               COALESCE(bio, ''), COALESCE(avatar_url, ''),
               ap_handle, ap_public_key, ap_private_key,
               COALESCE(trust_score, 0)
        FROM participants WHERE id = $1 AND type <> 'remote'`, participantID).
		Scan(&id, &displayName, &pType, &bio, &avatar, &handle, &pubKey, &privKey, &trust)
	if err != nil {
		return nil, err
	}

	// Materialize handle if missing.
	if handle == nil || *handle == "" {
		h, err := s.pickHandle(ctx, displayName, id)
		if err != nil {
			return nil, err
		}
		_, err = s.pool.Exec(ctx, `UPDATE participants SET ap_handle = $1 WHERE id = $2`, h, id)
		if err != nil {
			return nil, fmt.Errorf("assign handle: %w", err)
		}
		handle = &h
	}

	// Materialize keypair if missing.
	if pubKey == nil || *pubKey == "" || privKey == nil || *privKey == "" {
		pub, priv, err := generateKeypair()
		if err != nil {
			return nil, err
		}
		_, err = s.pool.Exec(ctx,
			`UPDATE participants SET ap_public_key = $1, ap_private_key = $2 WHERE id = $3`,
			pub, priv, id)
		if err != nil {
			return nil, fmt.Errorf("store keypair: %w", err)
		}
		pubKey = &pub
	}

	a := &Actor{
		ID:          id,
		DisplayName: displayName,
		Type:        pType,
		Bio:         bio,
		AvatarURL:   avatar,
		Handle:      *handle,
		PublicKey:   *pubKey,
		TrustScore:  trust,
	}
	return a, nil
}

// ResolveHandle finds an actor by either ap_handle or display_name
// (slugified). Returns nil, nil if no match — caller treats that as 404.
// Never materializes a keypair for a caller that just wants to look up;
// use EnsureHandleAndKey for the actor document path.
func (s *Store) ResolveHandle(ctx context.Context, handle string) (*Actor, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return nil, nil
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM participants WHERE ap_handle = $1 AND type <> 'remote'`, handle).Scan(&id)
	if err != nil {
		// Not found by ap_handle — try slugified display_name match.
		err = s.pool.QueryRow(ctx, `
            SELECT id FROM participants
            WHERE lower(regexp_replace(display_name, '[^a-zA-Z0-9]', '', 'g')) = $1
			  AND type <> 'remote'
            LIMIT 1`, handle).Scan(&id)
		if err != nil {
			return nil, nil
		}
	}
	return s.EnsureHandleAndKey(ctx, id)
}

var slugSafe = regexp.MustCompile(`[^a-z0-9]+`)

// PrivateKeyPEM returns the PEM-encoded RSA private key for a
// participant, lazily materializing one if missing. Separate from the
// actor lookup so outbound delivery paths don't have to hydrate the
// full Actor struct.
func (s *Store) PrivateKeyPEM(ctx context.Context, participantID string) (string, error) {
	// Ensure keypair exists.
	if _, err := s.EnsureHandleAndKey(ctx, participantID); err != nil {
		return "", err
	}
	var priv string
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(ap_private_key, '') FROM participants WHERE id = $1`,
		participantID).Scan(&priv)
	if err != nil {
		return "", err
	}
	return priv, nil
}

// pickHandle slugifies the display name and appends a short disambiguator
// if needed (first 4 chars of the id).
func (s *Store) pickHandle(ctx context.Context, displayName, id string) (string, error) {
	base := strings.Trim(slugSafe.ReplaceAllString(strings.ToLower(displayName), ""), "-")
	if base == "" {
		base = "user"
	}
	if len(base) > 32 {
		base = base[:32]
	}
	for i := range 6 {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s%d", base, i)
		}
		var exists bool
		err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM participants WHERE ap_handle = $1)`, candidate).
			Scan(&exists)
		if err != nil {
			return "", fmt.Errorf("handle existence check: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}
	// Final fallback — guaranteed unique via id prefix.
	short := strings.ReplaceAll(id, "-", "")
	if len(short) > 8 {
		short = short[:8]
	}
	return base + short, nil
}

// generateKeypair returns (publicPEM, privatePEM, error). RSA-2048 is
// what Mastodon et al. use; smaller keys would be rejected outright.
func generateKeypair() (string, string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generate rsa: %w", err)
	}
	privDER := x509.MarshalPKCS1PrivateKey(key)
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("marshal public: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return string(pubPEM), string(privPEM), nil
}
