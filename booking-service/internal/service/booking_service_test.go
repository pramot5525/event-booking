package service_test

import (
	"booking-service/internal/client"
	clientmocks "booking-service/internal/client/mocks"
	"booking-service/internal/model"
	"booking-service/internal/repository"
	repomocks "booking-service/internal/repository/mocks"
	"booking-service/internal/service"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// ---------------------------------------------------------------------------
// mock container
// ---------------------------------------------------------------------------

type testMocks struct {
	bookingRepo  *repomocks.BookingRepository
	waitlistRepo *repomocks.WaitlistRepository
	queueRepo    *repomocks.QueueRepository
	seatRepo     *repomocks.SeatRepository
	eventClient  *clientmocks.EventGetter
}

func newTestMocks() *testMocks {
	return &testMocks{
		bookingRepo:  &repomocks.BookingRepository{},
		waitlistRepo: &repomocks.WaitlistRepository{},
		queueRepo:    &repomocks.QueueRepository{},
		seatRepo:     &repomocks.SeatRepository{},
		eventClient:  &clientmocks.EventGetter{},
	}
}

func (m *testMocks) service() service.BookingService {
	return service.NewBookingService(
		m.bookingRepo, m.waitlistRepo, m.queueRepo, m.seatRepo, m.eventClient,
	)
}

func (m *testMocks) assertExpectations(t *testing.T) {
	m.bookingRepo.AssertExpectations(t)
	m.waitlistRepo.AssertExpectations(t)
	m.queueRepo.AssertExpectations(t)
	m.seatRepo.AssertExpectations(t)
	m.eventClient.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// scenario helpers — each helper sets up one stage of the booking flow
// ---------------------------------------------------------------------------

// givenEventCached stubs the seat-repo cache with the provided event.
func givenEventCached(m *testMocks, event *repository.CachedEvent) {
	m.seatRepo.On("GetEventCache", mock.Anything, event.ID).Return(event, nil)
}

// givenNoDuplicates stubs both duplicate checks to return "not found".
func givenNoDuplicates(m *testMocks, eventID int64) {
	m.bookingRepo.On("ExistsByEventAndUID", eventID, mock.Anything).Return(false, nil)
	m.waitlistRepo.On("ExistsByEventAndUser", eventID, mock.Anything).Return(false, nil)
}

// givenSeatCounterInRedis stubs the seat counter as already seeded in Redis.
// available = event.SeatLimit - booked.
func givenSeatCounterInRedis(m *testMocks, event *repository.CachedEvent, booked int64) {
	available := int32(event.SeatLimit) - int32(booked)
	m.seatRepo.On("GetBooked", mock.Anything, event.ID).Return(booked, nil)
	m.seatRepo.On("Init", mock.Anything, event.ID, available).Return(nil)
}

// givenSeatCounterFromDB stubs the cold-start path: GetBooked misses in Redis,
// so the counter is seeded from the confirmed-booking count in the DB.
func givenSeatCounterFromDB(m *testMocks, event *repository.CachedEvent, confirmedCount int64) {
	available := int32(event.SeatLimit) - int32(confirmedCount)
	m.seatRepo.On("GetBooked", mock.Anything, event.ID).Return(int64(0), redis.Nil)
	m.bookingRepo.On("CountConfirmedByEventID", event.ID).Return(confirmedCount, nil)
	m.seatRepo.On("SetBooked", mock.Anything, event.ID, confirmedCount).Return(nil)
	m.seatRepo.On("Init", mock.Anything, event.ID, available).Return(nil)
}

// givenSeatsFull stubs the atomic claim to return -1 (no seat) and the
// subsequent rollback Increment.
func givenSeatsFull(m *testMocks, eventID int64) {
	m.seatRepo.On("Decrement", mock.Anything, eventID).Return(int64(-1), nil)
	m.seatRepo.On("Increment", mock.Anything, eventID).Return(nil)
}

// givenBookingCreated stubs a successful DB write, booked-counter increment,
// and queue publish. Pass a non-nil queueErr to simulate queue failures.
func givenBookingCreated(m *testMocks, eventID int64, queueErr error) {
	m.bookingRepo.On("CreateBooking", mock.AnythingOfType("*model.Booking")).Return(nil)
	m.seatRepo.On("IncrementBooked", mock.Anything, eventID).Return(nil)
	m.queueRepo.On("Enqueue", mock.Anything, eventID, mock.AnythingOfType("string")).Return(queueErr)
}

// givenWaitlistEntry stubs CountWaiting (returning currentWaiting) and a
// successful Add. The resulting position will be currentWaiting + 1.
func givenWaitlistEntry(m *testMocks, eventID int64, currentWaiting int32) {
	m.waitlistRepo.On("CountWaiting", eventID).Return(currentWaiting, nil)
	m.waitlistRepo.On("Add", mock.AnythingOfType("*model.WaitlistEntry")).Return(nil)
}

// uniqueViolationErr mimics a Postgres unique-constraint error (code 23505).
func uniqueViolationErr() error {
	return &pgconn.PgError{Code: "23505"}
}

// ---------------------------------------------------------------------------
// suite
// ---------------------------------------------------------------------------

type BookingServiceSuite struct {
	suite.Suite
}

func TestBookingServiceSuite(t *testing.T) {
	suite.Run(t, new(BookingServiceSuite))
}

// ---------------------------------------------------------------------------
// BookEvent
// ---------------------------------------------------------------------------

func (s *BookingServiceSuite) TestBookEvent() {
	cachedEvent := &repository.CachedEvent{ID: 1, SeatLimit: 2}
	req := func(eventID int64, email string) *model.CreateBookingRequest {
		return &model.CreateBookingRequest{
			EventID: eventID, UserName: "Test User",
			UserEmail: email, UserPhone: "0800000000",
		}
	}

	tests := []struct {
		name    string
		request *model.CreateBookingRequest
		setup   func(m *testMocks)
		wantErr error                          // if set, errors.Is is asserted
		check   func(*service.BookEventResult) // if set, no error is expected
		// if neither wantErr nor check is set, any non-nil error is expected
	}{
		// --- confirmed ---
		{
			name:    "confirmed_counter_already_in_redis",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterInRedis(m, cachedEvent, 0) // 0 booked → 2 available
				m.seatRepo.On("Decrement", mock.Anything, int64(1)).Return(int64(1), nil)
				givenBookingCreated(m, 1, nil)
			},
			check: func(r *service.BookEventResult) {
				s.Equal("confirmed", r.Status)
				s.EqualValues(1, *r.Booking.SeatNumber) // SeatLimit(2) - remaining(1)
			},
		},
		{
			name:    "confirmed_counter_seeded_from_db",
			request: req(1, "bob@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterFromDB(m, cachedEvent, 0) // 0 confirmed in DB → 2 available
				m.seatRepo.On("Decrement", mock.Anything, int64(1)).Return(int64(0), nil)
				givenBookingCreated(m, 1, nil)
			},
			check: func(r *service.BookEventResult) {
				s.Equal("confirmed", r.Status)
				s.EqualValues(2, *r.Booking.SeatNumber) // SeatLimit(2) - remaining(0)
			},
		},
		{
			name:    "queue_failure_does_not_fail_confirmed_booking",
			request: req(1, "eve@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterInRedis(m, cachedEvent, 0)
				m.seatRepo.On("Decrement", mock.Anything, int64(1)).Return(int64(1), nil)
				givenBookingCreated(m, 1, errors.New("queue unavailable"))
			},
			check: func(r *service.BookEventResult) {
				s.Equal("confirmed", r.Status)
			},
		},
		// --- waitlisted ---
		{
			name:    "waitlisted_first_in_queue",
			request: req(1, "carol@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterInRedis(m, cachedEvent, 2) // all 2 seats taken → 0 available
				givenSeatsFull(m, 1)
				givenWaitlistEntry(m, 1, 0) // empty queue → position 1
			},
			check: func(r *service.BookEventResult) {
				s.Equal("waitlisted", r.Status)
				s.EqualValues(1, r.WaitlistEntry.Position)
			},
		},
		{
			name:    "waitlisted_appended_to_existing_queue",
			request: req(1, "dave@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterInRedis(m, cachedEvent, 2)
				givenSeatsFull(m, 1)
				givenWaitlistEntry(m, 1, 5) // 5 ahead → position 6
			},
			check: func(r *service.BookEventResult) {
				s.Equal("waitlisted", r.Status)
				s.EqualValues(6, r.WaitlistEntry.Position)
			},
		},
		// --- duplicate / guard errors ---
		{
			name:    "error_duplicate_booking",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				m.bookingRepo.On("ExistsByEventAndUID", int64(1), mock.Anything).Return(true, nil)
			},
			wantErr: service.ErrAlreadyBooked,
		},
		{
			name:    "error_duplicate_waitlist",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				m.bookingRepo.On("ExistsByEventAndUID", int64(1), mock.Anything).Return(false, nil)
				m.waitlistRepo.On("ExistsByEventAndUser", int64(1), mock.Anything).Return(true, nil)
			},
			wantErr: service.ErrAlreadyWaitlisted,
		},
		{
			name:    "error_event_not_found",
			request: req(99, "alice@example.com"),
			setup: func(m *testMocks) {
				m.seatRepo.On("GetEventCache", mock.Anything, int64(99)).Return(nil, redis.Nil)
				m.eventClient.On("GetEvent", int64(99)).Return(nil, client.ErrEventNotFound)
			},
			wantErr: service.ErrEventNotFound,
		},
		{
			name:    "error_create_booking_unique_violation_rolls_back_seat",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterInRedis(m, cachedEvent, 0)
				m.seatRepo.On("Decrement", mock.Anything, int64(1)).Return(int64(1), nil)
				m.bookingRepo.On("CreateBooking", mock.AnythingOfType("*model.Booking")).Return(uniqueViolationErr())
				m.seatRepo.On("Increment", mock.Anything, int64(1)).Return(nil) // rollback
			},
			wantErr: service.ErrAlreadyBooked,
		},
		{
			name:    "error_add_to_waitlist_unique_violation",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterInRedis(m, cachedEvent, 2)
				givenSeatsFull(m, 1)
				m.waitlistRepo.On("CountWaiting", int64(1)).Return(int32(0), nil)
				m.waitlistRepo.On("Add", mock.AnythingOfType("*model.WaitlistEntry")).Return(uniqueViolationErr())
			},
			wantErr: service.ErrAlreadyWaitlisted,
		},
		// --- infrastructure errors ---
		{
			name:    "error_event_cache_redis_failure",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				m.seatRepo.On("GetEventCache", mock.Anything, int64(1)).Return(nil, errors.New("redis error"))
			},
		},
		{
			name:    "error_duplicate_booking_check_db_failure",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				m.bookingRepo.On("ExistsByEventAndUID", int64(1), mock.Anything).Return(false, errors.New("db error"))
			},
		},
		{
			name:    "error_duplicate_waitlist_check_db_failure",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				m.bookingRepo.On("ExistsByEventAndUID", int64(1), mock.Anything).Return(false, nil)
				m.waitlistRepo.On("ExistsByEventAndUser", int64(1), mock.Anything).Return(false, errors.New("db error"))
			},
		},
		{
			name:    "error_get_booked_redis_failure",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				m.seatRepo.On("GetBooked", mock.Anything, int64(1)).Return(int64(0), errors.New("redis error"))
			},
		},
		{
			name:    "error_count_confirmed_db_failure",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				m.seatRepo.On("GetBooked", mock.Anything, int64(1)).Return(int64(0), redis.Nil)
				m.bookingRepo.On("CountConfirmedByEventID", int64(1)).Return(int64(0), errors.New("db error"))
			},
		},
		{
			name:    "error_set_booked_redis_failure",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				m.seatRepo.On("GetBooked", mock.Anything, int64(1)).Return(int64(0), redis.Nil)
				m.bookingRepo.On("CountConfirmedByEventID", int64(1)).Return(int64(0), nil)
				m.seatRepo.On("SetBooked", mock.Anything, int64(1), int64(0)).Return(errors.New("redis error"))
			},
		},
		{
			name:    "error_init_seat_counter_redis_failure",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				m.seatRepo.On("GetBooked", mock.Anything, int64(1)).Return(int64(0), nil)
				m.seatRepo.On("Init", mock.Anything, int64(1), int32(2)).Return(errors.New("redis error"))
			},
		},
		{
			name:    "error_decrement_redis_failure",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterInRedis(m, cachedEvent, 0)
				m.seatRepo.On("Decrement", mock.Anything, int64(1)).Return(int64(0), errors.New("redis error"))
			},
		},
		{
			name:    "error_create_booking_generic_db_failure_rolls_back_seat",
			request: req(1, "alice@example.com"),
			setup: func(m *testMocks) {
				givenEventCached(m, cachedEvent)
				givenNoDuplicates(m, 1)
				givenSeatCounterInRedis(m, cachedEvent, 0)
				m.seatRepo.On("Decrement", mock.Anything, int64(1)).Return(int64(1), nil)
				m.bookingRepo.On("CreateBooking", mock.AnythingOfType("*model.Booking")).Return(errors.New("db error"))
				m.seatRepo.On("Increment", mock.Anything, int64(1)).Return(nil) // rollback
			},
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			m := newTestMocks()
			tc.setup(m)

			result, err := m.service().BookEvent(tc.request)

			switch {
			case tc.wantErr != nil:
				s.ErrorIs(err, tc.wantErr)
			case tc.check != nil:
				s.NoError(err)
				tc.check(result)
			default:
				s.Error(err)
			}
			m.assertExpectations(s.T())
		})
	}
}

