package handler

import (
	"event-service/internal/model"
	"event-service/internal/service"

	"github.com/gofiber/fiber/v2"
)

type eventHandler struct {
	eventService service.EventService
}

func NewEventHandler(eventService service.EventService) *eventHandler {
	return &eventHandler{eventService: eventService}
}

// func (h *eventHandler) GetEvent(id int64) (*model.Event, error) {
// 	return h.eventService.GetEvent(id)
// }

// func (h *eventHandler) GetEvents() ([]*model.Event, error) {
// 	return h.eventService.GetEvents()
// }

func (h *eventHandler) CreateEvent(c *fiber.Ctx) error {
	event := new(model.Event)
	if err := c.BodyParser(event); err != nil {
		return err
	}
	id, err := h.eventService.CreateEvent(event)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(
		fiber.Map{
			"id": id,
		},
	)
}
