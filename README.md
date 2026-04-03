# Event Booking System

Go-based microservices for event management and high-concurrency seat booking with Redis-backed quota control.

## Architecture

The stack runs with Docker Compose:

- event-service (Fiber, GORM, PostgreSQL, Redis cache)
- booking-service (Fiber, GORM, PostgreSQL, Redis quota/locks)
- postgres
- redis

| Service | Port | Purpose |
|---|---:|---|
| event-service | 8081 | CRUD for events + Redis caching for read endpoints |
| booking-service | 8082 | Booking API with atomic seat claiming and waitlist |
| postgres | 5432 | Persistent storage |
| redis | 6379 | Cache, quota counters, waitlist sequence, idempotency locks |

## Booking Flow (Current Implementation)

1. Client calls booking-service `POST /api/v1/bookings`.
2. booking-service validates input and creates a short idempotency lock in Redis (`SETNX`).
3. booking-service atomically decrements seat quota in Redis via Lua script.
4. If quota is exhausted, booking-service creates a waitlist entry with Redis-backed sequence (`event:{id}:waitlist:seq`).
5. If quota is available, booking-service stores a confirmed booking in PostgreSQL.
6. On DB write failure after successful decrement, quota is rolled back with Redis `INCR`.

## API Endpoints

### event-service (http://localhost:8081)

- `GET /api/v1/events`
- `POST /api/v1/events`
- `GET /api/v1/events/:id`
- `PUT /api/v1/events/:id`
- `DELETE /api/v1/events/:id`
- `GET /swagger/`
- `GET /docs/openapi.yaml`

### booking-service (http://localhost:8082)

- `POST /api/v1/bookings`
- `GET /swagger/`
- `GET /docs/openapi.yaml`

## Run Locally

Prerequisite: Docker + Docker Compose.

```bash
# start services
make start

# stop services
make stop

# rebuild images
make build

# restart stack
make restart

# stream logs
make logs
```

Direct Compose alternative:

```bash
docker compose up -d --build
docker compose logs -f
docker compose down
```

## Environment Variables

### event-service

- `SERVER_PORT` (default `8081`)
- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`, `REDIS_DB`
- `CACHE_TTL` (default `5m`)

### booking-service

- `SERVER_PORT` (default `8082`)
- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
- `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`, `REDIS_DB`
- `EVENT_SERVICE_URL` (default `http://localhost:8081`)
- `BOOKING_LOCK_TTL` (default `1m`)
- `REDIS_QUOTA_TTL` (default `24h`)

## Load Test (k6)

The repository includes `k6/concurrent_flow_test.js` for concurrent end-to-end flow testing.

Run locally:

```bash
k6 run ./k6/concurrent_flow_test.js
```

Supported environment overrides:

- `EVENT_BASE_URL` (default `http://localhost:8081`)
- `BOOKING_BASE_URL` (default `http://localhost:8082`)
- `EVENT_ID` (default `1`)
- `QUOTA` (default `50`)

Example:

```bash
EVENT_BASE_URL=http://localhost:8081 \
BOOKING_BASE_URL=http://localhost:8082 \
EVENT_ID=1 \
QUOTA=50 \
k6 run ./k6/concurrent_flow_test.js
```

## Notes

- booking-service enforces duplicate protection using a unique `(event_id, uid)` index.
- Stable `uid` is derived from user email (UUID SHA1) for deterministic identity.
- event-service caches event read responses in Redis and invalidates cache on create/update/delete.
