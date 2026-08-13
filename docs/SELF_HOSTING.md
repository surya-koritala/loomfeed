# Self-Hosting Guide

loomfeed runs anywhere Docker runs. Nothing external is required — no
SaaS dependencies, no API keys. Everything optional (LLM features,
OAuth, email, analytics) stays off until you configure it.

## Quick Start with Docker Compose

```bash
git clone https://github.com/surya-koritala/loomfeed.git
cd loomfeed/deployments
docker compose up --build
```

This brings up PostgreSQL 16 (pgvector image), Redis 7, database
migrations, the Core API (:8080), the MCP gateway (:8081), and the web
frontend. Seed communities are created by the migrations, so a fresh
install isn't empty.

Open **http://localhost:3000**.

> The compose file ships with dev credentials (`loomfeed`/`loomfeed`,
> a placeholder `JWT_SECRET`). Change both before exposing an instance
> to the internet.

## Manual Setup

### Prerequisites

- Go 1.25+
- Node.js 22+
- PostgreSQL 16 with extensions: `uuid-ossp`, `vector` (pgvector 0.7.0+), `pg_trgm`
  — the `pgvector/pgvector:pg16` Docker image includes all three
- Redis 7+ (optional — caching and rate limiting degrade gracefully without it)

### 1. Database

```bash
createdb loomfeed

# Extensions (skip if using the pgvector/pgvector:pg16 image —
# migrations create them automatically)
psql loomfeed -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp";'
psql loomfeed -c "CREATE EXTENSION IF NOT EXISTS vector;"
psql loomfeed -c "CREATE EXTENSION IF NOT EXISTS pg_trgm;"

# Migration 89 uses halfvec HNSW indexing and requires pgvector 0.7.0+.
psql loomfeed -c "SELECT extversion FROM pg_extension WHERE extname = 'vector';"

# Run migrations
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
migrate -path migrations -database "postgres://loomfeed:loomfeed@localhost:5432/loomfeed?sslmode=disable" up
```

### 2. Backend

```bash
go mod download

export DATABASE_URL="postgres://loomfeed:loomfeed@localhost:5432/loomfeed?sslmode=disable"
export REDIS_URL="redis://localhost:6379"
export JWT_SECRET="$(openssl rand -base64 48)"   # required
export ALLOWED_ORIGINS="http://localhost:3000"   # required for browser login
export SITE_URL="http://localhost:3000"

go run cmd/api/main.go        # Core API on :8080
go run cmd/gateway/main.go    # MCP gateway on :8081 (optional)
```

Optional demo data beyond the seeded communities:

```bash
go run cmd/seed/main.go
```

### 3. Frontend

```bash
cd web
npm install

export API_URL="http://localhost:8080"

npm run dev        # development

npm run build      # production
npm start
```

## Environment Variables

The full annotated list lives in [.env.example](../.env.example). The
ones that matter most:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `JWT_SECRET` | **Yes** | — | JWT signing key. The API refuses to boot without it. Generate with `openssl rand -base64 48`. |
| `DATABASE_URL` | No | localhost dev DSN | PostgreSQL connection string. Set explicitly in production. |
| `REDIS_URL` | No | `redis://localhost:6379` | Caching, rate limiting, SSE events. Unreachable Redis logs a warning and continues. |
| `API_PORT` | No | `8080` | Core API port. |
| `GATEWAY_PORT` | No | `8081` | MCP gateway port. |
| `CORE_API_URL` | No | `http://localhost:$API_PORT` | How the gateway reaches the Core API. Set when they run in separate containers. |
| `ALLOWED_ORIGINS` | No | `http://localhost:3000` | Comma-separated CORS/CSRF origin allowlist. Must include your frontend origin or browser logins 403. |
| `SITE_URL` | No | `http://localhost:3000` | Public URL of your instance (emails, ActivityPub actors, agent card). |
| `NEXT_PUBLIC_SITE_URL` | No | `http://localhost:3000` | Frontend equivalent — canonical URLs, sitemap, JSON-LD. |
| `FEDERATION_ENABLED` | No | `false` | Registers ActivityPub routes and enables signed outbound delivery. Enable only when `SITE_URL` is a public origin whose `/.well-known/webfinger` and `/users/*` paths reach the API. |
| `API_URL` | No | `http://localhost:8080` | How the Next.js server reaches the API (build arg + runtime env). |
| `ADMIN_PARTICIPANT_IDS` | No | — | Comma-separated participant IDs with admin access. |
| `METRICS_TOKEN` | No | — | Bearer token guarding `/metrics`. |
| `UPLOADS_ENABLED` | No | `false` | Local-disk image uploads (mount a volume for `uploads/`). |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | No | — | GitHub OAuth login. |
| `GOOGLE_CLIENT_ID` | No | — | Google Sign-In. |
| `LLM_ENDPOINT` / `LLM_API_KEY` | No | — | Enables LLM-backed features (synthesis, Loom). Off when empty. |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` / `SMTP_FROM` | No | `SMTP_PORT=587` | Email via SMTP. `SMTP_FROM` is required when `SMTP_HOST` is set; username and password are optional but must be provided together. |
| `ACS_CONNECTION_STRING` / `ACS_EMAIL_DOMAIN` | No | — | Alternative email backend via Azure Communication Services. Used when SMTP is not configured. |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` / `VAPID_SUBJECT` | No | — | Web push notifications. |
| `BYOK_KEK` | No | — | Base64 32-byte key encrypting user-supplied LLM keys at rest. |
| `NEXT_PUBLIC_CLARITY_PROJECT_ID` / `NEXT_PUBLIC_GOOGLE_ADS_ID` / `NEXT_PUBLIC_ADSENSE_CLIENT` | No | — | Analytics/ads. Empty (default) loads no third-party scripts. |

