package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Environment string
	LogLevel    string

	API         APIConfig
	Gateway     GatewayConfig
	DB          DatabaseConfig
	Redis       RedisConfig
	JWT         JWTConfig
	OAuth       OAuthConfig
	Email       EmailConfig
	TenorAPIKey    string
	GoogleClientID string
	LLM             LLMConfig
	Uploads         UploadsConfig
	Push            PushConfig
	IndexNow        IndexNowConfig
	CuratedShorts   CuratedShortsConfig
	Sports          SportsConfig
	Auth            AuthConfig
	// LoomDeployment names the Azure OpenAI deployment used by the
	// @loom AI summon path specifically. Defaults to gpt-5.4-mini
	// (cheap, fast — right tier for short summaries). Falls back to
	// LLM.DeploymentName if unset, so a dev box configured for just
	// one deployment doesn't need a second env var to enable Loom.
	LoomDeployment string
}

// AuthConfig gates the cookie-auth migration. Phase 1.2 dual-issues
// httpOnly cookies alongside the existing JSON token response. Setting
// AUTH_COOKIE_ISSUANCE=false in an environment turns the cookie path
// off without touching code — fast rollback path during the rollout.
type AuthConfig struct {
	CookieIssuance bool
}

// CuratedShortsConfig gates the external-shorts ingest pipeline. An
// empty YouTubeAPIKey makes the whole system a silent no-op — fetches
// return nothing, scoring is skipped, the /shorts feed falls back to
// whatever's already in the DB. Same gating pattern as PushConfig
// and IndexNowConfig.
type CuratedShortsConfig struct {
	YouTubeAPIKey string
}

// SportsConfig holds the football-data.org API key for the World Cup
// schedule/score poller. Unlike CuratedShortsConfig, an empty key does
// NOT disable the feature — the poller runs keyless against the public
// tier (lower upstream rate limits) and adapts its cadence accordingly.
type SportsConfig struct {
	FootballDataKey string
}

// IndexNowConfig — host + shared key so we can ping IndexNow whenever
// content changes. Host must match the canonical public hostname
// (no scheme). Key must also be served at KeyLocation as a plain-
// text file so IndexNow can verify ownership. Leaving any field
// empty makes the pinger a silent no-op.
type IndexNowConfig struct {
	Host        string
	Key         string
	KeyLocation string
}

// PushConfig holds VAPID credentials for Web Push. When either key is
// empty, push notifications are disabled — /api/v1/push/subscribe
// still accepts subscriptions, but the send pipeline short-circuits.
// Generate keys once with: webpush.GenerateVAPIDKeys() and set the
// resulting base64url strings as env vars.
type PushConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string // mailto:contact@loomfeed.com
}

// UploadsConfig gates image uploads and their safety moderation. Both
// flags default to false so a misconfigured environment fails closed —
// no public image endpoint accepts content until an operator explicitly
// enables it AND wires Content Safety credentials.
type UploadsConfig struct {
	Enabled       bool
	ContentSafety ContentSafetyConfig
}

type ContentSafetyConfig struct {
	Enabled  bool
	Endpoint string
	Key      string
}

type LLMConfig struct {
	Endpoint       string
	APIKey         string
	DeploymentName string
	// EmbedDeployment is the Azure OpenAI deployment name for the
	// embeddings model (separate from DeploymentName which targets a
	// chat-completions model). Used by Loom v2's thread-connector
	// pipeline. Defaults to "text-embedding-3-large" (already
	// deployed on roamx-resource); operators with a different
	// embeddings deployment override via LLM_EMBED_DEPLOYMENT.
	EmbedDeployment string
}

type EmailConfig struct {
	ACSConnectionString string
	ACSEmailDomain      string
	SiteURL             string
}

type APIConfig struct {
	Host           string
	Port           string
	AllowedOrigins []string
}

type GatewayConfig struct {
	Port string
}

type DatabaseConfig struct {
	URL string
}

type RedisConfig struct {
	URL string
}

type JWTConfig struct {
	Secret string
	Expiry time.Duration
}

type OAuthConfig struct {
	GitHubClientID     string
	GitHubClientSecret string
	GitHubRedirectURI  string
}

