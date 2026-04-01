package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const seatTTL = 24 * time.Hour

type CachedEvent struct {
	ID        int64 `json:"id"`
	SeatLimit int32 `json:"seat_limit"`
}

type SeatRepository interface {
	Init(ctx context.Context, eventID int64, available int32) error
	Remaining(ctx context.Context, eventID int64) (int64, error)
	Decrement(ctx context.Context, eventID int64) (int64, error)
	Increment(ctx context.Context, eventID int64) error
	SetEventCache(ctx context.Context, event *CachedEvent) error
	GetEventCache(ctx context.Context, eventID int64) (*CachedEvent, error)
	GetBooked(ctx context.Context, eventID int64) (int64, error)
	SetBooked(ctx context.Context, eventID int64, booked int64) error
	IncrementBooked(ctx context.Context, eventID int64) error
}

type seatRepository struct {
	rdb *redis.Client
}

func NewSeatRepository(rdb *redis.Client) SeatRepository {
	return &seatRepository{rdb: rdb}
}

func seatKey(eventID int64) string {
	return fmt.Sprintf("seats:%d", eventID)
}

func bookedKey(eventID int64) string {
	return fmt.Sprintf("booked:%d", eventID)
}

// Init sets the available seat counter only if it does not exist yet (SETNX).
func (r *seatRepository) Init(ctx context.Context, eventID int64, available int32) error {
	err := r.rdb.SetArgs(ctx, seatKey(eventID), available, redis.SetArgs{
		Mode: "NX",
		TTL:  seatTTL,
	}).Err()
	if err == redis.Nil {
		return nil // key already exists, that's fine
	}
	return err
}

func (r *seatRepository) Remaining(ctx context.Context, eventID int64) (int64, error) {
	val, err := r.rdb.Get(ctx, seatKey(eventID)).Result()
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(val, 10, 64)
}

func (r *seatRepository) Decrement(ctx context.Context, eventID int64) (int64, error) {
	return r.rdb.Decr(ctx, seatKey(eventID)).Result()
}

func (r *seatRepository) Increment(ctx context.Context, eventID int64) error {
	return r.rdb.Incr(ctx, seatKey(eventID)).Err()
}

func eventCacheKey(eventID int64) string {
	return fmt.Sprintf("event:%d:cache", eventID)
}

func (r *seatRepository) SetEventCache(ctx context.Context, event *CachedEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.rdb.Set(ctx, eventCacheKey(event.ID), data, 5*time.Minute).Err()
}

func (r *seatRepository) GetEventCache(ctx context.Context, eventID int64) (*CachedEvent, error) {
	val, err := r.rdb.Get(ctx, eventCacheKey(eventID)).Result()
	if err != nil {
		return nil, err
	}
	var event CachedEvent
	if err := json.Unmarshal([]byte(val), &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *seatRepository) GetBooked(ctx context.Context, eventID int64) (int64, error) {
	return r.rdb.Get(ctx, bookedKey(eventID)).Int64()
}

// SetBooked sets the booked counter only if it does not already exist (SETNX).
// This prevents concurrent requests from overwriting a counter already initialised
// by a racing goroutine.
func (r *seatRepository) SetBooked(ctx context.Context, eventID int64, booked int64) error {
	err := r.rdb.SetArgs(ctx, bookedKey(eventID), booked, redis.SetArgs{
		Mode: "NX",
		TTL:  seatTTL,
	}).Err()
	if err == redis.Nil {
		return nil // already set by a concurrent request, that's fine
	}
	return err
}

func (r *seatRepository) IncrementBooked(ctx context.Context, eventID int64) error {
	return r.rdb.Incr(ctx, bookedKey(eventID)).Err()
}
