package model

import "time"

type Booking struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	EventID   uint      `json:"event_id" gorm:"not null;index:idx_event_user,unique"`
	UID       string    `json:"uid" gorm:"type:varchar(100);not null;index:idx_event_user,unique"`
	UserName  string    `json:"user_name" gorm:"not null"`
	UserEmail string    `json:"user_email" gorm:"not null"`
	UserPhone string    `json:"user_phone" gorm:"not null"`
	Status    string    `json:"status" gorm:"type:varchar(20);not null;default:confirmed"`
	CreatedAt time.Time `json:"created_at"`
}
