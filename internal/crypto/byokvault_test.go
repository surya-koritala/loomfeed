package crypto_test

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"

	cryptopkg "github.com/RoamXAI/loomfeed/internal/crypto"
)

func newTestVault(t *testing.T) *cryptopkg.BYOKVault {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	t.Setenv("BYOK_KEK", base64.StdEncoding.EncodeToString(key))
	// Force re-init each test — the package caches a process-wide vault by
	// default, but we want a fresh one bound to this env var.
	os.Unsetenv("BYOK_KEK_CACHE_BUSTER") // no-op, keeps go vet happy about t.Setenv sequencing
	v, err := cryptopkg.NewBYOKVault(key)
	if err != nil {
		t.Fatalf("NewBYOKVault: %v", err)
	}
	return v
}

func TestBYOKVault_RoundTrip(t *testing.T) {
	v := newTestVault(t)
	plain := "sk-this-is-a-fake-api-key-12345"
	sealed, err := v.Seal(plain)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if sealed == plain {
		t.Fatal("sealed output equals plaintext")
	}
	got, err := v.Open(sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != plain {
		t.Errorf("round trip mismatch: want %q, got %q", plain, got)
	}
}

func TestBYOKVault_TamperDetection(t *testing.T) {
	v := newTestVault(t)
	sealed, err := v.Seal("super-secret")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(sealed)
	raw[len(raw)-1] ^= 0xff // flip a tag byte
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := v.Open(tampered); err == nil {
		t.Fatal("expected tampered blob to fail, got nil error")
	}
}

func TestBYOKVault_Seal_EmptyRejected(t *testing.T) {
	v := newTestVault(t)
	if _, err := v.Seal(""); err == nil {
		t.Fatal("expected empty plaintext to be rejected")
	}
}
