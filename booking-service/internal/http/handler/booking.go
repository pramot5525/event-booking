package handler

import (
	"booking-service/internal/service"
	"errors"

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

type initQuotaRequest struct {
	EventID uint  `json:"event_id"`
	Quota   int64 `json:"quota"`
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
		case errors.Is(err, service.ErrBookingInProgress):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "booking is already in progress for this user"})
		case errors.Is(err, service.ErrAlreadyBooked):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "user already booked this event"})
		case errors.Is(err, service.ErrSoldOut):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "seats sold out"})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "booking failed"})
		}
	}

	return c.Status(fiber.StatusCreated).JSON(result)
}

func (h *BookingHandler) InitializeQuota(c *fiber.Ctx) error {
	var req initQuotaRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
	}

	if err := h.bookingService.InitializeQuota(c.UserContext(), req.EventID, req.Quota); err != nil {
		if errors.Is(err, service.ErrInvalidRequest) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to initialize quota"})
	}

	return c.JSON(fiber.Map{
		"message":  "quota initialized",
		"event_id": req.EventID,
		"quota":    req.Quota,
	})
}

func (h *BookingHandler) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}
