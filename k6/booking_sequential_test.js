import http from "k6/http";
import { check } from "k6";

const EVENT_SERVICE_URL = __ENV.EVENT_SERVICE_URL || "http://localhost:8081";
const BOOKING_SERVICE_URL = __ENV.BOOKING_SERVICE_URL || "http://localhost:8082";
const SEAT_LIMIT = parseInt(__ENV.SEAT_LIMIT || "100");

export const options = {
  scenarios: {
    sequential_booking: {
      executor: "per-vu-iterations",
      vus: 1,
      iterations: SEAT_LIMIT + 1, // 101 iterations: 100 should succeed, 1 fail
      maxDuration: "60s",
    },
  },
};

let eventId;

export function setup() {
  const payload = JSON.stringify({
    name: "Sequential Test Event",
    description: "k6 sequential test",
    seat_limit: SEAT_LIMIT,
    start_date: "2026-06-01T09:00:00Z",
    end_date: "2026-06-01T18:00:00Z",
  });

  const res = http.post(`${EVENT_SERVICE_URL}/api/v1/events`, payload, {
    headers: { "Content-Type": "application/json" },
  });

  check(res, { "event created": (r) => r.status === 200 });

  const body = JSON.parse(res.body);
  eventId = body.id;
  console.log(`Event created with id=${eventId}, seat_limit=${SEAT_LIMIT}`);
  return { eventId };
}

export default function (data) {
  const vuId = __VU * 1000 + __ITER;

  const payload = JSON.stringify({
    event_id: data.eventId,
    user_name: `User ${vuId}`,
    user_email: `user_${vuId}@seqtest.com`,
    user_phone: `08${String(vuId).padStart(8, "0")}`,
  });

  const res = http.post(`${BOOKING_SERVICE_URL}/api/v1/bookings`, payload, {
    headers: { "Content-Type": "application/json" },
  });

  if (__ITER <= SEAT_LIMIT) {
    // First 100 should succeed
    check(res, {
      [`iteration ${__ITER} status 201`]: (r) => r.status === 201,
      [`iteration ${__ITER} confirmed`]: (r) => r.status === 201 && JSON.parse(r.body).status === "confirmed",
    });
    if (res.status === 201) {
      console.log(`VU ${__VU} iteration ${__ITER}: Booked seat ${JSON.parse(res.body).seat_number}`);
    }
  } else {
    // 101st should fail
    check(res, {
      [`iteration ${__ITER} status 500`]: (r) => r.status === 500,
      [`iteration ${__ITER} no seats`]: (r) => r.status === 500 && JSON.parse(r.body).error === "no seats available",
    });
    if (res.status === 500) {
      console.log(`VU ${__VU} iteration ${__ITER}: Correctly failed - ${JSON.parse(res.body).error}`);
    }
  }
}

export function teardown(data) {
  console.log(`Sequential test completed for event ${data.eventId}`);
}