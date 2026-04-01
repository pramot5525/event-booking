import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate } from "k6/metrics";

// ── Custom metrics ────────────────────────────────────────────────────────────
const confirmedCount  = new Counter("bookings_confirmed");
const waitlistedCount = new Counter("bookings_waitlisted");
const errorCount      = new Counter("bookings_error");
const successRate     = new Rate("booking_success_rate");

// ── Config ────────────────────────────────────────────────────────────────────
const EVENT_SERVICE_URL  = __ENV.EVENT_SERVICE_URL  || "http://localhost:8081";
const BOOKING_SERVICE_URL = __ENV.BOOKING_SERVICE_URL || "http://localhost:8082";
const SEAT_LIMIT          = parseInt(__ENV.SEAT_LIMIT || "100");
const CONCURRENT_USERS    = 2000;

export const options = {
  scenarios: {
    concurrent_booking: {
      executor: "shared-iterations",
      vus: CONCURRENT_USERS,
      iterations: CONCURRENT_USERS,
      maxDuration: "120s",
    },
  },
  thresholds: {
    // confirmed bookings must not exceed seat limit
    bookings_confirmed: [`count<=${SEAT_LIMIT}`],
    // all requests should complete (no unexpected crashes)
    http_req_failed: ["rate<0.01"],
  },
};

// ── Setup: create event once before test ─────────────────────────────────────
export function setup() {
  const payload = JSON.stringify({
    name:        "Concurrent Test Event",
    description: "k6 load test",
    seat_limit:  SEAT_LIMIT,
    start_date:  "2026-06-01T09:00:00Z",
    end_date:    "2026-06-01T18:00:00Z",
  });

  const res = http.post(`${EVENT_SERVICE_URL}/api/v1/events`, payload, {
    headers: { "Content-Type": "application/json" },
  });

  check(res, { "event created": (r) => r.status === 200 });

  const body = JSON.parse(res.body);
  if (!body.id) {
    throw new Error(`Failed to create event: ${res.body}`);
  }

  console.log(`Event created with id=${body.id}, seat_limit=${SEAT_LIMIT}`);
  return { eventId: body.id };
}

// ── Main: each VU books once with unique email ────────────────────────────────
export default function (data) {
  const vuId = __VU * 1000 + __ITER; // unique per VU+iteration

  const payload = JSON.stringify({
    event_id:   data.eventId,
    user_name:  `User ${vuId}`,
    user_email: `user_${vuId}@loadtest.com`,
    user_phone: `08${String(vuId).padStart(8, "0")}`,
  });

  const res = http.post(`${BOOKING_SERVICE_URL}/api/v1/bookings`, payload, {
    headers: { "Content-Type": "application/json" },
  });

  const ok = check(res, {
    "status 201": (r) => r.status === 201,
  });

  if (res.status === 201) {
    const body = JSON.parse(res.body);
    if (body.status === "confirmed") {
      confirmedCount.add(1);
    } else if (body.status === "waitlisted") {
      waitlistedCount.add(1);
    }
    successRate.add(true);
  } else {
    errorCount.add(1);
    successRate.add(false);
    console.error(`VU ${__VU} failed: ${res.status} ${res.body}`);
  }
}

// ── Teardown: print summary ───────────────────────────────────────────────────
export function teardown(data) {
  console.log(`\n========== Booking Test Summary ==========`);
  console.log(`Event ID    : ${data.eventId}`);
  console.log(`Seat Limit  : ${SEAT_LIMIT}`);
  console.log(`Total VUs   : ${CONCURRENT_USERS}`);
  console.log(`==========================================`);
}