// ---------------------------------------------------------------------------
// GetUserBookings
// ---------------------------------------------------------------------------

func (s *BookingServiceSuite) TestGetUserBookings() {
	validUID := uuid.New()
	bookings := []*model.Booking{{ID: 1, EventID: 42}}

	tests := []struct {
		name    string
		uid     string
		setup   func(m *testMocks)
		wantErr error
		check   func([]*model.Booking)
	}{
		{
			name: "returns_bookings_for_valid_uid",
			uid:  validUID.String(),
			setup: func(m *testMocks) {
				m.bookingRepo.On("GetBookingsByUID", validUID).Return(bookings, nil)
			},
			check: func(result []*model.Booking) {
				s.Len(result, 1)
				s.Equal(int64(42), result[0].EventID)
			},
		},
		{
			name:    "error_invalid_uid_string",
			uid:     "not-a-uuid",
			setup:   func(m *testMocks) {},
			wantErr: service.ErrInvalidUID,
		},
		{
			name: "error_repository_db_failure",
			uid:  validUID.String(),
			setup: func(m *testMocks) {
				m.bookingRepo.On("GetBookingsByUID", validUID).Return(nil, errors.New("db error"))
			},
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			m := newTestMocks()
			tc.setup(m)

			result, err := m.service().GetUserBookings(tc.uid)

			switch {
			case tc.wantErr != nil:
				s.ErrorIs(err, tc.wantErr)
			case tc.check != nil:
				s.NoError(err)
				tc.check(result)
			default:
				s.Error(err)
			}
			m.assertExpectations(s.T())
		})
	}
}

