package model

import (
	"time"

	"github.com/google/uuid"
)

type BookingStatus string
type WaitlistStatus string

const (
	BookingStatusConfirmed BookingStatus = "confirmed"
	BookingStatusCancelled BookingStatus = "cancelled"
	BookingStatusPending   BookingStatus = "waitlisted"
)

const (
	WaitlistStatusWaiting   WaitlistStatus = "waiting"
	WaitlistStatusPromoted  WaitlistStatus = "promoted"
	WaitlistStatusCancelled WaitlistStatus = "cancelled"
)

type Booking struct {
	ID         int64         `json:"id" gorm:"primaryKey;autoIncrement"`
	EventID    int64         `json:"event_id" gorm:"uniqueIndex:idx_event_user"`
	UserID     uuid.UUID     `json:"user_id"  gorm:"uniqueIndex:idx_event_user"`
	Status     BookingStatus `json:"status"`
	SeatNumber *int32        `json:"seat_number,omitempty"`
	CreatedAt  time.Time     `json:"created_at" gorm:"autoCreateTime"`
}

type WaitlistEntry struct {
	ID        int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	EventID   int64          `json:"event_id"`
	UserID    int64          `json:"user_id"`
	Status    WaitlistStatus `json:"status"`
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
}

type CreateBookingRequest struct {
	EventID   int64  `json:"event_id"   validate:"required"`
	UserName  string `json:"user_name"  validate:"required,min=1,max=255"`
	UserEmail string `json:"user_email" validate:"required,email"`
	UserPhone string `json:"user_phone" validate:"required"`
}
