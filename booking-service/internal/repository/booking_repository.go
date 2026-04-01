package repository

import (
	"booking-service/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BookingRepository interface {
	CreateBooking(model *model.Booking) error
	GetBookingByUserID(userID uuid.UUID) (*model.Booking, error)
	GetBookingsByEventID(eventID int64) ([]*model.Booking, error)
	CountConfirmedByEventID(eventID int64) (int64, error)
	ExistsByEventAndUser(eventID int64, userID uuid.UUID) (bool, error)
}

type bookingRepository struct {
	db *gorm.DB
}

func NewBookingRepository(db *gorm.DB) BookingRepository {
	return &bookingRepository{db: db}
}

func (r *bookingRepository) CreateBooking(booking *model.Booking) error {
	return r.db.Create(booking).Error
}

func (r *bookingRepository) GetBookingByUserID(userID uuid.UUID) (*model.Booking, error) {
	var booking model.Booking
	if err := r.db.Where("user_id = ?", userID).First(&booking).Error; err != nil {
		return nil, err
	}
	return &booking, nil
}

func (r *bookingRepository) GetBookingsByEventID(eventID int64) ([]*model.Booking, error) {
	var bookings []*model.Booking
	if err := r.db.Where("event_id = ?", eventID).Find(&bookings).Error; err != nil {
		return nil, err
	}
	return bookings, nil
}

func (r *bookingRepository) CountConfirmedByEventID(eventID int64) (int64, error) {
	var count int64
	err := r.db.Model(&model.Booking{}).
		Where("event_id = ? AND status = ?", eventID, model.BookingStatusConfirmed).
		Count(&count).Error
	return count, err
}

func (r *bookingRepository) ExistsByEventAndUser(eventID int64, userID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&model.Booking{}).
		Where("event_id = ? AND user_id = ?", eventID, userID).
		Count(&count).Error
	return count > 0, err
}
