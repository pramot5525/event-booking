export type CreateBookingRequest = {
  event_id: number;
  user_name: string;
  user_email: string;
  user_phone: string;
};

export type Booking = {
  id: number;
  event_id: number;
  uid: string;
  status: string;
  seat_number?: number;
  created_at?: string;
};

export type WaitlistEntry = {
  id: number;
  event_id: number;
  uid: string;
  position: number;
  status: string;
  created_at?: string;
};

export type BookEventResult = {
  status: "confirmed" | "waitlisted";
  booking?: Booking;
  waitlist_entry?: WaitlistEntry;
};

export type ErrorResponse = {
  error: string;
};