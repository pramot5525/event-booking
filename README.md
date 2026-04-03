# Event Booking System

Go microservices for event management and high-concurrency seat booking with PostgreSQL-backed transactions and Redis quota control.

---

## System Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌──────────────────┐
│   Client    │────▶│  event-service   │────▶│   PostgreSQL     │
│             │     │  :8081           │     │   (events DB)    │
│             │     │  Fiber + GORM    │◀───▶│                  │
│             │     └──────────────────┘     └──────────────────┘
│             │              │ cache
│             │              ▼
│             │     ┌──────────────────┐
│             │     │     Redis        │
│             │     │  :6379           │
│             │     │  cache / locks   │
│             │     └──────────────────┘
│             │              ▲ quota
│             │     ┌──────────────────┐     ┌──────────────────┐
│             │────▶│ booking-service  │────▶│   PostgreSQL     │
│             │     │  :8082           │     │   (bookings DB)  │
│             │     │  Fiber + GORM    │◀───▶│                  │
└─────────────┘     └──────────────────┘     └──────────────────┘
                             │ HTTP
                             ▼
                    event-service :8081
                    (fetch seat limit)
```

| Service         | Port | Role                                              |
|-----------------|-----:|---------------------------------------------------|
| event-service   | 8081 | Event CRUD + Redis read cache                     |
| booking-service | 8082 | Seat booking, waitlist, duplicate protection      |
| postgres        | 5432 | Persistent storage for both services              |
| redis           | 6379 | Event cache, idempotency locks, quota counters    |

---

## Database Design

### event-service — `events` table

```
events
├── id           BIGSERIAL  PRIMARY KEY
├── name         TEXT       NOT NULL
├── description  TEXT
├── seat_limit   INT        NOT NULL
├── start_date   TIMESTAMP
├── end_date     TIMESTAMP
├── created_at   TIMESTAMP
└── deleted_at   TIMESTAMP  (soft delete)
```

### booking-service — `bookings` + `event_quotas` tables

```
bookings
├── id               SERIAL      PRIMARY KEY
├── event_id         INT         NOT NULL
├── uid              VARCHAR(100) NOT NULL       ← stable UUID derived from email (SHA1)
├── user_name        TEXT        NOT NULL
├── user_email       TEXT        NOT NULL
├── user_phone       TEXT        NOT NULL
├── status           VARCHAR(20) NOT NULL        ← "confirmed" | "waitlisted"
├── waitlist_position BIGINT                     ← NULL if confirmed
└── created_at       TIMESTAMP

UNIQUE INDEX (event_id, uid)                     ← prevents duplicate bookings
INDEX        (event_id, status)

event_quotas
├── event_id      INT    PRIMARY KEY
├── seats_total   BIGINT NOT NULL
└── seats_booked  BIGINT NOT NULL  DEFAULT 0
```

---

## Booking Flow

```
Client POST /api/v1/bookings
         │
         ▼
  Validate input
         │
         ▼
  Fetch seat limit from event-service
         │
         ▼
  Upsert event_quotas row (idempotent)
         │
         ▼
  BEGIN TRANSACTION
    │
    ├─ SELECT ... FOR UPDATE on event_quotas   ← row-level lock
    │
    ├─ seats_booked < seats_total?
    │       YES ──▶ INSERT bookings (status=confirmed)
    │               INCREMENT seats_booked
    │               RETURN { status: "confirmed" }
    │
    │       NO  ──▶ GET max waitlist_position
    │               INSERT bookings (status=waitlisted, position=max+1)
    │               RETURN { status: "waitlisted", position: N }
    │
  COMMIT
         │
         ▼
  Duplicate uid+event_id? → 409 Already Booked
```

---

## API Endpoints

### event-service `http://localhost:8081`

| Method | Path                  | Description        |
|--------|-----------------------|--------------------|
| GET    | /api/v1/events        | List all events    |
| POST   | /api/v1/events        | Create event       |
| GET    | /api/v1/events/:id    | Get event by ID    |
| PUT    | /api/v1/events/:id    | Update event       |
| DELETE | /api/v1/events/:id    | Delete event       |
| GET    | /swagger/             | Swagger UI         |
| GET    | /docs/openapi.yaml    | OpenAPI spec       |

### booking-service `http://localhost:8082`

| Method | Path               | Description             |
|--------|--------------------|-------------------------|
| POST   | /api/v1/bookings   | Book a seat / join waitlist |
| GET    | /swagger/          | Swagger UI              |
| GET    | /docs/openapi.yaml | OpenAPI spec            |

**POST /api/v1/bookings — request body:**
```json
{
  "event_id":   1,
  "user_name":  "Alice",
  "user_email": "alice@example.com",
  "user_phone": "0812345678"
}
```

---

## Run Locally

Requires Docker + Docker Compose.

```bash
make start      # start all services
make stop       # stop all services
make build      # rebuild images
make restart    # rebuild and restart
make logs       # stream logs
```

Or with Compose directly:

```bash
docker compose up -d --build
docker compose logs -f
docker compose down
```

---

## Environment Variables

### event-service

| Variable           | Default  |
|--------------------|----------|
| SERVER_PORT        | 8081     |
| POSTGRES_HOST/PORT/USER/PASSWORD/DB | — |
| REDIS_HOST/PORT/PASSWORD/DB | — |
| CACHE_TTL          | 5m       |

### booking-service

| Variable           | Default                  |
|--------------------|--------------------------|
| SERVER_PORT        | 8082                     |
| POSTGRES_HOST/PORT/USER/PASSWORD/DB | — |
| REDIS_HOST/PORT/PASSWORD/DB | — |
| EVENT_SERVICE_URL  | http://localhost:8081    |
| BOOKING_LOCK_TTL   | 1m                       |
| REDIS_QUOTA_TTL    | 24h                      |

---

## Load Testing (k6)

```bash
k6 run ./k6/concurrent_flow_test.js

# with overrides
EVENT_BASE_URL=http://localhost:8081 \
BOOKING_BASE_URL=http://localhost:8082 \
EVENT_ID=1 \
QUOTA=50 \
k6 run ./k6/concurrent_flow_test.js
```

Scripts: `k6/concurrent_flow_test.js`, `k6/max_concurrency_search_test.js`
