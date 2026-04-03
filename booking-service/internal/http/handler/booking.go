package handler

import (
	"booking-service/internal/service"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
)

type BookingHandler struct {
	bookingService service.BookingService
}

func NewBookingHandler(bookingService service.BookingService) *BookingHandler {
	return &BookingHandler{bookingService: bookingService}
}

type createBookingRequest struct {
	EventID   uint   `json:"event_id"`
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
	UserPhone string `json:"user_phone"`
}

func (h *BookingHandler) BookSeat(c *fiber.Ctx) error {
	var req createBookingRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	result, err := h.bookingService.BookSeat(c.UserContext(), service.BookSeatRequest{
		EventID:   req.EventID,
		UserName:  req.UserName,
		UserEmail: req.UserEmail,
		UserPhone: req.UserPhone,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRequest):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, service.ErrAlreadyBooked):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "user already booked this event"})
		default:
			log.Printf("booking error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "booking failed, please try again"})
		}
	}

	return c.Status(fiber.StatusCreated).JSON(result)
}
