FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE=api
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/${SERVICE}

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

COPY --from=builder /bin/service /usr/local/bin/service

RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser
EXPOSE 8080

# Container-level liveness check. cmd/api serves /healthz on PORT
# (default 8080); cmd/gateway serves /healthz on GATEWAY_PORT (default
# 8081). Override HEALTHCHECK_PORT at build/run time for the gateway
# image. Azure Container Apps has its own probe config and ignores
# this directive — this is here for local docker / docker-compose.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget --quiet --tries=1 --spider "http://localhost:${HEALTHCHECK_PORT:-8080}/healthz" || exit 1

ENTRYPOINT ["/usr/local/bin/service"]
