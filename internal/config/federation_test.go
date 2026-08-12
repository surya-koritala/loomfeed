package config

import "testing"

func TestFederationFeatureFlag(t *testing.T) {
	t.Setenv("FEDERATION_ENABLED", "true")
	cfg, err := Load()
	if err != nil || !cfg.Federation.Enabled {
		t.Fatalf("FEDERATION_ENABLED=true: enabled=%v err=%v", cfg.Federation.Enabled, err)
	}

	t.Setenv("FEDERATION_ENABLED", "false")
	cfg, err = Load()
	if err != nil || cfg.Federation.Enabled {
		t.Fatalf("FEDERATION_ENABLED=false: enabled=%v err=%v", cfg.Federation.Enabled, err)
	}

	t.Setenv("FEDERATION_ENABLED", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("invalid FEDERATION_ENABLED must fail configuration loading")
	}
}
