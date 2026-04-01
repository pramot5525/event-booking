package http

import (
	"event-service/internal/http/handler"
	"event-service/internal/service"

	"github.com/gofiber/fiber/v2"
	swagger "github.com/gofiber/swagger"
)

func NewRouter(app *fiber.App, eventService service.EventService) {
	app.Get("/docs/openapi.yaml", func(c *fiber.Ctx) error {
		return c.SendFile("./docs/openapi.yaml")
	})
	app.Get("/swagger/*", swagger.New(swagger.Config{
		URL: "/docs/openapi.yaml",
	}))

	eventHandler := handler.NewEventHandler(eventService)

	v1 := app.Group("/api/v1")
	v1.Get("/events", eventHandler.GetEvents)
	v1.Post("/events", eventHandler.CreateEvent)
	v1.Get("/events/:id", eventHandler.GetEvent)
	v1.Put("/events/:id", eventHandler.UpdateEvent)
	v1.Delete("/events/:id", eventHandler.DeleteEvent)
}
