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
)

// BYOKVault seals/opens BYOK API keys with AES-256-GCM using a key loaded
// from the BYOK_KEK env var (32 random bytes, base64-encoded).
type BYOKVault struct {
	gcm cipher.AEAD
}

// NewBYOKVault builds a vault directly from a 32-byte key. Callers using the
// operator-facing configuration should prefer NewBYOKVaultFromBase64 after
// validating the feature flag and key together.
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

// NewBYOKVaultFromBase64 constructs a vault from the operator-facing
// BYOK_KEK representation without reading process-global environment state.
func NewBYOKVaultFromBase64(encoded string) (*BYOKVault, error) {
	if encoded == "" {
		return nil, errors.New("BYOK_KEK env var not set")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("BYOK_KEK is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("BYOK_KEK must decode to 32 bytes, got %d", len(key))
	}
	return NewBYOKVault(key)
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