func Load() (*Config, error) {
	jwtExpiry, err := time.ParseDuration(getEnv("JWT_EXPIRY", "24h"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_EXPIRY: %w", err)
	}

	return &Config{
		Environment: getEnv("ENVIRONMENT", "development"),
		LogLevel:    getEnv("LOG_LEVEL", "debug"),
		API: APIConfig{
			Host:           getEnv("API_HOST", "0.0.0.0"),
			Port:           getEnv("API_PORT", "8080"),
			AllowedOrigins: parseAllowedOrigins(getEnv("ALLOWED_ORIGINS", "https://www.loomfeed.com,https://loomfeed.com")),
		},
		Gateway: GatewayConfig{
			Port: getEnv("GATEWAY_PORT", "8081"),
		},
		DB: DatabaseConfig{
			URL: getEnv("DATABASE_URL", "postgres://loomfeed:loomfeed@localhost:5432/loomfeed?sslmode=disable"),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", "redis://localhost:6379"),
		},
		JWT: JWTConfig{
			Secret: getEnv("JWT_SECRET", ""),
			Expiry: jwtExpiry,
		},
		OAuth: OAuthConfig{
			GitHubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
			GitHubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
			GitHubRedirectURI:  getEnv("GITHUB_REDIRECT_URI", "http://localhost:8080/api/v1/auth/github/callback"),
		},
		Email: EmailConfig{
			ACSConnectionString: getEnv("ACS_CONNECTION_STRING", ""),
			ACSEmailDomain:      getEnv("ACS_EMAIL_DOMAIN", ""),
			SiteURL:             getEnv("SITE_URL", "https://www.loomfeed.com"),
		},
		TenorAPIKey:    getEnv("TENOR_API_KEY", ""),
		GoogleClientID: getEnv("GOOGLE_CLIENT_ID", ""),
		LLM: LLMConfig{
			Endpoint:        getEnv("LLM_ENDPOINT", ""),
			APIKey:          getEnv("LLM_API_KEY", ""),
			DeploymentName:  getEnv("LLM_DEPLOYMENT", "gpt-5.4-nano"),
			EmbedDeployment: getEnv("LLM_EMBED_DEPLOYMENT", "text-embedding-3-large"),
		},
		Push: PushConfig{
			PublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
			PrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
			Subject:    getEnv("VAPID_SUBJECT", "mailto:contact@loomfeed.com"),
		},
		IndexNow: IndexNowConfig{
			Host:        getEnv("INDEXNOW_HOST", "www.loomfeed.com"),
			Key:         getEnv("INDEXNOW_KEY", ""),
			KeyLocation: getEnv("INDEXNOW_KEY_LOCATION", ""),
		},
		CuratedShorts: CuratedShortsConfig{
			YouTubeAPIKey: getEnv("YOUTUBE_API_KEY", ""),
		},
		Sports: SportsConfig{
			FootballDataKey: getEnv("SPORTS_FOOTBALL_DATA_KEY", ""),
		},
		LoomDeployment: getEnv("LOOM_DEPLOYMENT", "gpt-5.4-mini"),
		Auth: AuthConfig{
			// Default true: phase 1.2 ships cookie issuance enabled.
			// Set to "false" to roll back without redeploy.
			CookieIssuance: getEnv("AUTH_COOKIE_ISSUANCE", "true") != "false",
		},
		Uploads: UploadsConfig{
			Enabled: getEnv("UPLOADS_ENABLED", "") == "true",
			ContentSafety: ContentSafetyConfig{
				Enabled:  getEnv("CONTENT_SAFETY_ENABLED", "") == "true",
				Endpoint: strings.TrimSuffix(getEnv("CONTENT_SAFETY_ENDPOINT", ""), "/"),
				Key:      getEnv("CONTENT_SAFETY_KEY", ""),
			},
		},
	}, nil
}

func (c *Config) Validate() error {
	if c.JWT.Secret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if c.DB.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseAllowedOrigins splits a comma-separated list of origins.
// Empty fallback: cross-origin requests are blocked unless ALLOWED_ORIGINS is set.
// To allow all origins in development, set ALLOWED_ORIGINS=* explicitly.
func parseAllowedOrigins(raw string) []string {
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}
