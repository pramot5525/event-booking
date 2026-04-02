import Link from "next/link";

import { Event } from "@/types/event";

function formatEventDateRange(startDate?: string, endDate?: string): string | null {
  if (!startDate && !endDate) {
    return null;
  }

  const formatter = new Intl.DateTimeFormat("th-TH", { dateStyle: "long" });

  if (startDate && endDate) {
    return `${formatter.format(new Date(startDate))} - ${formatter.format(new Date(endDate))}`;
  }

  return formatter.format(new Date(startDate ?? endDate!));
}

async function getEvents(): Promise<Event[]> {
  const res = await fetch(
    `${process.env.EVENT_SERVICE_URL ?? "http://localhost:8081"}/api/v1/events`,
    { cache: "no-store" }
  );
  if (!res.ok) throw new Error("Failed to fetch events");
  return res.json();
}

export default async function EventsPage() {
  const events = await getEvents();

  return (
    <main className="max-w-4xl mx-auto py-10 px-4">
      <h1 className="text-3xl font-bold mb-8">Events</h1>
      {events.length === 0 ? (
        <p className="text-zinc-500">No events available.</p>
      ) : (
        <div className="grid gap-4">
          {events.map((event) => {
            const dateRange = formatEventDateRange(
              event.start_date,
              event.end_date
            );

            return (
              <div
                key={event.id}
                className="border border-zinc-200 rounded-xl p-6 bg-white shadow-sm"
              >
                <div className="flex justify-between items-start gap-4">
                  <h2 className="text-xl font-semibold">{event.name}</h2>
                  <span className="text-sm text-zinc-500 bg-zinc-100 px-2 py-1 rounded-full">
                    {event.seat_limit} seats
                  </span>
                </div>
                {event.description && (
                  <p className="mt-2 text-zinc-600">{event.description}</p>
                )}
                {dateRange && (
                  <p className="mt-4 inline-flex items-center rounded-full bg-zinc-50 px-3 py-1 text-sm text-zinc-500">
                    {dateRange}
                  </p>
                )}
                <div className="mt-5">
                  <Link
                    href={`/events/${event.id}`}
                    className="inline-flex rounded-full bg-zinc-950 px-4 py-2 text-sm font-medium text-white transition hover:bg-zinc-800"
                  >
                    Book event
                  </Link>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </main>
  );
}
