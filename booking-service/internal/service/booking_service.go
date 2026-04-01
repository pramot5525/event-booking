package service

import (
	"booking-service/internal/client"
	"booking-service/internal/model"
	"booking-service/internal/repository"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	lockRetryMax    = 700
	lockRetryDelay  = 20 * time.Millisecond
	lockRetryJitter = 20 * time.Millisecond
)

var (
	ErrEventBusy     = errors.New("event is busy, please try again later")
	ErrAlreadyBooked = errors.New("already booked this event")
	ErrEventNotFound = errors.New("event not found")
)

type BookingService interface {
	BookEvent(request *model.CreateBookingRequest) (*model.Booking, error)
	GetUserBooking(userID string) (*model.Booking, error)
	GetEventBookings(eventID string) ([]*model.Booking, error)
}

type bookingService struct {
	bookingRepo repository.BookingRepository
	lockRepo    repository.LockRepository
	queueRepo   repository.QueueRepository
	seatRepo    repository.SeatRepository
	eventClient *client.EventClient
}

func NewBookingService(
	bookingRepo repository.BookingRepository,
	lockRepo repository.LockRepository,
	queueRepo repository.QueueRepository,
	seatRepo repository.SeatRepository,
	eventClient *client.EventClient,
) BookingService {
	return &bookingService{
		bookingRepo: bookingRepo,
		lockRepo:    lockRepo,
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

func (s *bookingService) acquireLockWithRetry(ctx context.Context, key string, ttl time.Duration) error {
	for i := 0; i < lockRetryMax; i++ {
		acquired, err := s.lockRepo.Acquire(ctx, key, ttl)
		if err != nil {
			return fmt.Errorf("acquire lock: %w", err)
		}
		if acquired {
			return nil
		}
		jitter := time.Duration(rand.Int63n(int64(lockRetryJitter)))
		time.Sleep(lockRetryDelay + jitter)
	}
	return ErrEventBusy
}

func (s *bookingService) BookEvent(request *model.CreateBookingRequest) (*model.Booking, error) {
	ctx := context.Background()

	event, err := s.getEvent(ctx, request.EventID)
	if err != nil {
		return nil, err
	}

	userID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(request.UserEmail))

	lockKey := fmt.Sprintf("event:%d:lock", request.EventID)
	if err := s.acquireLockWithRetry(ctx, lockKey, 30*time.Second); err != nil {
		return nil, err
	}
	defer s.lockRepo.Release(ctx, lockKey)

	exists, err := s.bookingRepo.ExistsByEventAndUser(request.EventID, userID)
	if err != nil {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}
	if exists {
		return nil, ErrAlreadyBooked
	}

	if err := s.seatRepo.Init(ctx, request.EventID, event.SeatLimit); err != nil {
		return nil, fmt.Errorf("init seat counter: %w", err)
	}

	remaining, err := s.seatRepo.Remaining(ctx, request.EventID)
	if err != nil {
		return nil, fmt.Errorf("get remaining seats: %w", err)
	}

	booking := &model.Booking{
		EventID: request.EventID,
		UserID:  userID,
		Status:  model.BookingStatusConfirmed,
	}

	if remaining <= 0 {
		booking.Status = model.BookingStatusPending
	} else {
		newRemaining, err := s.seatRepo.Decrement(ctx, request.EventID)
		if err != nil {
			return nil, fmt.Errorf("decrement seat: %w", err)
		}
		seatNum := int32(event.SeatLimit) - int32(newRemaining)
		booking.SeatNumber = &seatNum
	}

	if err := s.bookingRepo.CreateBooking(booking); err != nil {
		if booking.Status == model.BookingStatusConfirmed {
			_ = s.seatRepo.Increment(ctx, request.EventID)
		}
		return nil, fmt.Errorf("create booking: %w", err)
	}

	payload, _ := json.Marshal(booking)
	_ = s.queueRepo.Enqueue(ctx, string(payload))

	return booking, nil
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
