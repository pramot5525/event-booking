package service

import (
	"booking-service/internal/client"
	"booking-service/internal/model"
	"booking-service/internal/repository"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

// bookingNamespace is the UUID v5 namespace used to derive user IDs from email
// addresses. A dedicated namespace avoids collisions with other v5 namespaces.
var bookingNamespace = uuid.MustParse("e9b5c5a0-1f4a-4e8b-9d5c-7f2b3e6a8d1c")

var (
	ErrAlreadyBooked     = errors.New("already booked this event")
	ErrAlreadyWaitlisted = errors.New("already on the waitlist for this event")
	ErrEventNotFound     = errors.New("event not found")
	ErrInvalidUID        = errors.New("invalid uid")
)

// BookEventResult is returned by BookEvent. Status is either "confirmed" or
// "waitlisted"; only the corresponding field is non-nil.
type BookEventResult struct {
	Status        string               `json:"status"`
	Booking       *model.Booking       `json:"booking,omitempty"`
	WaitlistEntry *model.WaitlistEntry `json:"waitlist_entry,omitempty"`
}

type BookingService interface {
	BookEvent(request *model.CreateBookingRequest) (*BookEventResult, error)
	GetUserBookings(uid string) ([]*model.Booking, error)
	GetEventBookings(eventID string) ([]*model.Booking, error)
}

type bookingService struct {
	bookingRepo  repository.BookingRepository
	waitlistRepo repository.WaitlistRepository
	queueRepo    repository.QueueRepository
	seatRepo     repository.SeatRepository
	eventClient  client.EventGetter
}

func NewBookingService(
	bookingRepo repository.BookingRepository,
	waitlistRepo repository.WaitlistRepository,
	queueRepo repository.QueueRepository,
	seatRepo repository.SeatRepository,
	eventClient client.EventGetter,
) BookingService {
	return &bookingService{
		bookingRepo:  bookingRepo,
		waitlistRepo: waitlistRepo,
		queueRepo:    queueRepo,
		seatRepo:     seatRepo,
		eventClient:  eventClient,
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
	if err := s.seatRepo.SetEventCache(ctx, cached); err != nil {
		log.Printf("warning: failed to cache event %d: %v", eventID, err)
	}
	return cached, nil
}

// BookEvent attempts to claim a confirmed seat. When no seat is available the
// caller is automatically added to the waitlist instead.
func (s *bookingService) BookEvent(request *model.CreateBookingRequest) (*BookEventResult, error) {
	ctx := context.Background()

	event, err := s.getEvent(ctx, request.EventID)
	if err != nil {
		return nil, err
	}

	uid := uuid.NewSHA1(bookingNamespace, []byte(request.UserEmail))

	if err := s.initSeatCounter(ctx, request.EventID, event.SeatLimit); err != nil {
		return nil, err
	}

	remaining, err := s.seatRepo.Decrement(ctx, request.EventID)
	if err != nil {
		return nil, fmt.Errorf("claim seat: %w", err)
	}

	if remaining < 0 {
		_ = s.seatRepo.Increment(ctx, request.EventID) // release the over-claimed slot
		return s.addToWaitlist(request, uid)
	}

	return s.persistConfirmedBooking(ctx, request, uid, event.SeatLimit, remaining)
}

// initSeatCounter ensures the Redis seat counter exists for the event.
//
// Under high concurrency every goroutine races to initialise the counter at
// start-up (cold cache). Without a guard all of them would hit the DB
// simultaneously. A short-lived Redis lock (SETNX) ensures exactly one
// goroutine performs the expensive DB count; the rest skip the DB query and
// let Init's SETNX semantics make their call a no-op once the winner sets
// the key.
func (s *bookingService) initSeatCounter(ctx context.Context, eventID int64, seatLimit int32) error {
	booked, err := s.seatRepo.GetBooked(ctx, eventID)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("get booked count: %w", err)
	}

	if err == redis.Nil {
		// Only the goroutine that wins the lock queries the DB.
		won, lockErr := s.seatRepo.TryAcquireInitLock(ctx, eventID)
		if lockErr != nil {
			return fmt.Errorf("acquire init lock: %w", lockErr)
		}
		if won {
			booked, err = s.bookingRepo.CountConfirmedByEventID(eventID)
			if err != nil {
				return fmt.Errorf("count confirmed bookings: %w", err)
			}
			if err := s.seatRepo.SetBooked(ctx, eventID, booked); err != nil {
				return fmt.Errorf("set booked count: %w", err)
			}
		} else {
			// Another goroutine is seeding; use 0 as a safe default.
			// Init below uses SETNX so it becomes a no-op once the winner
			// sets the key with the correct value.
			booked = 0
		}
	}

	available := int32(seatLimit) - int32(booked)
	if err := s.seatRepo.Init(ctx, eventID, available); err != nil {
		return fmt.Errorf("init seat counter: %w", err)
	}
	return nil
}

// persistConfirmedBooking saves the booking record and enqueues it for
// downstream processing. On DB failure it rolls back the claimed seat slot.
func (s *bookingService) persistConfirmedBooking(ctx context.Context, request *model.CreateBookingRequest, uid uuid.UUID, seatLimit int32, remaining int64) (*BookEventResult, error) {
	onWaitlist, err := s.waitlistRepo.ExistsByEventAndUser(request.EventID, uid)
	if err != nil {
		return nil, fmt.Errorf("check duplicate waitlist: %w", err)
	}
	if onWaitlist {
		_ = s.seatRepo.Increment(ctx, request.EventID)
		return nil, ErrAlreadyWaitlisted
	}

	seatNum := int32(seatLimit) - int32(remaining)
	booking := &model.Booking{
		EventID:    request.EventID,
		UID:        uid,
		UserName:   request.UserName,
		UserEmail:  request.UserEmail,
		UserPhone:  request.UserPhone,
		Status:     model.BookingStatusConfirmed,
		SeatNumber: &seatNum,
	}

	if err := s.bookingRepo.CreateBooking(booking); err != nil {
		_ = s.seatRepo.Increment(ctx, request.EventID) // roll back the claimed slot
		if isUniqueViolation(err) {
			return nil, ErrAlreadyBooked
		}
		return nil, fmt.Errorf("create booking: %w", err)
	}

	_ = s.seatRepo.IncrementBooked(ctx, request.EventID)
	s.enqueue(booking)

	return &BookEventResult{Status: "confirmed", Booking: booking}, nil
}

func (s *bookingService) addToWaitlist(request *model.CreateBookingRequest, uid uuid.UUID) (*BookEventResult, error) {
	exists, err := s.bookingRepo.ExistsByEventAndUID(request.EventID, uid)
	if err != nil {
		return nil, fmt.Errorf("check duplicate booking: %w", err)
	}
	if exists {
		return nil, ErrAlreadyBooked
	}

	position, err := s.waitlistRepo.CountWaiting(request.EventID)
	if err != nil {
		return nil, fmt.Errorf("count waitlist: %w", err)
	}

	entry := &model.WaitlistEntry{
		EventID:   request.EventID,
		UID:       uid,
		UserName:  request.UserName,
		UserEmail: request.UserEmail,
		UserPhone: request.UserPhone,
		Position:  position + 1,
		Status:    model.WaitlistStatusWaiting,
	}
	if err := s.waitlistRepo.Add(entry); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyWaitlisted
		}
		return nil, fmt.Errorf("add to waitlist: %w", err)
	}

	return &BookEventResult{Status: "waitlisted", WaitlistEntry: entry}, nil
}

func (s *bookingService) GetUserBookings(u string) ([]*model.Booking, error) {
	uid, err := uuid.Parse(u)
	if err != nil {
		return nil, ErrInvalidUID
	}
	return s.bookingRepo.GetBookingsByUID(uid)
}

func (s *bookingService) GetEventBookings(eventID string) ([]*model.Booking, error) {
	id, err := strconv.ParseInt(eventID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid event_id: %w", err)
	}
	return s.bookingRepo.GetBookingsByEventID(id)
}

func (s *bookingService) enqueue(booking *model.Booking) {
	payload, err := json.Marshal(booking)
	if err != nil {
		log.Printf("warning: failed to marshal booking %d for queue: %v", booking.ID, err)
		return
	}
	if err := s.queueRepo.Enqueue(context.Background(), booking.EventID, string(payload)); err != nil {
		log.Printf("warning: failed to enqueue booking %d: %v", booking.ID, err)
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
