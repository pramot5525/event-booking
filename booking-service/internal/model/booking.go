package model

import (
	"time"

	"github.com/google/uuid"
)

type BookingStatus string

const (
	BookingStatusConfirmed  BookingStatus = "confirmed"
	BookingStatusWaitlisted BookingStatus = "waitlisted"
	BookingStatusCancelled  BookingStatus = "cancelled"
)

type Booking struct {
	ID         int64         `json:"id" gorm:"primaryKey;autoIncrement"`
	EventID    int64         `json:"event_id" gorm:"uniqueIndex:idx_event_user"`
	UserID     uuid.UUID     `json:"user_id"  gorm:"uniqueIndex:idx_event_user"`
	Status     BookingStatus `json:"status"`
	SeatNumber *int32        `json:"seat_number,omitempty"`
	CreatedAt  time.Time     `json:"created_at" gorm:"autoCreateTime"`
}

type CreateBookingRequest struct {
	EventID   int64  `json:"event_id"`
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
	UserPhone string `json:"user_phone"`
}
