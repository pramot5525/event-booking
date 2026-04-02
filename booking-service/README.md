# Booking Service

Event booking microservice that handles seat reservations with concurrency safety and automatic waitlist fallback.

## Architecture

```
POST /bookings
      │
      ▼
 BookingService
      │
      ├─ 1. Resolve event (Redis cache → Event Service HTTP)
      │
      ├─ 2. Check duplicates (booking + waitlist)
      │
      ├─ 3. Init seat counter (Redis SETNX, seeded from Postgres on first request)
      │
      ├─ 4. Atomic claim: Redis DECR
      │         │
      │    remaining ≥ 0 ──► Persist Booking (Postgres) ──► Enqueue (Redis list)
      │         │
      │    remaining < 0 ──► Rollback INCR ──► Add WaitlistEntry (Postgres)
      │
      └─ Return { status: "confirmed" | "waitlisted", booking | waitlist_entry }
```

**Stack**

| Layer | Technology |
|---|---|
| HTTP | Go Fiber v2 |
| Database | PostgreSQL (GORM) |
| Cache / Queue | Redis |
| External dependency | Event Service (HTTP) |

**Key design decisions**

- **Deterministic user ID** — UID is derived from the user's email via UUID v5, so the same person always maps to the same UID without a user table.
- **Atomic seat claiming** — Redis `DECR` is the single source of truth for available seats. A negative result means no seat; the counter is rolled back with `INCR`.
- **Double-booking safety** — a `(event_id, uid)` unique constraint in Postgres is the authoritative guard if Redis and the service ever diverge.
- **Lazy counter init** — the seat counter is seeded from the confirmed-booking count in Postgres on the very first request per event (`SETNX` makes subsequent calls no-ops).

## API

Base URL: `http://localhost:8082`
Authentication: `X-API-Key` header (leave `API_KEY` empty in `.env` to disable auth locally).

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/bookings` | Book an event (confirmed or waitlisted) |
| `GET` | `/api/v1/bookings/user/:uid` | Get all bookings for a user |
| `GET` | `/api/v1/bookings/event/:eventID` | Get all bookings for an event |
| `GET` | `/swagger/` | Swagger UI |

**Book an event**

```http
POST /api/v1/bookings
Content-Type: application/json
X-API-Key: your-key

{
  "event_id": 1,
  "user_name": "Somchai Jaidee",
  "user_email": "somchai@example.com",
  "user_phone": "0812345678"
}
```

Response `201` — seat available:
```json
{ "status": "confirmed", "booking": { "id": 42, "seat_number": 7, ... } }
```

Response `201` — event full:
```json
{ "status": "waitlisted", "waitlist_entry": { "id": 5, "position": 3, ... } }
```

## Run locally

**Prerequisites:** Go 1.24+, PostgreSQL, Redis, and the companion Event Service running on port `8081`.

### 1. Configure environment

```bash
cp .env.example .env
# edit .env if your Postgres/Redis credentials differ
```

### 2. Export env vars and start

```bash
export $(grep -v '^#' .env | xargs)
go run ./cmd/main.go
```

The service starts on `http://localhost:8082`.

### 3. Run with Docker

```bash
docker build -t booking-service .
docker run --env-file .env -p 8082:8082 booking-service
```

## Development

```bash
# Run all tests
go test ./...

# Run a specific test
go test ./internal/service -run TestBookEvent_Confirmed -v

# Regenerate mocks (requires mockery)
mockery
```
