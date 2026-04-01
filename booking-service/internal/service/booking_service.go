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
	ErrInvalidUserID     = errors.New("invalid user_id")
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
	CancelBooking(bookingID int64) error
	GetUserBookings(userID string) ([]*model.Booking, error)
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

	userID := uuid.NewSHA1(bookingNamespace, []byte(request.UserEmail))

	// Fast-path duplicate check (authoritative guard is the DB unique constraint).
	exists, err := s.bookingRepo.ExistsByEventAndUser(request.EventID, userID)
	if err != nil {
		return nil, fmt.Errorf("check duplicate booking: %w", err)
	}
	if exists {
		return nil, ErrAlreadyBooked
	}

	// Check waitlist duplicate.
	onWaitlist, err := s.waitlistRepo.ExistsByEventAndUser(request.EventID, userID)
	if err != nil {
		return nil, fmt.Errorf("check duplicate waitlist: %w", err)
	}
	if onWaitlist {
		return nil, ErrAlreadyWaitlisted
	}

	// Initialise seat counter from DB on first request for this event.
	booked, err := s.seatRepo.GetBooked(ctx, request.EventID)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("get booked: %w", err)
	}
	if err == redis.Nil {
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
	if err := s.seatRepo.Init(ctx, request.EventID, available); err != nil {
		return nil, fmt.Errorf("init seat counter: %w", err)
	}

	// Atomically claim one seat.
	newRemaining, err := s.seatRepo.Decrement(ctx, request.EventID)
	if err != nil {
		return nil, fmt.Errorf("decrement seat: %w", err)
	}

	if newRemaining < 0 {
		// No seat — return the counter and add the user to the waitlist.
		_ = s.seatRepo.Increment(ctx, request.EventID)
		return s.addToWaitlist(request, userID)
	}

	// Seat claimed — persist the booking.
	seatNum := int32(event.SeatLimit) - int32(newRemaining)
	booking := &model.Booking{
		EventID:    request.EventID,
		UserID:     userID,
		Status:     model.BookingStatusConfirmed,
		SeatNumber: &seatNum,
	}

	if err := s.bookingRepo.CreateBooking(booking); err != nil {
		_ = s.seatRepo.Increment(ctx, request.EventID)
		if isUniqueViolation(err) {
			return nil, ErrAlreadyBooked
		}
		return nil, fmt.Errorf("create booking: %w", err)
	}

	_ = s.seatRepo.IncrementBooked(ctx, request.EventID)
	s.enqueue(booking)

	return &BookEventResult{Status: "confirmed", Booking: booking}, nil
}

func (s *bookingService) addToWaitlist(
	request *model.CreateBookingRequest,
	userID uuid.UUID,
) (*BookEventResult, error) {
	position, err := s.waitlistRepo.CountWaiting(request.EventID)
	if err != nil {
		return nil, fmt.Errorf("count waitlist: %w", err)
	}

	entry := &model.WaitlistEntry{
		EventID:   request.EventID,
		UserID:    userID,
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

// CancelBooking cancels a confirmed booking and promotes the next waiting user
// (if any) into that seat atomically. If no one is waiting the seat is freed
// back into the Redis counter.
func (s *bookingService) CancelBooking(bookingID int64) error {
	ctx := context.Background()

	// Pre-flight: verify the booking exists and is confirmed.
	booking, err := s.bookingRepo.GetByID(bookingID)
	if err != nil {
		return err // ErrBookingNotFound is already wrapped
	}
	if booking.Status == model.BookingStatusCancelled {
		return repository.ErrAlreadyCancelled
	}

	// Atomically cancel and maybe promote (single DB transaction).
	promoted, newBooking, err := s.bookingRepo.CancelAndMaybePromote(bookingID)
	if err != nil {
		return fmt.Errorf("cancel booking: %w", err)
	}

	if promoted == nil {
		// No one was waiting — free the seat in Redis.
		if incrErr := s.seatRepo.Increment(ctx, booking.EventID); incrErr != nil {
			log.Printf("warning: failed to increment seat counter for event %d: %v", booking.EventID, incrErr)
		}
	} else {
		// A waitlisted user was promoted — notify them.
		s.enqueue(newBooking)
		log.Printf("waitlist promotion: entry %d → booking %d (event %d)", promoted.ID, newBooking.ID, booking.EventID)
	}

	return nil
}

func (s *bookingService) GetUserBookings(userID string) ([]*model.Booking, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrInvalidUserID
	}
	return s.bookingRepo.GetBookingsByUserID(uid)
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
	if err := s.queueRepo.Enqueue(context.Background(), string(payload)); err != nil {
		log.Printf("warning: failed to enqueue booking %d: %v", booking.ID, err)
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
