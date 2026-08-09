// Package crypto wraps small primitives we need for BYOK secret handling.
// For anything fancier we should reach for a vetted library.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sync"
)

// BYOKVault seals/opens BYOK API keys with AES-256-GCM using a key loaded
// from the BYOK_KEK env var (32 random bytes, base64-encoded).
type BYOKVault struct {
	gcm cipher.AEAD
}

var (
	defaultVault   *BYOKVault
	defaultVaultMu sync.Mutex
)

// NewBYOKVault builds a vault directly from a 32-byte key. Mostly used by
// tests; production code should use DefaultBYOKVault which loads from env.
func NewBYOKVault(key []byte) (*BYOKVault, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &BYOKVault{gcm: gcm}, nil
}

// DefaultBYOKVault returns a process-wide vault, initialized lazily from the
// BYOK_KEK env var. Returns an error if the env var is missing or malformed.
func DefaultBYOKVault() (*BYOKVault, error) {
	defaultVaultMu.Lock()
	defer defaultVaultMu.Unlock()
	if defaultVault != nil {
		return defaultVault, nil
	}
	raw := os.Getenv("BYOK_KEK")
	if raw == "" {
		return nil, errors.New("BYOK_KEK env var not set")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("BYOK_KEK is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("BYOK_KEK must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	defaultVault = &BYOKVault{gcm: gcm}
	return defaultVault, nil
}

// Seal encrypts plaintext and returns a base64-encoded blob that packs
// nonce || ciphertext || tag together so it stores in a single column.
func (v *BYOKVault) Seal(plaintext string) (string, error) {
	if plaintext == "" {
		return "", errors.New("plaintext is empty")
	}
	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := v.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open decrypts a blob produced by Seal.
func (v *BYOKVault) Open(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("decode sealed blob: %w", err)
	}
	ns := v.gcm.NonceSize()
	if len(raw) < ns+v.gcm.Overhead() {
		return "", errors.New("sealed blob too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := v.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(pt), nil
}
