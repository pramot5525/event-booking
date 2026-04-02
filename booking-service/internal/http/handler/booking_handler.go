package handler

import (
	"booking-service/internal/model"
	"booking-service/internal/pkg/validate"
	"booking-service/internal/service"
	"errors"
	"log"

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
	if req.EventID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid event_id"})
	}
	if req.UserName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_name is required"})
	}
	if req.UserEmail == "" || !validate.ValidateEmail(req.UserEmail) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user_email"})
	}
	if req.UserPhone == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_phone is required"})
	}

	result, err := h.bookingService.BookEvent(req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAlreadyBooked):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, service.ErrAlreadyWaitlisted):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, service.ErrEventNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		default:
			log.Printf("book event error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
		}
	}

	return c.Status(fiber.StatusCreated).JSON(result)
}

func (h *bookingHandler) GetUserBookings(c *fiber.Ctx) error {
	uid := c.Params("uid")

	bookings, err := h.bookingService.GetUserBookings(uid)
	if err != nil {
		if errors.Is(err, service.ErrInvalidUID) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid uid"})
		}
		log.Printf("get user bookings error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}

	return c.Status(fiber.StatusOK).JSON(bookings)
}

func (h *bookingHandler) GetEventBookings(c *fiber.Ctx) error {
	eventID := c.Params("eventID")

	bookings, err := h.bookingService.GetEventBookings(eventID)
	if err != nil {
		log.Printf("get event bookings error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}

	return c.Status(fiber.StatusOK).JSON(bookings)
}
