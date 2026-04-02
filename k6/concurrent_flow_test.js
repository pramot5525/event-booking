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
 *   EVENT_ID=1 QUOTA=5000 \
 *   k6 run k6/concurrent_flow_test.js
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

// ── Config ────────────────────────────────────────────────────────────────────
const EVENT_BASE_URL = __ENV.EVENT_BASE_URL || "http://localhost:8081";
const BOOKING_BASE_URL = __ENV.BOOKING_BASE_URL || "http://localhost:8082";
const EVENT_ID = parseInt(__ENV.EVENT_ID || "1", 10);
const QUOTA = parseInt(__ENV.QUOTA || "5000", 10);

// ── Custom metrics ────────────────────────────────────────────────────────────
const bookingConfirmed = new Counter("booking_confirmed");
const bookingWaitlisted = new Counter("booking_waitlisted");
const bookingFailed = new Counter("booking_failed");
const errorRate = new Rate("error_rate");
const listEventsDuration = new Trend("list_events_duration_ms", true);
const bookingDuration = new Trend("booking_duration_ms", true);

// ── Options: 1000 concurrent VUs ─────────────────────────────────────────────
export const options = {
  scenarios: {
    concurrent_booking: {
      executor: "constant-vus",
      vus: 1000,
      duration: "1m",
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
  // 1. Ensure the target event exists (create if needed)
  const eventRes = http.post(
    `${EVENT_BASE_URL}/api/v1/events`,
    JSON.stringify({
      name: "Load Test Event",
      description: "Created by k6 setup",
      seat_limit: QUOTA,
      start_date: "2030-01-01T09:00:00Z",
      end_date: "2030-01-01T18:00:00Z",
    }),
    { headers: { "Content-Type": "application/json" } }
  );
  if (eventRes.status === 201) {
    const body = JSON.parse(eventRes.body);
    console.log(`[setup] event created – id=${body.id}`);
  } else {
    console.log(`[setup] event create status=${eventRes.status} (may already exist)`);
  }

  // 2. Init booking quota in Redis
  const quotaRes = http.post(
    `${BOOKING_BASE_URL}/api/v1/bookings/quota/init`,
    JSON.stringify({ event_id: EVENT_ID, quota: QUOTA }),
    { headers: { "Content-Type": "application/json" } }
  );
  if (quotaRes.status !== 200) {
    console.error(`[setup] quota init failed: status=${quotaRes.status} body=${quotaRes.body}`);
  } else {
    console.log(`[setup] quota initialized – event_id=${EVENT_ID} quota=${QUOTA}`);
  }
}

// ── Default function (runs per VU) ───────────────────────────────────────────
export default function () {
  const uid = `vu${__VU}_iter${__ITER}`;

  // ── Step 1: List events (event-service) ────────────────────────────────────
  const t0 = Date.now();
  const listRes = http.get(`${EVENT_BASE_URL}/api/v1/events`, {
    tags: { name: "ListEvents" },
  });
  listEventsDuration.add(Date.now() - t0);

  check(listRes, {
    "list events 200": (r) => r.status === 200,
    "list events no 5xx": (r) => r.status < 500,
  });

  if (listRes.status !== 200) {
    errorRate.add(1);
    console.warn(`[VU ${__VU}] list events failed: status=${listRes.status}`);
    return;
  }
  errorRate.add(0);

  // ── Step 2: Book a seat (booking-service) ──────────────────────────────────
  const payload = JSON.stringify({
    event_id: EVENT_ID,
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
    }
  );
  bookingDuration.add(Date.now() - t1);

  const bookOk = check(bookRes, {
    "book seat 201": (r) => r.status === 201,
    "book seat no 5xx": (r) => r.status < 500,
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
