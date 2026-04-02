# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o server ./cmd/main.go

# Run all tests
go test ./...

# Run tests in a specific package with verbose output
go test ./internal/service -v

# Run a single test
go test ./internal/service -run TestBookEvent_Confirmed -v

# Run with coverage
go test ./internal/service -cover

# Regenerate mocks (requires mockery installed)
mockery
```

## Architecture

This is an **event booking microservice** written in Go. It handles seat reservations with concurrency safety and a waitlist fallback.

**Stack:** Fiber v2 (HTTP), GORM + PostgreSQL (persistence), Redis (atomic counters + queue), Swagger (docs)

**Key layers:**
- `cmd/main.go` — entry point; wires all dependencies
- `config/` — env-based config (see `.env.example`)
- `internal/http/` — Fiber router, handlers, API key auth middleware
- `internal/service/` — core booking logic (`booking_service.go`)
- `internal/repository/` — GORM-backed DB repos + Redis-backed seat/queue repos
- `internal/client/` — HTTP client to the external event service
- `internal/model/` — GORM models (`Booking`, `WaitlistEntry`)

**Routes** (all under `/api/v1/bookings`, gated by `X-API-Key`):
- `POST /` — create a booking
- `GET /user/:uid` — get bookings by user UID
- `GET /event/:eventID` — get all bookings for an event

## Core Booking Flow

1. Derive a deterministic `uid` (UUID v5 from email) — same email always maps to the same UID
2. Check for duplicate bookings in DB and waitlist
3. **Atomically claim a seat via Redis `DECR`** on key `seats:{eventID}`
   - Counter initialized lazily via `SETNX` (seeded from DB count + event capacity)
   - If `DECR` result < 0: add to waitlist, rollback via `INCR`
   - If `DECR` result ≥ 0: persist confirmed `Booking` to Postgres
4. Enqueue confirmed booking onto `booking:queue:{eventID}` Redis list

## Concurrency Design

The seat counter (`seats:{eventID}`) in Redis is the single source of truth for available seats. `DECR` is atomic, so concurrent requests cannot double-book. Postgres unique constraints on `(event_id, uid)` serve as a safety net. Event data is cached in Redis with a 5-minute TTL to avoid hammering the event service.

## Testing

Tests use **testify/suite** with mockery-generated mocks for all interfaces (`BookingRepository`, `WaitlistRepository`, `QueueRepository`, `SeatRepository`, `EventGetter`). Mocks live in `internal/repository/mocks/` and `internal/client/mocks/`.

To add new tests, follow the `BookingServiceSuite` pattern in `internal/service/booking_service_test.go`.

## Environment

Copy `.env.example` and configure:
- `SERVER_PORT` (default `8082`)
- `API_KEY` — leave empty to disable auth in dev
- `EVENT_SERVICE_URL` — URL of the companion event service
- Postgres and Redis connection details
