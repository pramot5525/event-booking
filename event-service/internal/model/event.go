package model

import (
	"time"

	"gorm.io/gorm"
)

type Event struct {
	ID          int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	SeatLimit   int32          `json:"seat_limit"`
	StartDate   *time.Time     `json:"start_date"`
	EndDate     *time.Time     `json:"end_date"`
	CreatedAt   time.Time      `json:"created_at" gorm:"autoCreateTime"`
	DeletedAt   gorm.DeletedAt `json:"deleted_at"`
}

func (Event) TableName() string {
	return "events"
}
