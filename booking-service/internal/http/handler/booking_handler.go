package handler

import (
	"booking-service/internal/model"
	"booking-service/internal/service"
	"errors"

	"github.com/gofiber/fiber/v2"
)

type bookingHandler struct {
	bookingService service.BookingService
}

func NewBookingHandler(bookingService service.BookingService) *bookingHandler {
	return &bookingHandler{bookingService: bookingService}
}

func (h *bookingHandler) BookEvent(c *fiber.Ctx) error {
	req := new(model.CreateBookingRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	booking, err := h.bookingService.BookEvent(req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEventBusy):
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, service.ErrAlreadyBooked):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, service.ErrEventNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	return c.Status(fiber.StatusCreated).JSON(booking)
}

func (h *bookingHandler) GetUserBooking(c *fiber.Ctx) error {
	userID := c.Params("userID")

	booking, err := h.bookingService.GetUserBooking(userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(booking)
}

func (h *bookingHandler) GetEventBookings(c *fiber.Ctx) error {
	eventID := c.Params("eventID")

	bookings, err := h.bookingService.GetEventBookings(eventID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(bookings)
}
