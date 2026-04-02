package service

import (
	"booking-service/config"
	"booking-service/internal/model"
	"booking-service/internal/repository"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalidRequest      = errors.New("invalid request")
	ErrBookingInProgress   = errors.New("booking already in progress")
	ErrAlreadyBooked       = errors.New("user already booked")
	ErrQuotaNotInitialized = errors.New("quota not initialized")
)

// luaDecrQuota atomically checks quota and decrements it in a single round trip.
// Returns remaining seats (>= 0) on success, -1 if quota is already 0 (no seats).
// Returns a Redis error "QUOTA_NOT_INITIALIZED" if the key does not exist.
var luaDecrQuota = redis.NewScript(`
local val = redis.call('GET', KEYS[1])
if val == false then
    return redis.error_reply('QUOTA_NOT_INITIALIZED')
end
local n = tonumber(val)
if n <= 0 then
    return -1
end
return redis.call('DECR', KEYS[1])
`)

// luaIncrWaitlistSeq seeds the waitlist sequence key if absent, then increments it.
// Combining SETNX + EXPIRE + INCR in one script prevents duplicate positions when
// the TTL expires under concurrent load.
// KEYS[1] = sequence key, ARGV[1] = seed value, ARGV[2] = TTL in seconds.
var luaIncrWaitlistSeq = redis.NewScript(`
local set = redis.call('SETNX', KEYS[1], ARGV[1])
if set == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return redis.call('INCR', KEYS[1])
`)

type BookingService interface {
	BookSeat(ctx context.Context, req BookSeatRequest) (*BookSeatResult, error)
	InitializeQuota(ctx context.Context, eventID uint, quota int64) error
}

type BookSeatRequest struct {
	EventID   uint   `json:"event_id"`
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
	UserPhone string `json:"user_phone"`
}

type BookSeatResult struct {
	Status        string         `json:"status"`
	Booking       *model.Booking `json:"booking,omitempty"`
	WaitlistEntry *model.Booking `json:"waitlist_entry,omitempty"`
	Remaining     int64          `json:"remaining_seats"`
}

type bookingService struct {
	repo         repository.BookingRepository
	rdb          *redis.Client
	eventClient  EventClient
	lockTTL      time.Duration
	quotaSeedTTL time.Duration
}

func NewBookingService(repo repository.BookingRepository, rdb *redis.Client, eventClient EventClient, cfg *config.Config) BookingService {
	return &bookingService{
		repo:         repo,
		rdb:          rdb,
		eventClient:  eventClient,
		lockTTL:      cfg.BookingLockTTL,
		quotaSeedTTL: cfg.RedisQuotaTTL,
	}
}

func (s *bookingService) InitializeQuota(ctx context.Context, eventID uint, quota int64) error {
	if eventID == 0 || quota < 0 {
		return ErrInvalidRequest
	}
	return s.rdb.Set(ctx, quotaKey(eventID), quota, s.quotaSeedTTL).Err()
}

