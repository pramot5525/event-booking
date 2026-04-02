import Link from "next/link";
import { notFound } from "next/navigation";

import BookingForm from "./booking-form";

import type { Event } from "@/types/event";

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

async function getEvent(id: string): Promise<Event> {
  const response = await fetch(
    `${process.env.EVENT_SERVICE_URL ?? "http://localhost:8081"}/api/v1/events/${id}`,
    { cache: "no-store" }
  );

  if (response.status === 404) {
    notFound();
  }

  if (!response.ok) {
    throw new Error("Failed to fetch event");
  }

  return response.json();
}

export default async function EventBookingPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const event = await getEvent(id);
  const dateRange = formatEventDateRange(event.start_date, event.end_date);

  return (
    <main className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-8 px-4 py-10 lg:flex-row lg:items-start">
      <section className="flex-1 rounded-[2rem] border border-zinc-200 bg-zinc-50 p-8">
        <Link
          href="/"
          className="text-sm font-medium text-zinc-500 transition hover:text-zinc-900"
        >
          Back to events
        </Link>

        <div className="mt-6 space-y-4">
          <span className="inline-flex rounded-full bg-white px-3 py-1 text-sm text-zinc-500 ring-1 ring-zinc-200">
            Event #{event.id}
          </span>
          <h1 className="text-4xl font-semibold tracking-tight text-zinc-950">
            {event.name}
          </h1>
          {event.description && (
            <p className="max-w-2xl text-base leading-7 text-zinc-600">
              {event.description}
            </p>
          )}
        </div>

        <dl className="mt-8 grid gap-4 sm:grid-cols-2">
          <div className="rounded-3xl bg-white p-5 shadow-sm ring-1 ring-zinc-200">
            <dt className="text-sm text-zinc-500">Seat capacity</dt>
            <dd className="mt-2 text-2xl font-semibold text-zinc-900">
              {event.seat_limit}
            </dd>
          </div>
          <div className="rounded-3xl bg-white p-5 shadow-sm ring-1 ring-zinc-200">
            <dt className="text-sm text-zinc-500">Schedule</dt>
            <dd className="mt-2 text-base font-medium text-zinc-900">
              {dateRange ?? "Schedule not announced"}
            </dd>
          </div>
        </dl>
      </section>

      <div className="w-full lg:max-w-md">
        <BookingForm eventId={event.id} />
      </div>
    </main>
  );
}