package activitypub

import (
	"bytes"
	"crypto/rsa"
	"net/http"
	"testing"
)

func TestSignAndVerifyRoundtrip(t *testing.T) {
	pubPEM, privPEM, err := generateKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// Build a realistic outbound POST.
	body := []byte(`{"type":"Create","object":{"type":"Note"}}`)
	req, err := http.NewRequest(http.MethodPost,
		"https://example.com/users/alice/inbox", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/activity+json")

	keyID := "https://sender.example/users/bob#main-key"
	if err := SignRequest(req, keyID, privPEM, body); err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Signature header should be populated.
	if req.Header.Get("Signature") == "" {
		t.Fatal("no Signature header after signing")
	}
	if req.Header.Get("Digest") == "" {
		t.Fatal("no Digest header after signing a POST")
	}

	// Verify with a resolver that hands back the matching public key.
	resolver := func(gotKeyID string) (*rsa.PublicKey, error) {
		if gotKeyID != keyID {
			t.Errorf("unexpected keyID: %s", gotKeyID)
		}
		return parsePublicKey(pubPEM)
	}
	resolvedKeyID, err := VerifyRequest(req, body, resolver)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if resolvedKeyID != keyID {
		t.Errorf("resolved keyID mismatch: got %s", resolvedKeyID)
	}
}

func TestVerifyFailsOnTamperedBody(t *testing.T) {
	pubPEM, privPEM, err := generateKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	body := []byte(`{"type":"Follow"}`)
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/users/alice/inbox", bytes.NewReader(body))
	if err := SignRequest(req, "keyid", privPEM, body); err != nil {
		t.Fatalf("sign: %v", err)
	}
	tampered := []byte(`{"type":"Create"}`) // different body = different digest
	_, err = VerifyRequest(req, tampered, func(string) (*rsa.PublicKey, error) {
		return parsePublicKey(pubPEM)
	})
	if err == nil {
		t.Fatal("expected verify to fail on tampered body")
	}
}

func TestAttestationRoundtrip(t *testing.T) {
	pubPEM, privPEM, err := generateKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	sig, err := SignAttestation("https://loomfeed.com", "2026-04-20T19:00:00Z", "0-100", 87.3, privPEM)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := VerifyAttestation("https://loomfeed.com", "2026-04-20T19:00:00Z", "0-100", 87.3, sig, pubPEM); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Tampered score should fail.
	if err := VerifyAttestation("https://loomfeed.com", "2026-04-20T19:00:00Z", "0-100", 87.4, sig, pubPEM); err == nil {
		t.Fatal("expected verify to fail on tampered score")
	}
}

func TestVerifyFailsWithWrongKey(t *testing.T) {
	_, privPEM, _ := generateKeypair()
	otherPubPEM, _, _ := generateKeypair()

	body := []byte(`{"type":"Follow"}`)
	req, _ := http.NewRequest(http.MethodPost, "https://example.com/users/alice/inbox", bytes.NewReader(body))
	if err := SignRequest(req, "keyid", privPEM, body); err != nil {
		t.Fatalf("sign: %v", err)
	}
	_, err := VerifyRequest(req, body, func(string) (*rsa.PublicKey, error) {
		return parsePublicKey(otherPubPEM)
	})
	if err == nil {
		t.Fatal("expected verify to fail with wrong key")
	}
}