func (s *bookingService) BookSeat(ctx context.Context, req BookSeatRequest) (*BookSeatResult, error) {
	// Validate required fields before doing any external calls.
	if req.EventID == 0 || strings.TrimSpace(req.UserEmail) == "" || strings.TrimSpace(req.UserName) == "" {
		return nil, ErrInvalidRequest
	}

	// Build a deterministic user ID from email for idempotency and uniqueness checks.
	userID := stableUserID(req.UserEmail)
	lockKey := lockRedisKey(req.EventID, userID)

	// Acquire short-lived lock to prevent duplicate concurrent bookings per user/event.
	ok, err := s.rdb.SetNX(ctx, lockKey, "1", s.lockTTL).Result()
	if err != nil {
		return nil, fmt.Errorf("create idempotency lock: %w", err)
	}
	if !ok {
		return nil, ErrBookingInProgress
	}
	defer s.rdb.Del(ctx, lockKey) // release lock on all exit paths

	// Atomically consume one seat from Redis quota.
	qKey := quotaKey(req.EventID)
	remaining, err := luaDecrQuota.Run(ctx, s.rdb, []string{qKey}).Int64()
	if err != nil {
		if err.Error() == "QUOTA_NOT_INITIALIZED" {
			return nil, ErrQuotaNotInitialized
		}
		return nil, fmt.Errorf("decrement quota: %w", err)
	}

	// No seats left: assign waitlist position instead of confirming a booking.
	if remaining < 0 {
		waitlistEntry, err := s.createWaitlistEntry(ctx, req, userID)
		if err != nil {
			if errors.Is(err, repository.ErrDuplicateBooking) {
				return nil, ErrAlreadyBooked
			}
			return nil, err
		}
		return &BookSeatResult{
			Status:        "waitlisted",
			WaitlistEntry: waitlistEntry,
			Remaining:     0,
		}, nil
	}

	// Seats are available: prepare a confirmed booking record.
	booking := &model.Booking{
		EventID:   req.EventID,
		UID:       userID,
		UserName:  req.UserName,
		UserEmail: req.UserEmail,
		UserPhone: req.UserPhone,
		Status:    "confirmed",
	}

	// Persist booking in DB; if it fails, restore the decremented seat in Redis.
	if err := s.repo.CreateBooking(ctx, booking); err != nil {
		if rollbackErr := s.rdb.Incr(ctx, qKey).Err(); rollbackErr != nil {
			log.Printf("CRITICAL: quota rollback failed for event %d user %s: %v", req.EventID, userID, rollbackErr)
		}
		if errors.Is(err, repository.ErrDuplicateBooking) {
			return nil, ErrAlreadyBooked
		}
		return nil, fmt.Errorf("persist booking: %w", err)
	}

	return &BookSeatResult{
		Status:    "confirmed",
		Booking:   booking,
		Remaining: remaining,
	}, nil
}

func (s *bookingService) createWaitlistEntry(ctx context.Context, req BookSeatRequest, userID string) (*model.Booking, error) {
	seqKey := waitlistSeqKey(req.EventID)

	// Only query DB to seed the sequence if the Redis key doesn't exist.
	// Once seeded, luaIncrWaitlistSeq just INCRs — the DB value is unused.
	var maxPos int64
	exists, err := s.rdb.Exists(ctx, seqKey).Result()
	if err != nil {
		return nil, fmt.Errorf("check waitlist seq key: %w", err)
	}
	if exists == 0 {
		maxPos, err = s.repo.GetMaxWaitlistPosition(ctx, req.EventID)
		if err != nil {
			return nil, fmt.Errorf("get max waitlist position: %w", err)
		}
	}

	ttlSecs := int64(s.quotaSeedTTL.Seconds())
	position, err := luaIncrWaitlistSeq.Run(ctx, s.rdb, []string{seqKey}, maxPos, ttlSecs).Int64()
	if err != nil {
		return nil, fmt.Errorf("increment waitlist sequence: %w", err)
	}

	waitlist := &model.Booking{
		EventID:   req.EventID,
		UID:       userID,
		UserName:  req.UserName,
		UserEmail: req.UserEmail,
		UserPhone: req.UserPhone,
		Status:    "waitlisted",
		Position:  &position,
	}

	if err := s.repo.CreateBooking(ctx, waitlist); err != nil {
		return nil, fmt.Errorf("persist waitlist entry: %w", err)
	}

	return waitlist, nil
}

func quotaKey(eventID uint) string {
	return fmt.Sprintf("event:%d:quota", eventID)
}

func lockRedisKey(eventID uint, userID string) string {
	return fmt.Sprintf("lock:%d:%s", eventID, userID)
}

func waitlistSeqKey(eventID uint) string {
	return fmt.Sprintf("event:%d:waitlist:seq", eventID)
}

func stableUserID(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(normalized)).String()
}
