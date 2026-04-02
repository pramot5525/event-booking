"use client";

import { useState } from "react";
import { FieldErrors, useForm } from "react-hook-form";

import type {
  BookEventResult,
  CreateBookingRequest,
  ErrorResponse,
} from "@/types/booking";

type BookingFormProps = {
  eventId: number;
};

const initialForm: Omit<CreateBookingRequest, "event_id"> = {
  user_name: "",
  user_email: "",
  user_phone: "",
};

export default function BookingForm({ eventId }: BookingFormProps) {
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<BookEventResult | null>(null);
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<Omit<CreateBookingRequest, "event_id">>({
    defaultValues: initialForm,
  });

  async function onSubmit(form: Omit<CreateBookingRequest, "event_id">) {
    setError(null);
    setResult(null);

    try {
      const response = await fetch("/api/bookings", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          event_id: eventId,
          ...form,
        }),
      });

      const contentType = response.headers.get("content-type") ?? "";
      const data = contentType.includes("application/json")
        ? ((await response.json()) as BookEventResult | ErrorResponse)
        : ({ error: await response.text() } as ErrorResponse);

      if (!response.ok) {
        setError("error" in data ? data.error : "booking failed");
        return;
      }

      setResult(data as BookEventResult);
      reset(initialForm);
    } catch {
      setError("unable to submit booking right now");
    }
  }

  function onInvalid(errors: FieldErrors<Omit<CreateBookingRequest, "event_id">>) {
    const firstMessage =
      errors.user_name?.message ??
      errors.user_email?.message ??
      errors.user_phone?.message ??
      "please check the form fields";
    setError(String(firstMessage));
  }

  return (
    <section className="rounded-3xl border border-zinc-200 bg-white p-6 shadow-sm">
      <div className="mb-6">
        <h2 className="text-2xl font-semibold text-zinc-900">Book this event</h2>
        <p className="mt-2 text-sm text-zinc-500">
          Fill in your contact details. If seats are full, you will be added to
          the waitlist automatically.
        </p>
      </div>

      <form
        className="space-y-4"
        noValidate
        onSubmit={handleSubmit(onSubmit, onInvalid)}
      >
        <label className="block">
          <span className="mb-2 block text-sm font-medium text-zinc-700">Name</span>
          <input
            {...register("user_name", {
              required: "name is required",
            })}
            className="w-full rounded-2xl border border-zinc-300 px-4 py-3 text-zinc-900 outline-none transition focus:border-zinc-500"
            placeholder="Somchai Jaidee"
          />
          {errors.user_name && (
            <p className="mt-2 text-sm text-red-600">{errors.user_name.message}</p>
          )}
        </label>

        <label className="block">
          <span className="mb-2 block text-sm font-medium text-zinc-700">Email</span>
          <input
            type="email"
            {...register("user_email", {
              required: "email is required",
              pattern: {
                value: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
                message: "invalid email address",
              },
            })}
            className="w-full rounded-2xl border border-zinc-300 px-4 py-3 text-zinc-900 outline-none transition focus:border-zinc-500"
            placeholder="somchai@example.com"
          />
          {errors.user_email && (
            <p className="mt-2 text-sm text-red-600">{errors.user_email.message}</p>
          )}
        </label>

        <label className="block">
          <span className="mb-2 block text-sm font-medium text-zinc-700">Phone</span>
          <input
            {...register("user_phone", {
              required: "phone is required",
            })}
            className="w-full rounded-2xl border border-zinc-300 px-4 py-3 text-zinc-900 outline-none transition focus:border-zinc-500"
            placeholder="0812345678"
          />
          {errors.user_phone && (
            <p className="mt-2 text-sm text-red-600">{errors.user_phone.message}</p>
          )}
        </label>

        <button
          type="submit"
          disabled={isSubmitting}
          className="w-full rounded-2xl bg-zinc-950 px-4 py-3 text-sm font-semibold text-white transition hover:bg-zinc-800 disabled:cursor-not-allowed disabled:bg-zinc-400"
        >
          {isSubmitting ? "Submitting..." : "Confirm booking"}
        </button>
      </form>

      {error && (
        <p className="mt-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
        </p>
      )}

      {result?.status === "confirmed" && result.booking && (
        <div className="mt-4 rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-4 text-sm text-emerald-800">
          Booking confirmed. Your seat number is {result.booking.seat_number}.
        </div>
      )}

      {result?.status === "waitlisted" && result.waitlist_entry && (
        <div className="mt-4 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-4 text-sm text-amber-800">
          Seats are full. You have been added to the waitlist at position {result.waitlist_entry.position}.
        </div>
      )}
    </section>
  );
}