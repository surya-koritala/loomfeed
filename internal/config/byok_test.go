package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func validBYOKKEK() string {
	return base64.StdEncoding.EncodeToString(make([]byte, 32))
}

func validBaseConfig() *Config {
	return &Config{
		DB:  DatabaseConfig{URL: "postgres://example"},
		JWT: JWTConfig{Secret: "test-secret"},
	}
}

func TestLoadBYOKFeatureFlag(t *testing.T) {
	t.Run("valid key preserves legacy implicit enablement", func(t *testing.T) {
		t.Setenv("BYOK_ENABLED", "")
		t.Setenv("BYOK_KEK", validBYOKKEK())
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if !cfg.BYOK.Enabled || cfg.BYOK.ExplicitlyEnabled {
			t.Fatalf("BYOK config = %#v, want implicitly enabled", cfg.BYOK)
		}
	})

	t.Run("explicit false disables a configured key", func(t *testing.T) {
		t.Setenv("BYOK_ENABLED", "false")
		t.Setenv("BYOK_KEK", validBYOKKEK())
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.BYOK.Enabled || cfg.BYOK.ExplicitlyEnabled {
			t.Fatalf("BYOK config = %#v, want disabled", cfg.BYOK)
		}
	})

	t.Run("invalid flag is rejected", func(t *testing.T) {
		t.Setenv("BYOK_ENABLED", "sometimes")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "BYOK_ENABLED") {
			t.Fatalf("Load() error = %v, want BYOK_ENABLED error", err)
		}
	})
}

func TestValidateBYOKExplicitEnablement(t *testing.T) {
	tests := []struct {
		name string
		kek  string
	}{
		{name: "missing key"},
		{name: "malformed base64", kek: "not-base64"},
		{name: "wrong key length", kek: base64.StdEncoding.EncodeToString([]byte("short"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.BYOK = BYOKConfig{
				Enabled:           true,
				ExplicitlyEnabled: true,
				KEK:               tt.kek,
			}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), "BYOK_KEK") {
				t.Fatalf("Validate() error = %v, want BYOK_KEK error", err)
			}
		})
	}

	cfg := validBaseConfig()
	cfg.BYOK = BYOKConfig{
		Enabled:           true,
		ExplicitlyEnabled: true,
		KEK:               validBYOKKEK(),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with valid BYOK key error = %v", err)
	}
}
