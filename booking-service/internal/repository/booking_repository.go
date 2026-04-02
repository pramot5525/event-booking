package repository

import (
	"booking-service/internal/model"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrBookingNotFound  = errors.New("booking not found")
	ErrAlreadyCancelled = errors.New("booking is already cancelled")
)

type BookingRepository interface {
	CreateBooking(booking *model.Booking) error
	GetByID(id int64) (*model.Booking, error)
	GetBookingsByUID(uid uuid.UUID) ([]*model.Booking, error)
	GetBookingsByEventID(eventID int64) ([]*model.Booking, error)
	CountConfirmedByEventID(eventID int64) (int64, error)
	ExistsByEventAndUID(eventID int64, uid uuid.UUID) (bool, error)
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

func (r *bookingRepository) GetByID(id int64) (*model.Booking, error) {
	var booking model.Booking
	if err := r.db.First(&booking, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBookingNotFound
		}
		return nil, err
	}
	return &booking, nil
}

func (r *bookingRepository) GetBookingsByUID(uid uuid.UUID) ([]*model.Booking, error) {
	var bookings []*model.Booking
	if err := r.db.Where("uid = ?", uid).Find(&bookings).Error; err != nil {
		return nil, err
	}
	return bookings, nil
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

func (r *bookingRepository) ExistsByEventAndUID(eventID int64, uid uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&model.Booking{}).
		Where("event_id = ? AND uid = ? AND status = ?", eventID, uid, model.BookingStatusConfirmed).
		Count(&count).Error
	return count > 0, err
}
