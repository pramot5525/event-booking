/**
 * k6 Max Concurrency Search – Booking Flow
 *
 * Purpose:
 * - Run the same booking flow at increasing constant concurrency levels.
 * - Find the highest level that still meets success/error/latency thresholds.
 *
 * Run:
 *   k6 run k6/max_concurrency_search_test.js
 *
 * Optional overrides:
 *   EVENT_BASE_URL=http://localhost:8081 \
 *   BOOKING_BASE_URL=http://localhost:8082 \
 *   EVENT_ID=1 \
 *   QUOTA=200000 \
 *   STEP_DURATION=45s \
 *   k6 run k6/max_concurrency_search_test.js
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

const EVENT_BASE_URL = __ENV.EVENT_BASE_URL || "http://localhost:8081";
const BOOKING_BASE_URL = __ENV.BOOKING_BASE_URL || "http://localhost:8082";
const EVENT_ID = parseInt(__ENV.EVENT_ID || "1", 10);
const QUOTA = parseInt(__ENV.QUOTA || "200000", 10);
const STEP_DURATION = __ENV.STEP_DURATION || "45s";

// Increasing test steps. The first level that fails threshold is near your limit.
const STEP_VUS = [50, 100, 200, 400, 800, 1200, 1600];

const bookingConfirmed = new Counter("booking_confirmed");
const bookingWaitlisted = new Counter("booking_waitlisted");
const bookingFailed = new Counter("booking_failed");
const errorRate = new Rate("error_rate");
const listEventsDuration = new Trend("list_events_duration_ms", true);
const bookingDuration = new Trend("booking_duration_ms", true);

function parseSeconds(duration) {
  const match = String(duration).trim().match(/^(\d+)(s)$/i);
  if (!match) {
    throw new Error("STEP_DURATION must be in seconds format, e.g. 30s or 60s");
  }
  return parseInt(match[1], 10);
}

function buildScenarios() {
  const scenarios = {};
  const stepSecs = parseSeconds(STEP_DURATION);

  for (let i = 0; i < STEP_VUS.length; i += 1) {
    const vus = STEP_VUS[i];
    const name = `step_${vus}`;
    scenarios[name] = {
      executor: "constant-vus",
      exec: "bookingFlow",
      vus,
      duration: STEP_DURATION,
      startTime: `${i * stepSecs}s`,
      gracefulStop: "5s",
    };
  }

  return scenarios;
}

function buildThresholds() {
  const thresholds = {
    // Overall guardrails for full run.
    error_rate: [{ threshold: "rate<0.10" }],
    http_req_failed: [{ threshold: "rate<0.10" }],
    booking_duration_ms: [{ threshold: "p(95)<2000" }],
  };

  // Per-step thresholds let you see exactly which concurrency level breaks first.
  for (const vus of STEP_VUS) {
    const scenarioTag = `{scenario:step_${vus}}`;
    thresholds[`error_rate${scenarioTag}`] = [{ threshold: "rate<0.05" }];
    thresholds[`http_req_failed${scenarioTag}`] = [{ threshold: "rate<0.05" }];
    thresholds[`booking_duration_ms${scenarioTag}`] = [{ threshold: "p(95)<1000" }];
    thresholds[`list_events_duration_ms${scenarioTag}`] = [{ threshold: "p(95)<500" }];
  }

  return thresholds;
}

export const options = {
  scenarios: buildScenarios(),
  thresholds: buildThresholds(),
  summaryTrendStats: ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"],
};

export function setup() {
  // Create event if endpoint accepts it; safe if it already exists.
  const eventRes = http.post(
    `${EVENT_BASE_URL}/api/v1/events`,
    JSON.stringify({
      name: "Max Concurrency Search Event",
      description: "Created by k6 setup",
      seat_limit: QUOTA,
      start_date: "2030-01-01T09:00:00Z",
      end_date: "2030-01-01T18:00:00Z",
    }),
    { headers: { "Content-Type": "application/json" } }
  );

  if (eventRes.status === 201) {
    try {
      const body = JSON.parse(eventRes.body);
      console.log(`[setup] event created id=${body.id}`);
    } catch (_) {
      console.log("[setup] event created");
    }
  }

  const quotaRes = http.post(
    `${BOOKING_BASE_URL}/api/v1/bookings/quota/init`,
    JSON.stringify({ event_id: EVENT_ID, quota: QUOTA }),
    { headers: { "Content-Type": "application/json" } }
  );

  if (quotaRes.status !== 200) {
    console.error(`[setup] quota init failed status=${quotaRes.status} body=${quotaRes.body}`);
  } else {
    console.log(`[setup] quota initialized event_id=${EVENT_ID} quota=${QUOTA}`);
  }
}

export function bookingFlow() {
  const uid = `vu${__VU}_iter${__ITER}`;

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
    return;
  }

  const payload = JSON.stringify({
    event_id: EVENT_ID,
    user_name: `User ${uid}`,
    user_email: `${uid}@max-concurrency.test`,
    user_phone: "0800000000",
  });

  const t1 = Date.now();
  const bookRes = http.post(`${BOOKING_BASE_URL}/api/v1/bookings`, payload, {
    headers: { "Content-Type": "application/json" },
    tags: { name: "BookSeat" },
  });
  bookingDuration.add(Date.now() - t1);

  check(bookRes, {
    "book seat 201 or 409": (r) => r.status === 201 || r.status === 409,
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
    bookingFailed.add(1);
    // This is a controlled business response under race/dup conditions.
    errorRate.add(0);
  } else {
    bookingFailed.add(1);
    errorRate.add(1);
  }

  sleep(0);
}

export function handleSummary(data) {
  const lines = [];
  lines.push("\n=== Max Concurrency Search Summary ===");
  lines.push(`Step duration: ${STEP_DURATION}`);
  lines.push(`Steps (VUs): ${STEP_VUS.join(", ")}`);
  lines.push("Interpretation: highest step whose scenario thresholds PASS is your current safe concurrent level.");
  lines.push("Inspect failed thresholds to see where errors/latency start to break guardrails.\n");

  return {
    stdout: `${lines.join("\n")}\n`,
    "k6/max_concurrency_summary.json": JSON.stringify(data, null, 2),
  };
}
