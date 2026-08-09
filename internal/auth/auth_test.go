package auth

import (
	"testing"
)

func TestGenerateVerificationToken_Length(t *testing.T) {
	token, err := GenerateVerificationToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 32 bytes = 64 hex characters
	if len(token) != 64 {
		t.Errorf("expected token length 64, got %d", len(token))
	}
}

func TestGenerateVerificationToken_Uniqueness(t *testing.T) {
	token1, err := GenerateVerificationToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token2, err := GenerateVerificationToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token1 == token2 {
		t.Error("expected unique tokens, got identical values")
	}
}

func TestGenerateVerificationToken_HexOnly(t *testing.T) {
	token, err := GenerateVerificationToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("expected hex character, got %c", c)
		}
	}
}
