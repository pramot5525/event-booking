package http

import (
	"booking-service/internal/http/handler"
	"booking-service/internal/http/middleware"
	"booking-service/internal/service"

	swagger "github.com/gofiber/swagger"
	"github.com/gofiber/fiber/v2"
)

func NewRouter(app *fiber.App, bookingService service.BookingService, apiKey string) {
	app.Get("/docs/openapi.yaml", func(c *fiber.Ctx) error {
		return c.SendFile("./docs/openapi.yaml")
	})
	app.Get("/swagger/*", swagger.New(swagger.Config{
		URL: "/docs/openapi.yaml",
	}))

	bookingHandler := handler.NewBookingHandler(bookingService)

	v1 := app.Group("/api/v1", middleware.APIKeyAuth(apiKey))
	v1.Post("/bookings", bookingHandler.BookEvent)
	v1.Delete("/bookings/:id", bookingHandler.CancelBooking)
	v1.Get("/bookings/user/:userID", bookingHandler.GetUserBookings)
	v1.Get("/bookings/event/:eventID", bookingHandler.GetEventBookings)
}
