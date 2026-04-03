/**
 * k6 Concurrent Flow Test – Event Service + Booking Service
 * Goal: simulate 1000 concurrent users running the full booking flow:
 *       1. List events  (event-service)
 *       2. Book a seat  (booking-service)
 *
 * Run:
 *   k6 run k6/concurrent_flow_test.js
 *
 * Override defaults:
 *   EVENT_BASE_URL=http://localhost:8081 \
 *   BOOKING_BASE_URL=http://localhost:8082 \
 *   EVENT_ID=1 QUOTA=1000 \
 *   k6 run k6/concurrent_flow_test.js
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

// ── Config ────────────────────────────────────────────────────────────────────
const EVENT_BASE_URL = __ENV.EVENT_BASE_URL || "http://localhost:8081";
const BOOKING_BASE_URL = __ENV.BOOKING_BASE_URL || "http://localhost:8082";
const EVENT_ID = parseInt(__ENV.EVENT_ID || "1", 10);
const QUOTA = parseInt(__ENV.QUOTA || "50", 10);
const VUS = parseInt(__ENV.VUS || "300", 10);
const RAMP_UP = __ENV.RAMP_UP || "10s";
const HOLD = __ENV.HOLD || "20s";
const RAMP_DOWN = __ENV.RAMP_DOWN || "5s";
const REQUEST_TIMEOUT = __ENV.REQUEST_TIMEOUT || "10s";

// ── Custom metrics ────────────────────────────────────────────────────────────
const bookingConfirmed = new Counter("booking_confirmed");
const bookingWaitlisted = new Counter("booking_waitlisted");
const bookingFailed = new Counter("booking_failed");
const errorRate = new Rate("error_rate");
const listEventsDuration = new Trend("list_events_duration_ms", true);
const bookingDuration = new Trend("booking_duration_ms", true);

// ── Options: ramp to 1000 concurrent VUs ─────────────────────────────────────
export const options = {
  scenarios: {
    concurrent_booking: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: RAMP_UP, target: VUS }, // ramp up
        { duration: HOLD, target: VUS }, // hold
        { duration: RAMP_DOWN, target: 0 }, // ramp down
      ],
      gracefulRampDown: "10s",
    },
  },
  thresholds: {
    error_rate: [{ threshold: "rate<0.05" }],           // < 5% errors
    http_req_failed: [{ threshold: "rate<0.05" }],      // < 5% HTTP failures
    booking_duration_ms: [
      { threshold: "p(95)<1000" },                      // p95 < 1s
      { threshold: "p(99)<3000" },                      // p99 < 3s
    ],
    list_events_duration_ms: [{ threshold: "p(95)<500" }],
  },
};

// ── Setup: seed event + quota once before VUs start ──────────────────────────
export function setup() {
  const eventRes = http.post(
    `${EVENT_BASE_URL}/api/v1/events`,
    JSON.stringify({
      name: "Load Test Event",
      description: "Created by k6 setup",
      seat_limit: QUOTA,
      start_date: "2030-01-01T09:00:00Z",
      end_date: "2030-01-01T18:00:00Z",
    }),
    {
      headers: { "Content-Type": "application/json" },
      timeout: REQUEST_TIMEOUT,
    }
  );
  if (eventRes.status === 200 || eventRes.status === 201) {
    try {
      const body = JSON.parse(eventRes.body);
      if (body?.id) {
        console.log(`[setup] event ready - id=${body.id}`);
        return { eventId: body.id };
      }
    } catch (_) {
      // fall back to EVENT_ID below when response is not JSON
    }
    console.log(`[setup] event ready - status=${eventRes.status}, using EVENT_ID=${EVENT_ID}`);
    return { eventId: EVENT_ID };
  }

  console.log(`[setup] event create failed status=${eventRes.status}, using EVENT_ID=${EVENT_ID}`);
  return { eventId: EVENT_ID };
}

// ── Default function (runs per VU) ───────────────────────────────────────────
export default function (data) {
  const uid = `vu${__VU}_iter${__ITER}`;
  const targetEventID = data?.eventId || EVENT_ID;

  // ── Step 1: List events (event-service) ────────────────────────────────────
  const t0 = Date.now();
  const listRes = http.get(`${EVENT_BASE_URL}/api/v1/events`, {
    tags: { name: "ListEvents" },
    timeout: REQUEST_TIMEOUT,
  });
  listEventsDuration.add(Date.now() - t0);

  check(listRes, {
    "list events 200": (r) => r.status === 200,
    "list events no 5xx": (r) => r.status > 0 && r.status < 500,
  });

  if (listRes.status !== 200) {
    errorRate.add(1);
    console.warn(`[VU ${__VU}] list events failed: status=${listRes.status}`);
    return;
  }
  errorRate.add(0);

  // ── Step 2: Book a seat (booking-service) ──────────────────────────────────
  const payload = JSON.stringify({
    event_id: targetEventID,
    user_name: `User ${uid}`,
    user_email: `${uid}@flow.test`,
    user_phone: "0800000000",
  });

  const t1 = Date.now();
  const bookRes = http.post(
    `${BOOKING_BASE_URL}/api/v1/bookings`,
    payload,
    {
      headers: { "Content-Type": "application/json" },
      tags: { name: "BookSeat" },
      timeout: REQUEST_TIMEOUT,
    }
  );
  bookingDuration.add(Date.now() - t1);

  check(bookRes, {
    "book seat 201": (r) => r.status === 201,
    "book seat no 5xx": (r) => r.status > 0 && r.status < 500,
  });

  if (bookRes.status === 201) {
    try {
      const body = JSON.parse(bookRes.body);
      if (body.status === "waitlisted") {
        bookingWaitlisted.add(1);
      } else {
        bookingConfirmed.add(1);
      }
    } catch (_) {
      bookingConfirmed.add(1);
    }
    errorRate.add(0);
  } else if (bookRes.status === 409) {
    // Duplicate or in-progress – expected under high concurrency
    bookingFailed.add(1);
    errorRate.add(0);
  } else {
    bookingFailed.add(1);
    errorRate.add(1);
    console.warn(`[VU ${__VU}] book seat failed: status=${bookRes.status} body=${bookRes.body?.substring(0, 200)}`);
  }

  sleep(0);
}
