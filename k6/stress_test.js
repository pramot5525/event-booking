import http from "k6/http";
import { check } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

// ── Custom metrics ────────────────────────────────────────────────────────────
const confirmedCount  = new Counter("bookings_confirmed");
const waitlistedCount = new Counter("bookings_waitlisted");
const errorCount      = new Counter("bookings_error");
const successRate     = new Rate("booking_success_rate");
const bookingLatency  = new Trend("booking_latency_ms", true);

// ── Config ────────────────────────────────────────────────────────────────────
const EVENT_SERVICE_URL   = __ENV.EVENT_SERVICE_URL   || "http://localhost:8081";
const BOOKING_SERVICE_URL = __ENV.BOOKING_SERVICE_URL || "http://localhost:8082";
const SEAT_LIMIT          = parseInt(__ENV.SEAT_LIMIT || "9999999");

// ── Scenario ──────────────────────────────────────────────────────────────────
// ramping-arrival-rate: k6 drives a target RPS and spawns only as many VUs as
// needed to sustain it — no upfront VU pre-allocation, so it runs on any machine.
//
// Equivalent user concurrency at each stage (assuming ~200 ms avg response):
//   100 rps  ≈    20 concurrent users
//   500 rps  ≈   100 concurrent users
//  1 000 rps  ≈   200 concurrent users
//  5 000 rps  ≈ 1 000 concurrent users
// 10 000 rps  ≈ 2 000 concurrent users   ← previous test peak
// 50 000 rps  ≈ 10 000 concurrent users
//
// To run the original ramping-vus version at true 200k VUs, use k6 Cloud:
//   k6 cloud --vus 200000 stress_test.js
export const options = {
  scenarios: {
    stress: {
      executor:        "ramping-arrival-rate",
      startRate:       0,
      timeUnit:        "1s",
      preAllocatedVUs: 500,   // initial pool; k6 grows it as needed
      maxVUs:          5000,  // safety cap — raise for cloud / large machines
      stages: [
        { duration: "30s", target:     50 }, // warm-up        50 rps
        { duration: "30s", target:     50 }, // hold
        { duration: "60s", target:    500 }, // ramp →   500 rps
        { duration: "30s", target:    500 }, // hold
        { duration: "60s", target:  2_000 }, // ramp →  2 000 rps
        { duration: "30s", target:  2_000 }, // hold
        { duration: "60s", target:  5_000 }, // ramp →  5 000 rps
        { duration: "30s", target:  5_000 }, // hold
        { duration: "60s", target: 10_000 }, // ramp → 10 000 rps
        { duration: "30s", target: 10_000 }, // hold at peak
        { duration: "30s", target:      0 }, // ramp down
      ],
    },
  },
  thresholds: {
    http_req_duration:    ["p(95)<10000", "p(99)<20000"],
    http_req_failed:      ["rate<0.05"],
    booking_success_rate: ["rate>0.95"],
  },
};

// ── Setup: create one event ───────────────────────────────────────────────────
export function setup() {
  const res = http.post(
    `${EVENT_SERVICE_URL}/api/v1/events`,
    JSON.stringify({
      name:        "Stress Test Event",
      description: "k6 stress — ramping arrival rate",
      seat_limit:  SEAT_LIMIT,
      start_date:  "2026-06-01T09:00:00Z",
      end_date:    "2026-06-01T18:00:00Z",
    }),
    { headers: { "Content-Type": "application/json" } },
  );

  check(res, { "event created": (r) => r.status === 200 });

  const body = JSON.parse(res.body);
  if (!body.id) throw new Error(`Setup failed: ${res.body}`);

  console.log(`Event id=${body.id}  seat_limit=${SEAT_LIMIT}`);
  return { eventId: body.id };
}

// ── Main ──────────────────────────────────────────────────────────────────────
export default function (data) {
  const start = Date.now();

  const res = http.post(
    `${BOOKING_SERVICE_URL}/api/v1/bookings`,
    JSON.stringify({
      event_id:   data.eventId,
      user_name:  `Stress ${__VU}-${__ITER}`,
      user_email: `s_${__VU}_${__ITER}@loadtest.com`,
      user_phone: `08${String((__VU * 1000 + __ITER) % 100_000_000).padStart(8, "0")}`,
    }),
    { headers: { "Content-Type": "application/json" } },
  );

  bookingLatency.add(Date.now() - start);

  check(res, { "status 201": (r) => r.status === 201 });

  if (res.status === 201) {
    const body = JSON.parse(res.body);
    body.status === "confirmed" ? confirmedCount.add(1) : waitlistedCount.add(1);
    successRate.add(true);
  } else {
    errorCount.add(1);
    successRate.add(false);
    if (res.status !== 409) {
      console.error(`VU ${__VU} iter ${__ITER}: ${res.status} ${res.body}`);
    }
  }
}

// ── Teardown ──────────────────────────────────────────────────────────────────
export function teardown(data) {
  console.log("\n========== Stress Test Summary ==========");
  console.log(`Event ID   : ${data.eventId}`);
  console.log(`Seat limit : ${SEAT_LIMIT}`);
  console.log(`Peak rate  : 10,000 req/s`);
  console.log(`Max VUs    : 5,000 (pool)`);
  console.log("Stages     : 50 → 500 → 2k → 5k → 10k rps");
  console.log("==========================================");
}
