package http

import (
	"booking-service/internal/http/handler"
	"booking-service/internal/service"

	"github.com/gofiber/fiber/v2"
)

func NewRouter(app *fiber.App, bookingService service.BookingService) {
	h := handler.NewBookingHandler(bookingService)

	app.Get("/health", h.Health)

	v1 := app.Group("/api/v1")
	v1.Post("/bookings", h.BookSeat)
	v1.Post("/bookings/quota/init", h.InitializeQuota)
}
