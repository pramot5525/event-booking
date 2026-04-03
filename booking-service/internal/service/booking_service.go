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

	var result *BookSeatResult

	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		quota, err := s.repo.LockQuota(ctx, tx, req.EventID)
		if err != nil {
			return fmt.Errorf("lock quota: %w", err)
		}

		if quota.SeatsBooked < quota.SeatsTotal {
			// Seat available: confirm booking.
			if err := s.repo.IncrementSeatsBooked(ctx, tx, req.EventID); err != nil {
				return fmt.Errorf("increment seats booked: %w", err)
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
				return err
			}

			result = &BookSeatResult{
				Status:    "confirmed",
				Booking:   booking,
				Remaining: quota.SeatsTotal - quota.SeatsBooked - 1,
			}
		} else {
			// No seats: add to waitlist.
			maxPos, err := s.repo.GetMaxWaitlistPosition(ctx, req.EventID)
			if err != nil {
				return fmt.Errorf("get max waitlist position: %w", err)
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
				return err
			}

			result = &BookSeatResult{
				Status:        "waitlisted",
				WaitlistEntry: waitlist,
				Remaining:     0,
			}
		}

		return nil
	})

	if txErr != nil {
		if errors.Is(txErr, repository.ErrDuplicateBooking) {
			return nil, ErrAlreadyBooked
		}
		return nil, txErr
	}

	return result, nil
}

func stableUserID(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(normalized)).String()
}
