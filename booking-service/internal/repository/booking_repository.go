package repository

import (
	"booking-service/internal/model"
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var ErrDuplicateBooking = errors.New("duplicate booking")

type BookingRepository interface {
	CreateBooking(ctx context.Context, booking *model.Booking) error
	CountBookingsByEvent(ctx context.Context, eventID uint) (int64, error)
}

type bookingRepository struct {
	db *gorm.DB
}

func NewBookingRepository(db *gorm.DB) BookingRepository {
	return &bookingRepository{db: db}
}

func (r *bookingRepository) CreateBooking(ctx context.Context, booking *model.Booking) error {
	err := r.db.WithContext(ctx).Create(booking).Error
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateBooking
	}

	return err
}

func (r *bookingRepository) CountBookingsByEvent(ctx context.Context, eventID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Booking{}).Where("event_id = ?", eventID).Count(&count).Error
	return count, err
}
