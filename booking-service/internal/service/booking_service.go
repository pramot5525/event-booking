package service

import (
	"booking-service/config"
	"booking-service/internal/model"
	"booking-service/internal/repository"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalidRequest      = errors.New("invalid request")
	ErrBookingInProgress   = errors.New("booking already in progress")
	ErrSoldOut             = errors.New("seats sold out")
	ErrAlreadyBooked       = errors.New("user already booked")
	ErrQuotaNotInitialized = errors.New("quota not initialized")
)

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
	Status    string         `json:"status"`
	Booking   *model.Booking `json:"booking,omitempty"`
	Remaining int64          `json:"remaining_seats"`
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
	if req.EventID == 0 || strings.TrimSpace(req.UserEmail) == "" || strings.TrimSpace(req.UserName) == "" {
		return nil, ErrInvalidRequest
	}

	userID := stableUserID(req.UserEmail)
	lockKey := lockRedisKey(req.EventID, userID)

	ok, err := s.rdb.SetNX(ctx, lockKey, "1", s.lockTTL).Result()
	if err != nil {
		return nil, fmt.Errorf("create idempotency lock: %w", err)
	}
	if !ok {
		return nil, ErrBookingInProgress
	}

	quotaKey := quotaKey(req.EventID)
	if err := s.ensureQuota(ctx, req.EventID, quotaKey); err != nil {
		_ = s.rdb.Del(ctx, lockKey).Err()
		return nil, err
	}

	remaining, err := s.rdb.Decr(ctx, quotaKey).Result()
	if err != nil {
		_ = s.rdb.Del(ctx, lockKey).Err()
		return nil, fmt.Errorf("decrement quota: %w", err)
	}

	if remaining < 0 {
		_, _ = s.rdb.Incr(ctx, quotaKey).Result()
		_ = s.rdb.Del(ctx, lockKey).Err()
		return nil, ErrSoldOut
	}

	booking := &model.Booking{
		EventID:   req.EventID,
		UID:       userID,
		UserName:  req.UserName,
		UserEmail: req.UserEmail,
		UserPhone: req.UserPhone,
		Status:    "confirmed",
	}

	if err := s.repo.CreateBooking(ctx, booking); err != nil {
		_, _ = s.rdb.Incr(ctx, quotaKey).Result()
		_ = s.rdb.Del(ctx, lockKey).Err()

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

func (s *bookingService) ensureQuota(ctx context.Context, eventID uint, key string) error {
	exists, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("check quota exists: %w", err)
	}
	if exists == 1 {
		return nil
	}

	seatLimit, err := s.eventClient.GetEventSeatLimit(ctx, eventID)
	if err != nil {
		return fmt.Errorf("get event seat_limit: %w", err)
	}

	booked, err := s.repo.CountBookingsByEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("count current bookings: %w", err)
	}

	remaining := seatLimit - booked
	if remaining < 0 {
		remaining = 0
	}

	if err := s.rdb.SetNX(ctx, key, remaining, s.quotaSeedTTL).Err(); err != nil {
		return fmt.Errorf("seed redis quota: %w", err)
	}

	return nil
}

func quotaKey(eventID uint) string {
	return fmt.Sprintf("event:%d:quota", eventID)
}

func lockRedisKey(eventID uint, userID string) string {
	return fmt.Sprintf("lock:%d:%s", eventID, userID)
}

func stableUserID(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(normalized)).String()
}
