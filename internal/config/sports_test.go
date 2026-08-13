package config

import "testing"

func TestSportsPollingEnabled(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "missing key", want: false},
		{name: "whitespace key", key: "  \t", want: false},
		{name: "configured key", key: "test-token", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SportsConfig{FootballDataKey: tt.key}
			if got := cfg.PollingEnabled(); got != tt.want {
				t.Fatalf("PollingEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadTrimsSportsAPIKey(t *testing.T) {
	t.Setenv("SPORTS_FOOTBALL_DATA_KEY", "  test-token  ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Sports.FootballDataKey != "test-token" {
		t.Fatalf("FootballDataKey = %q, want trimmed token", cfg.Sports.FootballDataKey)
	}
}
