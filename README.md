# Event-Driven Notification System

A scalable notification system that processes and delivers messages through SMS, Email, and Push channels with priority queuing, rate limiting, intelligent retry logic, and real-time observability.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  REST API (chi)                  │
│  POST /notifications    GET /notifications/{id}  │
│  POST /notifications/batch    GET /batches/{id}  │
│  DELETE /notifications/{id}   GET /api/v1/metrics│
└──────────────────────┬──────────────────────────┘
                       │
               ┌───────▼────────┐
               │    Service     │  idempotency, cancel
               │    Layer       │  state machine, batch
               └───────┬────────┘
                       │
          ┌────────────┴────────────┐
          │                         │
  ┌───────▼──────┐        ┌─────────▼──────┐
  │  Repository  │        │ Priority Queue  │
  │  (pgx/v5)   │        │ high/normal/low │
  └───────┬──────┘        └─────────┬──────┘
          │                         │
   PostgreSQL              ┌────────▼────────┐
                           │   Worker Pool   │
                           │  15 goroutines  │
                           └────────┬────────┘
                                    │
                           ┌────────▼────────┐
                           │  Rate Limiter   │
                           │ 100 req/s/ch    │
                           └────────┬────────┘
                                    │
                           ┌────────▼────────┐
                           │  webhook.site   │
                           │   (provider)    │
                           └─────────────────┘

Background goroutines:
  RetryWorker     — polls DB every 10s for failed notifications due for retry
  SchedulerWorker — polls DB every 5s for scheduled notifications due for delivery
```

### Key Design Decisions

| Concern | Decision | Rationale |
|---|---|---|
| Queue | In-process buffered channels | No extra infra; double-select pattern ensures priority ordering |
| Priority | Double-select (drain high first, then fair-select) | High items never starved; workers never spin |
| Rate limit | `golang.org/x/time/rate` per channel | Token bucket, official Go library, zero deps |
| Retry | DB-backed `next_retry_at` + polling worker | Survives restarts; decoupled from worker lifecycle |
| Idempotency | `UNIQUE idempotency_key` in DB | Atomic at DB level; no race conditions |
| Migrations | `golang-migrate` at startup | `docker compose up` is truly one command |
| Metrics | `/metrics` (Prometheus) + `/api/v1/metrics` (JSON) | Satisfies both ops tooling and API consumers |
| Error mapping | Sentinel errors in domain, `mapError()` in one handler | Domain stays HTTP-free; all status codes in one place |
| Graceful shutdown | ctx cancel → HTTP drain → worker pool wait | No in-flight message is dropped on SIGTERM |

## Quick Start

```bash
# 1. Clone
git clone https://github.com/ricirt/event-driven-arch
cd event-driven-arch

# 2. Set your webhook.site URL
export PROVIDER_BASE_URL=https://webhook.site/your-uuid-here

# 3. One command — builds image, starts postgres, runs migrations, starts server
docker compose up --build
```

Server is available at `http://localhost:8080`.

## API Reference

### Create a Notification

```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -H "X-Idempotency-Key: order-shipped-42" \
  -d '{
    "channel":   "sms",
    "recipient": "+905551234567",
    "content":   "Your order has shipped.",
    "priority":  "high"
  }'
```

**Response `201 Created`:**
```json
{
  "id": "a4d86c6f-dacc-4c02-8883-6d7423ece42c",
  "channel": "sms",
  "recipient": "+905551234567",
  "content": "Your order has shipped.",
  "priority": "high",
  "status": "queued",
  "retry_count": 0,
  "max_retries": 3,
  "created_at": "2026-02-22T17:09:35Z",
  "updated_at": "2026-02-22T17:09:35Z"
}
```

### Schedule a Notification

```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "channel":      "email",
    "recipient":    "user@example.com",
    "content":      "Your weekly digest is ready.",
    "priority":     "normal",
    "scheduled_at": "2026-03-01T09:00:00Z"
  }'
```

### Create a Batch (up to 1000)

```bash
curl -X POST http://localhost:8080/api/v1/notifications/batch \
  -H "Content-Type: application/json" \
  -d '{
    "notifications": [
      {"channel":"sms",   "recipient":"+901111111111","content":"Flash sale!","priority":"high"},
      {"channel":"email", "recipient":"a@b.com",      "content":"Flash sale!","priority":"high"},
      {"channel":"push",  "recipient":"device-token", "content":"Flash sale!","priority":"high"}
    ]
  }'
```

### Get Notification Status

```bash
curl http://localhost:8080/api/v1/notifications/{id}
```

### List with Filters

