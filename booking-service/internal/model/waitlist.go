package model

import (
	"time"

	"github.com/google/uuid"
)

type WaitlistStatus string

const (
	WaitlistStatusWaiting   WaitlistStatus = "waiting"
	WaitlistStatusPromoted  WaitlistStatus = "promoted"
	WaitlistStatusCancelled WaitlistStatus = "cancelled"
)

// WaitlistEntry records users who attempted to book a full event.
// When a confirmed booking is cancelled the oldest waiting entry is promoted
// to a confirmed Booking atomically within a single database transaction.
type WaitlistEntry struct {
	ID        int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	EventID   int64          `json:"event_id" gorm:"uniqueIndex:idx_waitlist_event_user;index"`
	UID       uuid.UUID      `json:"uid" gorm:"uniqueIndex:idx_waitlist_event_user"`
	UserName  string         `json:"user_name"`
	UserEmail string         `json:"user_email"`
	UserPhone string         `json:"user_phone"`
	Position  int32          `json:"position"` // ordinal rank at insertion time
	Status    WaitlistStatus `json:"status" gorm:"default:'waiting'"`
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
}
