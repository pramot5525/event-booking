package service_test

import (
	clientmocks "booking-service/internal/client/mocks"
	"booking-service/internal/model"
	"booking-service/internal/repository"
	repomocks "booking-service/internal/repository/mocks"
	"booking-service/internal/service"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BookingServiceSuite struct {
	suite.Suite

	bookingRepo  *repomocks.BookingRepository
	waitlistRepo *repomocks.WaitlistRepository
	queueRepo    *repomocks.QueueRepository
	seatRepo     *repomocks.SeatRepository
	eventClient  *clientmocks.EventGetter

	svc service.BookingService
}

func (s *BookingServiceSuite) SetupTest() {
	s.bookingRepo = &repomocks.BookingRepository{}
	s.waitlistRepo = &repomocks.WaitlistRepository{}
	s.queueRepo = &repomocks.QueueRepository{}
	s.seatRepo = &repomocks.SeatRepository{}
	s.eventClient = &clientmocks.EventGetter{}

	s.svc = service.NewBookingService(
		s.bookingRepo,
		s.waitlistRepo,
		s.queueRepo,
		s.seatRepo,
		s.eventClient,
	)
}

func (s *BookingServiceSuite) TearDownTest() {
	s.bookingRepo.AssertExpectations(s.T())
	s.waitlistRepo.AssertExpectations(s.T())
	s.queueRepo.AssertExpectations(s.T())
	s.seatRepo.AssertExpectations(s.T())
	s.eventClient.AssertExpectations(s.T())
}

func (s *BookingServiceSuite) TestBookEvent_Confirmed() {
	request := &model.CreateBookingRequest{
		EventID:   1,
		UserName:  "Alice",
		UserEmail: "alice@example.com",
		UserPhone: "0811111111",
	}

	s.seatRepo.On("GetEventCache", mock.Anything, int64(1)).Return(&repository.CachedEvent{ID: 1, SeatLimit: 1}, nil)
	s.bookingRepo.On("ExistsByEventAndUID", int64(1), mock.Anything).Return(false, nil)
	s.waitlistRepo.On("ExistsByEventAndUser", int64(1), mock.Anything).Return(false, nil)
	s.seatRepo.On("GetBooked", mock.Anything, int64(1)).Return(int64(0), redis.Nil)
	s.bookingRepo.On("CountConfirmedByEventID", int64(1)).Return(int64(0), nil)
	s.seatRepo.On("SetBooked", mock.Anything, int64(1), int64(0)).Return(nil)
	s.seatRepo.On("Init", mock.Anything, int64(1), int32(1)).Return(nil)
	s.seatRepo.On("Decrement", mock.Anything, int64(1)).Return(int64(0), nil)
	s.bookingRepo.On("CreateBooking", mock.AnythingOfType("*model.Booking")).Return(nil)
	s.seatRepo.On("IncrementBooked", mock.Anything, int64(1)).Return(nil)
	s.queueRepo.On("Enqueue", mock.Anything, int64(1), mock.AnythingOfType("string")).Return(nil)

	result, err := s.svc.BookEvent(request)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "confirmed", result.Status)
	require.NotNil(s.T(), result.Booking)
	require.NotNil(s.T(), result.Booking.SeatNumber)
	require.EqualValues(s.T(), 1, *result.Booking.SeatNumber)
}

func (s *BookingServiceSuite) TestBookEvent_WaitlistedWhenNoSeat() {
	request := &model.CreateBookingRequest{
		EventID:   2,
		UserName:  "Bob",
		UserEmail: "bob@example.com",
		UserPhone: "0822222222",
	}

	s.seatRepo.On("GetEventCache", mock.Anything, int64(2)).Return(&repository.CachedEvent{ID: 2, SeatLimit: 1}, nil)
	s.bookingRepo.On("ExistsByEventAndUID", int64(2), mock.Anything).Return(false, nil)
	s.waitlistRepo.On("ExistsByEventAndUser", int64(2), mock.Anything).Return(false, nil)
	s.seatRepo.On("GetBooked", mock.Anything, int64(2)).Return(int64(1), nil)
	s.seatRepo.On("Init", mock.Anything, int64(2), int32(0)).Return(nil)
	s.seatRepo.On("Decrement", mock.Anything, int64(2)).Return(int64(-1), nil)
	s.seatRepo.On("Increment", mock.Anything, int64(2)).Return(nil)
	s.waitlistRepo.On("CountWaiting", int64(2)).Return(int32(3), nil)
	s.waitlistRepo.On("Add", mock.AnythingOfType("*model.WaitlistEntry")).Return(nil)

	result, err := s.svc.BookEvent(request)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "waitlisted", result.Status)
	require.NotNil(s.T(), result.WaitlistEntry)
	require.EqualValues(s.T(), 4, result.WaitlistEntry.Position)
}

func (s *BookingServiceSuite) TestBookEvent_EventNotFound() {
	request := &model.CreateBookingRequest{EventID: 99, UserEmail: "notfound@example.com"}

	s.seatRepo.On("GetEventCache", mock.Anything, int64(99)).Return(nil, redis.Nil)
	s.eventClient.On("GetEvent", int64(99)).Return(nil, errors.New("event not found"))

	_, err := s.svc.BookEvent(request)
	require.Error(s.T(), err)

	// TODO: switch to errors.Is(err, service.ErrEventNotFound) when EventGetter
	// mock returns client.ErrEventNotFound in your local tests.
}

// TODOs for you:
// 1) Add table-driven cases for duplicate booking and duplicate waitlist.
// 2) Add rollback case: CreateBooking returns error -> Increment must be called.
// 3) Add queue failure case: Enqueue error must not fail BookEvent.
// 4) Add concurrency-focused service tests using a thread-safe fake SeatRepository.

func TestBookingServiceSuite(t *testing.T) {
	suite.Run(t, new(BookingServiceSuite))
}
