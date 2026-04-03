package model

import "time"

type Booking struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	EventID   uint      `json:"event_id" gorm:"not null;index:idx_event_user,unique;index:idx_event_status"`
	UID       string    `json:"uid" gorm:"type:varchar(100);not null;index:idx_event_user,unique"`
	UserName  string    `json:"user_name" gorm:"not null"`
	UserEmail string    `json:"user_email" gorm:"not null"`
	UserPhone string    `json:"user_phone" gorm:"not null"`
	Status    string    `json:"status" gorm:"type:varchar(20);not null;default:confirmed;index:idx_event_status"`
	Position  *int64    `json:"position,omitempty" gorm:"column:waitlist_position"`
	CreatedAt time.Time `json:"created_at"`
}

type EventQuota struct {
	EventID     uint  `gorm:"primaryKey"`
	SeatsTotal  int64 `gorm:"not null"`
	SeatsBooked int64 `gorm:"not null;default:0"`
}
