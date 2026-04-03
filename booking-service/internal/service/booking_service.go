package service

import (
	"booking-service/internal/model"
	"booking-service/internal/repository"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidRequest = errors.New("invalid request")
	ErrAlreadyBooked  = errors.New("user already booked")
)

type BookingService interface {
	BookSeat(ctx context.Context, req BookSeatRequest) (*BookSeatResult, error)
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
	db          *gorm.DB
	repo        repository.BookingRepository
	eventClient EventClient
}

func NewBookingService(repo repository.BookingRepository, db *gorm.DB, eventClient EventClient) BookingService {
	return &bookingService{
		db:          db,
		repo:        repo,
		eventClient: eventClient,
	}
}

func (s *bookingService) BookSeat(ctx context.Context, req BookSeatRequest) (*BookSeatResult, error) {
	if req.EventID == 0 || strings.TrimSpace(req.UserEmail) == "" || strings.TrimSpace(req.UserName) == "" {
		return nil, ErrInvalidRequest
	}

	userID := stableUserID(req.UserEmail)

	// Fetch seat limit and upsert quota row (idempotent — ON CONFLICT DO NOTHING).
	seatLimit, err := s.eventClient.GetEventSeatLimit(ctx, req.EventID)
	if err != nil {
		return nil, fmt.Errorf("get event seat limit: %w", err)
	}
	if err := s.repo.UpsertQuota(ctx, s.db, req.EventID, int64(seatLimit)); err != nil {
		return nil, fmt.Errorf("upsert quota: %w", err)
	}

	// Plain SELECT (no lock) to get current remaining — used as Redis seed.
	quota, err := s.repo.GetQuota(ctx, req.EventID)
	if err != nil {
		return nil, fmt.Errorf("get quota: %w", err)
	}
	available := quota.SeatsTotal - quota.SeatsBooked

	// Atomically reserve a seat in Redis (init-if-absent + DECR via Lua).
	reserved, err := s.repo.TryReserveQuota(ctx, req.EventID, available)
	if err != nil {
		return nil, fmt.Errorf("try reserve quota: %w", err)
	}

	if reserved {
		return s.confirmBooking(ctx, req, userID, available)
	}
	return s.waitlistBooking(ctx, req, userID)
}

func (s *bookingService) confirmBooking(ctx context.Context, req BookSeatRequest, userID string, available int64) (*BookSeatResult, error) {
	var booking model.Booking

	// Only INSERT the booking row in the transaction — no hot-row UPDATE.
	// seats_booked is reconciled separately; Redis is the authoritative counter.
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		booking = model.Booking{
			EventID:   req.EventID,
			UID:       userID,
			UserName:  req.UserName,
			UserEmail: req.UserEmail,
			UserPhone: req.UserPhone,
			Status:    "confirmed",
		}
		return s.repo.CreateBooking(ctx, &booking)
	})

	if txErr != nil {
		// DB failed after Redis was already decremented — restore the counter.
		_ = s.repo.ReleaseQuotaReservation(ctx, req.EventID)

		if errors.Is(txErr, repository.ErrDuplicateBooking) {
			return nil, ErrAlreadyBooked
		}
		return nil, txErr
	}

	// Update seats_booked outside the booking transaction — this is a best-effort
	// audit counter; contention here no longer blocks the booking response.
	if err := s.repo.IncrementSeatsBooked(ctx, s.db, req.EventID); err != nil {
		// Non-fatal: Redis already holds the authoritative remaining count.
		_ = err
	}

	return &BookSeatResult{
		Status:    "confirmed",
		Booking:   &booking,
		Remaining: available - 1,
	}, nil
}

func (s *bookingService) waitlistBooking(ctx context.Context, req BookSeatRequest, userID string) (*BookSeatResult, error) {
	maxPos, err := s.repo.GetMaxWaitlistPosition(ctx, req.EventID)
	if err != nil {
		return nil, fmt.Errorf("get max waitlist position: %w", err)
	}
	position := maxPos + 1

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
		if errors.Is(err, repository.ErrDuplicateBooking) {
			return nil, ErrAlreadyBooked
		}
		return nil, err
	}

	return &BookSeatResult{
		Status:        "waitlisted",
		WaitlistEntry: waitlist,
		Remaining:     0,
	}, nil
}

func stableUserID(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(normalized)).String()
}
