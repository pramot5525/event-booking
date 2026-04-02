package http

import (
	"booking-service/internal/http/handler"
	"booking-service/internal/service"

	"github.com/gofiber/fiber/v2"
	swagger "github.com/gofiber/swagger"
)

func NewRouter(app *fiber.App, bookingService service.BookingService) {
	app.Get("/docs/openapi.yaml", func(c *fiber.Ctx) error {
		return c.SendFile("./docs/openapi.yaml")
	})
	app.Get("/swagger/*", swagger.New(swagger.Config{
		URL: "/docs/openapi.yaml",
	}))

	h := handler.NewBookingHandler(bookingService)

	app.Get("/health", h.Health)

	v1 := app.Group("/api/v1")
	v1.Post("/bookings", h.BookSeat)
	v1.Post("/bookings/quota/init", h.InitializeQuota)
}