```bash
# Filter by status and channel, paginate
curl "http://localhost:8080/api/v1/notifications?status=sent&channel=sms&page=1&limit=20"

# Filter by date range
curl "http://localhost:8080/api/v1/notifications?from=2026-02-01T00:00:00Z&to=2026-02-28T23:59:59Z"
```

### Cancel a Notification

```bash
curl -X DELETE http://localhost:8080/api/v1/notifications/{id}
# 204 No Content on success
# 409 Conflict if already sent/processing/cancelled
```

### Get Batch Status

```bash
curl http://localhost:8080/api/v1/batches/{batch-id}
```

### Metrics

```bash
# JSON snapshot (queue depths)
curl http://localhost:8080/api/v1/metrics

# Prometheus scrape format
curl http://localhost:8080/metrics
```

### Health Check

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

## Retry Logic

Failed deliveries are retried with exponential backoff:

| Attempt | Delay |
|---------|-------|
| 1st retry | 5 seconds |
| 2nd retry | 30 seconds |
| 3rd retry | 120 seconds |
| After 3rd | `status = failed` permanently |

Retry state is persisted in the database (`next_retry_at` column) so retries survive server restarts.

## Priority Queue

```
Enqueue → [high: 1000] [normal: 5000] [low: 2000]
                           ↓
         Worker double-select:
           1. Non-blocking drain of high channel
           2. Fair blocking select across all three when high is empty
```

High-priority items are never starved by a flood of normal/low items.

## Rate Limiting

Each channel (SMS, Email, Push) has its own token bucket limiter capped at **100 tokens/second**. Workers call `limiter.Wait()` before every provider send — back-pressure is applied at the worker level, not at the API level.

## Configuration

All settings are environment variables with sensible defaults:

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | *(required)* | PostgreSQL connection string |
| `HTTP_PORT` | `8080` | Server listen port |
| `PROVIDER_BASE_URL` | *(required)* | External notification provider URL |
| `SMS_WORKERS` | `5` | Number of SMS worker goroutines |
| `EMAIL_WORKERS` | `5` | Number of Email worker goroutines |
| `PUSH_WORKERS` | `5` | Number of Push worker goroutines |
| `RATE_LIMIT_PER_CHANNEL` | `100` | Max sends per second per channel |
| `RETRY_BACKOFF_1` | `5s` | Delay before 1st retry |
| `RETRY_BACKOFF_2` | `30s` | Delay before 2nd retry |
| `RETRY_BACKOFF_3` | `120s` | Delay before 3rd retry |
| `SCHEDULER_INTERVAL` | `5s` | How often the scheduler polls for due notifications |
| `RETRY_INTERVAL` | `10s` | How often the retry worker polls for due retries |
| `SHUTDOWN_TIMEOUT` | `30s` | Graceful HTTP shutdown timeout |

## Development

```bash
# Run tests (race detector enabled)
make test

# Run with coverage report
make test-cover

# Lint
make lint

# Build binary
make build

# Start / stop Docker environment
make docker-up
make docker-down
```

## Database Migrations

Migrations run automatically at startup via `golang-migrate`. SQL files are in `migrations/`:

```
migrations/
  000001_create_batches.up.sql
  000001_create_batches.down.sql
  000002_create_notifications.up.sql
  000002_create_notifications.down.sql
```

To run manually:
```bash
make migrate-up    # apply all pending migrations
make migrate-down  # roll back last migration
```

## API Documentation

OpenAPI 3.0 specification: [`docs/swagger.yaml`](docs/swagger.yaml)

## Project Structure

```
.
├── cmd/server/main.go          # Entry point: wires all deps, graceful shutdown
├── internal/
│   ├── api/                    # HTTP layer (router, handlers, middleware)
│   ├── config/                 # Env-based config loader
│   ├── db/                     # pgxpool setup + golang-migrate runner
│   ├── domain/                 # Core types, enums, sentinel errors, validation
│   ├── metrics/                # Prometheus instruments
│   ├── provider/               # External provider interface + webhook.site impl
│   ├── queue/                  # Priority queue (double-select pattern)
│   ├── ratelimiter/            # Per-channel token bucket
│   ├── repository/             # NotificationRepository interface + pgx impl
│   ├── service/                # Business logic (idempotency, cancel state machine)
│   └── worker/                 # Worker, Pool, RetryWorker, SchedulerWorker
├── migrations/                 # Versioned SQL migrations
├── docs/swagger.yaml           # OpenAPI 3.0 specification
├── Dockerfile                  # Multi-stage build (golang:1.24 → distroless)
└── docker-compose.yml          # postgres + app, one-command setup
```
