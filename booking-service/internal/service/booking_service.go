package service

import (
	"booking-service/internal/client"
	"booking-service/internal/model"
	"booking-service/internal/repository"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

var (
	ErrAlreadyBooked    = errors.New("already booked this event")
	ErrEventNotFound    = errors.New("event not found")
	ErrNoSeatsAvailable = errors.New("no seats available")
)

type BookingService interface {
	BookEvent(request *model.CreateBookingRequest) (*model.Booking, error)
	GetUserBooking(userID string) (*model.Booking, error)
	GetEventBookings(eventID string) ([]*model.Booking, error)
}

type bookingService struct {
	bookingRepo repository.BookingRepository
	queueRepo   repository.QueueRepository
	seatRepo    repository.SeatRepository
	eventClient *client.EventClient
}

func NewBookingService(
	bookingRepo repository.BookingRepository,
	queueRepo repository.QueueRepository,
	seatRepo repository.SeatRepository,
	eventClient *client.EventClient,
) BookingService {
	return &bookingService{
		bookingRepo: bookingRepo,
		queueRepo:   queueRepo,
		seatRepo:    seatRepo,
		eventClient: eventClient,
	}
}

func (s *bookingService) getEvent(ctx context.Context, eventID int64) (*repository.CachedEvent, error) {
	cached, err := s.seatRepo.GetEventCache(ctx, eventID)
	if err == nil {
		return cached, nil
	}
	if err != redis.Nil {
		return nil, fmt.Errorf("get event cache: %w", err)
	}

	ev, err := s.eventClient.GetEvent(eventID)
	if err != nil {
		if errors.Is(err, client.ErrEventNotFound) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}

	cached = &repository.CachedEvent{ID: ev.ID, SeatLimit: ev.SeatLimit}
	_ = s.seatRepo.SetEventCache(ctx, cached)
	return cached, nil
}

func (s *bookingService) BookEvent(request *model.CreateBookingRequest) (*model.Booking, error) {
	ctx := context.Background()

	event, err := s.getEvent(ctx, request.EventID)
	if err != nil {
		return nil, err
	}

	userID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(request.UserEmail))

	// Fast-path duplicate check. Racy under extreme concurrency, but the DB
	// unique constraint on (event_id, user_id) is the authoritative guard.
	exists, err := s.bookingRepo.ExistsByEventAndUser(request.EventID, userID)
	if err != nil {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}
	if exists {
		return nil, ErrAlreadyBooked
	}

	// Get or init booked count
	booked, err := s.seatRepo.GetBooked(ctx, request.EventID)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("get booked: %w", err)
	}
	if err == redis.Nil {
		// Count from DB once
		count, err := s.bookingRepo.CountConfirmedByEventID(request.EventID)
		if err != nil {
			return nil, fmt.Errorf("count confirmed bookings: %w", err)
		}
		booked = count
		if err := s.seatRepo.SetBooked(ctx, request.EventID, booked); err != nil {
			return nil, fmt.Errorf("set booked: %w", err)
		}
	}

	available := int32(event.SeatLimit) - int32(booked)

	// Ensure the seat counter exists (SETNX — safe to call concurrently).
	if err := s.seatRepo.Init(ctx, request.EventID, available); err != nil {
		return nil, fmt.Errorf("init seat counter: %w", err)
	}

	// Atomically claim one seat. Redis is single-threaded, so DECR is safe
	// without any application-level lock.
	newRemaining, err := s.seatRepo.Decrement(ctx, request.EventID)
	if err != nil {
		return nil, fmt.Errorf("decrement seat: %w", err)
	}

	if newRemaining < 0 {
		// No seat available — return the counter and fail the booking.
		_ = s.seatRepo.Increment(ctx, request.EventID)
		return nil, ErrNoSeatsAvailable
	}

	seatNum := int32(event.SeatLimit) - int32(newRemaining)

	booking := &model.Booking{
		EventID:    request.EventID,
		UserID:     userID,
		Status:     model.BookingStatusConfirmed,
		SeatNumber: &seatNum,
	}

	if err := s.bookingRepo.CreateBooking(booking); err != nil {
		// On any error, return the seat since booking failed.
		_ = s.seatRepo.Increment(ctx, request.EventID)
		if isUniqueViolation(err) {
			return nil, ErrAlreadyBooked
		}
		return nil, fmt.Errorf("create booking: %w", err)
	}

	// Increment booked count on success
	_ = s.seatRepo.IncrementBooked(ctx, request.EventID)

	payload, _ := json.Marshal(booking)
	_ = s.queueRepo.Enqueue(ctx, string(payload))

	return booking, nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *bookingService) GetUserBooking(userID string) (*model.Booking, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}
	return s.bookingRepo.GetBookingByUserID(uid)
}

func (s *bookingService) GetEventBookings(eventID string) ([]*model.Booking, error) {
	id, err := strconv.ParseInt(eventID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid event_id: %w", err)
	}
	return s.bookingRepo.GetBookingsByEventID(id)
}
