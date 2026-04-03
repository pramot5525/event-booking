package repository

import (
	"booking-service/internal/model"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrDuplicateBooking = errors.New("duplicate booking")

// luaReserve atomically initializes the counter (if absent) and decrements it.
// Returns 1 if a seat was reserved, 0 if no seats are available.
var luaReserve = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
    redis.call('SET', KEYS[1], ARGV[1])
end
local val = redis.call('DECR', KEYS[1])
if val < 0 then
    redis.call('INCR', KEYS[1])
    return 0
end
return 1
`)

type BookingRepository interface {
	CreateBooking(ctx context.Context, booking *model.Booking) error
	GetMaxWaitlistPosition(ctx context.Context, eventID uint) (int64, error)
	CountBookingsByEvent(ctx context.Context, eventID uint) (int64, error)
	UpsertQuota(ctx context.Context, tx *gorm.DB, eventID uint, seatsTotal int64) error
	GetQuota(ctx context.Context, eventID uint) (*model.EventQuota, error)
	TryReserveQuota(ctx context.Context, eventID uint, available int64) (bool, error)
	ReleaseQuotaReservation(ctx context.Context, eventID uint) error
	IncrementSeatsBooked(ctx context.Context, tx *gorm.DB, eventID uint) error
}

type bookingRepository struct {
	db  *gorm.DB
	rdb *redis.Client
}

func NewBookingRepository(db *gorm.DB, rdb *redis.Client) BookingRepository {
	return &bookingRepository{db: db, rdb: rdb}
}

func quotaKey(eventID uint) string {
	return fmt.Sprintf("quota:seats_remaining:%d", eventID)
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
	err := r.db.WithContext(ctx).Model(&model.Booking{}).Where("event_id = ? AND status = ?", eventID, "confirmed").Count(&count).Error
	return count, err
}

func (r *bookingRepository) GetMaxWaitlistPosition(ctx context.Context, eventID uint) (int64, error) {
	var maxPos int64
	err := r.db.WithContext(ctx).
		Model(&model.Booking{}).
		Where("event_id = ? AND status = ?", eventID, "waitlisted").
		Select("COALESCE(MAX(waitlist_position), 0)").
		Scan(&maxPos).Error
	if err != nil {
		return 0, err
	}
	return maxPos, nil
}

func (r *bookingRepository) UpsertQuota(ctx context.Context, tx *gorm.DB, eventID uint, seatsTotal int64) error {
	quota := model.EventQuota{EventID: eventID, SeatsTotal: seatsTotal, SeatsBooked: 0}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&quota).Error
}

func (r *bookingRepository) GetQuota(ctx context.Context, eventID uint) (*model.EventQuota, error) {
	var quota model.EventQuota
	err := r.db.WithContext(ctx).Where("event_id = ?", eventID).First(&quota).Error
	if err != nil {
		return nil, err
	}
	return &quota, nil
}

// TryReserveQuota atomically tries to reserve one seat using a Redis Lua script.
// available is used as the seed only when the Redis key does not yet exist.
// Returns true if a seat was reserved, false if the event is sold out.
func (r *bookingRepository) TryReserveQuota(ctx context.Context, eventID uint, available int64) (bool, error) {
	key := quotaKey(eventID)
	result, err := luaReserve.Run(ctx, r.rdb, []string{key}, available).Int()
	if err != nil {
		return false, fmt.Errorf("redis reserve quota: %w", err)
	}
	return result == 1, nil
}

// ReleaseQuotaReservation increments the Redis counter back, used to roll back
// a TryReserveQuota call when the subsequent DB transaction fails.
func (r *bookingRepository) ReleaseQuotaReservation(ctx context.Context, eventID uint) error {
	return r.rdb.Incr(ctx, quotaKey(eventID)).Err()
}

func (r *bookingRepository) IncrementSeatsBooked(ctx context.Context, tx *gorm.DB, eventID uint) error {
	return tx.WithContext(ctx).Model(&model.EventQuota{}).
		Where("event_id = ?", eventID).
		UpdateColumn("seats_booked", gorm.Expr("seats_booked + 1")).Error
}
