import { NextResponse } from "next/server";

const bookingServiceUrl =
  process.env.BOOKING_SERVICE_URL ?? "http://localhost:8082";

export async function POST(request: Request) {
  try {
    const payload = await request.json();
    const headers: HeadersInit = {
      "Content-Type": "application/json",
    };


    const response = await fetch(`${bookingServiceUrl}/api/v1/bookings`, {
      method: "POST",
      headers,
      body: JSON.stringify(payload),
      cache: "no-store",
    });

    const contentType = response.headers.get("content-type") ?? "";

    if (contentType.includes("application/json")) {
      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    }

    const text = await response.text();
    return new NextResponse(text, {
      status: response.status,
      headers: {
        "Content-Type": contentType || "text/plain",
      },
    });
  } catch {
    return NextResponse.json(
      { error: "unable to reach booking service" },
      { status: 500 }
    );
  }
}