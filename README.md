# Event Booking System

A microservice-based event booking system with concurrency-safe seat reservations and automatic waitlist management.

## Services

```
┌──────────────┐     ┌───────────────────┐     ┌─────────────────┐
│  xxx    │────▶│  Event Service    │     │ Booking Service │
│  xxx   │     │  (Go) :8081       │◀────│  (Go) :8082     │
│  :xxx       │────▶│                   │     │                 │
└──────────────┘     └───────────────────┘     └─────────────────┘
                               │                        │
                               ▼                        ▼
                          PostgreSQL               PostgreSQL + Redis
```

| Service | Description | Port |
|---|---|---|
| `event-service` | Manages event catalog (create, list, get by ID) | 8081 |
| `booking-service` | Handles seat reservations and waitlist | 8082 |

## Booking Flow

```
User submits booking
        │
        ▼
 booking-service
        │
        ├─ 1. Fetch event details from event-service (cached in Redis)
        ├─ 2. Check for duplicate booking or waitlist entry
        ├─ 3. Init seat counter in Redis (seeded from Postgres on first request)
        ├─ 4. Atomically claim a seat: Redis DECR
        │
        ├─ Seat available (remaining ≥ 0)
        │       └─▶ Save confirmed Booking in Postgres
        │           Enqueue booking to Redis list for downstream processing
        │           Return  { status: "confirmed", booking: { seat_number: N } }
        │
        └─ No seat (remaining < 0)
                └─▶ Rollback: Redis INCR
                    Save WaitlistEntry in Postgres (ordered by position)
                    Return  { status: "waitlisted", waitlist_entry: { position: N } }
```

**Concurrency safety**
- Redis `DECR` is atomic — no two users can claim the same seat simultaneously.
- A `(event_id, uid)` unique constraint in Postgres is the authoritative guard against double-booking.
- User identity (UID) is derived from email via UUID v5, so the same email always maps to the same UID.

## Run Locally

**Prerequisites:** Docker and Docker Compose.

```bash
# Start all services (Postgres, Redis, event-service, booking-service, web-app)
make start

# Stop everything
make stop

# View logs
make logs
```

Once running:

| URL | Description |
|---|---|
| http://localhost:3000 | Web app |
| http://localhost:8081/swagger/ | Event Service API docs |
| http://localhost:8082/swagger/ | Booking Service API docs |

## Load Testing

Concurrency tests use [k6](https://k6.io). With all services running, you can run k6 via Docker:

```bash
docker run --rm -i \
        --add-host=host.docker.internal:host-gateway \
        -v "${PWD}/k6:/scripts" \
        grafana/k6 run \
        -e EVENT_SERVICE_URL=http://host.docker.internal:8081 \
        -e BOOKING_SERVICE_URL=http://host.docker.internal:8082 \
        -e SEAT_LIMIT=100 \
        -e CONCURRENT_USERS=2000 \
        /scripts/booking_concurrent_test.js
```

The test verifies that exactly `SEAT_LIMIT` bookings are confirmed and the rest are waitlisted, with zero errors.