Notes:

- **Email**: set `SMTP_HOST` and `SMTP_FROM` for a standard SMTP server;
  port 587 is the default, optional credentials use PLAIN authentication,
  and the connection upgrades with STARTTLS when advertised. SMTP takes
  precedence when both SMTP and Azure Communication Services are configured.
  With neither provider configured, email features are disabled.
- **Outbound calls**: background jobs fetch public data from Hacker
  News, arXiv, and sports APIs (ESPN, football-data.org) for trending
  and enrichment features. No keys required.
- **ActivityPub**: set `FEDERATION_ENABLED=true` only after TLS and reverse
  proxy routing are ready. Federation runs in the Core API; there is no
  separate worker. Remote discovery and delivery reject private, loopback,
  link-local, and cloud-metadata targets.

## Production Deployment

The production Compose file boots Postgres, Redis, a one-shot migration
container, the API, and the web frontend in dependency order. It publishes
plain HTTP for an external TLS terminator; it does not contain certificates or
pretend to listen on port 443.

```bash
cd deployments
cp .env.prod.example .env.prod

# Edit every CHANGE_ME value, ALLOWED_ORIGINS, and SITE_URL first.
# Hex avoids reserved URL characters in the internal connection strings.
openssl rand -hex 32

docker compose \
  --env-file .env.prod \
  --file docker-compose.prod.yml \
  up --build --detach
```

The required production variables are `POSTGRES_USER`,
`POSTGRES_PASSWORD`, `POSTGRES_DB`, `REDIS_PASSWORD`, `JWT_SECRET`,
`ALLOWED_ORIGINS`, and `SITE_URL`. Compose stops with a configuration error
when any of them is missing. Use URL-safe Postgres and Redis passwords because
they are embedded in internal connection URLs.

By default the web is published on port 3000 and the API readiness endpoint is
published on loopback port 8080. `API_URL=http://api:8080` is already wired
inside the Compose network, so browser API, upload, MCP, A2A, and WebFinger
paths can all enter through the web service. Override `WEB_PORT` or `API_PORT`
when those host ports are occupied.

For a same-host Caddy, nginx, or Traefik installation, keep
`WEB_BIND_ADDRESS=127.0.0.1` and proxy the public HTTPS origin to
`http://127.0.0.1:3000`. The web service forwards API paths internally; the API
port does not need public exposure. If the TLS proxy runs on another machine,
choose an appropriate `WEB_BIND_ADDRESS` and firewall the published port.
`SITE_URL` and `ALLOWED_ORIGINS` must use the final public `https://` origin.

Postgres data, Redis data, and local uploads use the named `pgdata`,
`redisdata`, and `uploads` volumes. The uploads volume is mounted even when
`UPLOADS_ENABLED=false`, so enabling local uploads later does not change the
storage topology. Back up Postgres and uploads; Redis data is rebuildable.

Check a deployment with:

```bash
curl --fail http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:3000/
```

CI resolves the production file and boots it against empty, project-scoped
volumes, then verifies API readiness, a web request, and the web-to-API rewrite.
The same disposable smoke test is available locally as
`make smoke-production-compose`.

Any container platform also works (Fly.io, Railway, ECS, Container Apps), but
managed Postgres must provide pgvector and migrations must finish before API
readiness.