// ---------------------------------------------------------------------------
// GetEventBookings
// ---------------------------------------------------------------------------

func (s *BookingServiceSuite) TestGetEventBookings() {
	bookings := []*model.Booking{{ID: 1, EventID: 10}}

	tests := []struct {
		name    string
		eventID string
		setup   func(m *testMocks)
		check   func([]*model.Booking)
		wantErr bool
	}{
		{
			name:    "returns_bookings_for_valid_event_id",
			eventID: "10",
			setup: func(m *testMocks) {
				m.bookingRepo.On("GetBookingsByEventID", int64(10)).Return(bookings, nil)
			},
			check: func(result []*model.Booking) {
				s.Len(result, 1)
				s.Equal(int64(10), result[0].EventID)
			},
		},
		{
			name:    "error_non_numeric_event_id",
			eventID: "abc",
			setup:   func(m *testMocks) {},
			wantErr: true,
		},
		{
			name:    "error_repository_db_failure",
			eventID: "10",
			setup: func(m *testMocks) {
				m.bookingRepo.On("GetBookingsByEventID", int64(10)).Return(nil, errors.New("db error"))
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			m := newTestMocks()
			tc.setup(m)

			result, err := m.service().GetEventBookings(tc.eventID)

			if tc.wantErr {
				s.Error(err)
			} else {
				s.NoError(err)
				if tc.check != nil {
					tc.check(result)
				}
			}
			m.assertExpectations(s.T())
		})
	}
}
