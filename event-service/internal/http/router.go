package http

import (
	"event-service/internal/http/handler"
	"event-service/internal/service"

	swagger "github.com/gofiber/swagger"
	"github.com/gofiber/fiber/v2"
)

func NewRouter(app *fiber.App, eventService service.EventService) {
	app.Static("/docs/openapi.yaml", "./docs/openapi.yaml")
	app.Get("/docs/*", swagger.New(swagger.Config{
		URL: "/docs/openapi.yaml",
	}))

	eventHandler := handler.NewEventHandler(eventService)

	v1 := app.Group("/api/v1")
	v1.Post("/events", eventHandler.CreateEvent)
}
