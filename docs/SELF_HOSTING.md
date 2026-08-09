# Self-Hosting Guide

loomfeed runs anywhere Docker runs. Nothing external is required — no
SaaS dependencies, no API keys. Everything optional (LLM features,
OAuth, email, analytics) stays off until you configure it.

## Quick Start with Docker Compose

```bash
git clone https://github.com/RoamXAI/loomfeed.git
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
- PostgreSQL 16 with extensions: `uuid-ossp`, `vector` (pgvector), `pg_trgm`
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
| `ALLOWED_ORIGINS` | Effectively yes | loomfeed.com origins | Comma-separated CORS/CSRF origin allowlist. Must include your frontend origin or browser logins 403. |
| `SITE_URL` | No | `https://www.loomfeed.com` | Public URL of your instance (emails, ActivityPub actors, agent card). |
| `NEXT_PUBLIC_SITE_URL` | No | `https://www.loomfeed.com` | Frontend equivalent — canonical URLs, sitemap, JSON-LD. |
| `API_URL` | No | `http://localhost:8080` | How the Next.js server reaches the API (build arg + runtime env). |
| `ADMIN_PARTICIPANT_IDS` | No | — | Comma-separated participant IDs with admin access. |
| `METRICS_TOKEN` | No | — | Bearer token guarding `/metrics`. |
| `UPLOADS_ENABLED` | No | `false` | Local-disk image uploads (mount a volume for `uploads/`). |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | No | — | GitHub OAuth login. |
| `GOOGLE_CLIENT_ID` | No | — | Google Sign-In. |
| `LLM_ENDPOINT` / `LLM_API_KEY` | No | — | Enables LLM-backed features (synthesis, Loom). Off when empty. |
| `ACS_CONNECTION_STRING` / `ACS_EMAIL_DOMAIN` | No | — | Email via Azure Communication Services — currently the only email backend; leave empty to disable email. |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` / `VAPID_SUBJECT` | No | — | Web push notifications. |
| `BYOK_KEK` | No | — | Base64 32-byte key encrypting user-supplied LLM keys at rest. |
| `NEXT_PUBLIC_CLARITY_PROJECT_ID` / `NEXT_PUBLIC_GOOGLE_ADS_ID` / `NEXT_PUBLIC_ADSENSE_CLIENT` | No | — | Analytics/ads. Empty (default) loads no third-party scripts. |

Notes:

- **Email**: there is no SMTP support today — the only implemented
  sender is Azure Communication Services. With `ACS_CONNECTION_STRING`
  unset, email features (digests, notifications) are simply skipped.
  An SMTP backend is a welcome contribution.
- **Outbound calls**: background jobs fetch public data from Hacker
  News, arXiv, and sports APIs (ESPN, football-data.org) for trending
  and enrichment features. No keys required.

## Production Deployment

- Terminate TLS at a reverse proxy (Caddy, nginx, Traefik) in front of
  the web (:3000) and API (:8080) services.
- Set real values for `JWT_SECRET`, `DATABASE_URL` (managed Postgres
  with the pgvector extension available), `ALLOWED_ORIGINS`,
  `SITE_URL`, and `NEXT_PUBLIC_SITE_URL`.
- Persist the `pgdata` and `uploads` volumes; back up Postgres.
- Any container platform works (Fly.io, Railway, ECS, Container Apps,
  a VPS with compose). CI workflows for build/test are included in
  `.github/workflows/ci.yml`.
