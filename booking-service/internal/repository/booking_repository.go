package repository

import (
	"booking-service/internal/model"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrBookingNotFound  = errors.New("booking not found")
	ErrAlreadyCancelled = errors.New("booking is already cancelled")
)

type BookingRepository interface {
	CreateBooking(booking *model.Booking) error
	GetByID(id int64) (*model.Booking, error)
	GetBookingsByUserID(userID uuid.UUID) ([]*model.Booking, error)
	GetBookingsByEventID(eventID int64) ([]*model.Booking, error)
	CountConfirmedByEventID(eventID int64) (int64, error)
	ExistsByEventAndUser(eventID int64, userID uuid.UUID) (bool, error)
	// CancelAndMaybePromote atomically cancels the booking and, if a waiting
	// WaitlistEntry exists for the same event, promotes it to a new confirmed
	// Booking. Returns the promoted entry and the new Booking (both nil when
	// no one was waiting).
	CancelAndMaybePromote(bookingID int64) (*model.WaitlistEntry, *model.Booking, error)
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

func (r *bookingRepository) GetBookingsByUserID(userID uuid.UUID) ([]*model.Booking, error) {
	var bookings []*model.Booking
	if err := r.db.Where("user_id = ?", userID).Find(&bookings).Error; err != nil {
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

func (r *bookingRepository) ExistsByEventAndUser(eventID int64, userID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&model.Booking{}).
		Where("event_id = ? AND user_id = ? AND status = ?", eventID, userID, model.BookingStatusConfirmed).
		Count(&count).Error
	return count > 0, err
}

// CancelAndMaybePromote runs entirely inside a single database transaction:
//  1. Marks the booking as cancelled (fails if already cancelled).
//  2. Locks the oldest waiting WaitlistEntry for the same event (SELECT FOR UPDATE).
//  3. If found: creates a new confirmed Booking for that user and marks the
//     entry as promoted.
//
// Redis counter update (Increment) is the caller's responsibility when the
// returned WaitlistEntry is nil (no promotion occurred).
func (r *bookingRepository) CancelAndMaybePromote(bookingID int64) (*model.WaitlistEntry, *model.Booking, error) {
	var promoted *model.WaitlistEntry
	var newBooking *model.Booking

	err := r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Lock and cancel the booking.
		var cancelled model.Booking
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&cancelled, bookingID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBookingNotFound
			}
			return err
		}
		if cancelled.Status == model.BookingStatusCancelled {
			return ErrAlreadyCancelled
		}

		cancelled.Status = model.BookingStatusCancelled
		if err := tx.Save(&cancelled).Error; err != nil {
			return err
		}

		// 2. Find the oldest waiting entry for this event (lock to prevent
		//    concurrent promotions of the same row).
		var entry model.WaitlistEntry
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("event_id = ? AND status = ?", cancelled.EventID, model.WaitlistStatusWaiting).
			Order("created_at ASC").
			First(&entry).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // no one to promote — seat goes back to the pool
		}
		if err != nil {
			return err
		}

		// 3. Promote: create a new confirmed booking reusing the freed seat.
		newBooking = &model.Booking{
			EventID:    cancelled.EventID,
			UserID:     entry.UserID,
			Status:     model.BookingStatusConfirmed,
			SeatNumber: cancelled.SeatNumber,
		}
		if err := tx.Create(newBooking).Error; err != nil {
			return err
		}

		// 4. Mark the waitlist entry as promoted.
		entry.Status = model.WaitlistStatusPromoted
		if err := tx.Save(&entry).Error; err != nil {
			return err
		}

		promoted = &entry
		return nil
	})

	return promoted, newBooking, err
}
