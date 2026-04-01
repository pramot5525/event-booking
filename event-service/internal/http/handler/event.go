package handler

import (
	"errors"
	"event-service/internal/model"
	"event-service/internal/service"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type eventHandler struct {
	eventService service.EventService
}

func NewEventHandler(eventService service.EventService) *eventHandler {
	return &eventHandler{eventService: eventService}
}

func (h *eventHandler) GetEvent(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	event, err := h.eventService.GetEvent(int64(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(event)
}

func (h *eventHandler) GetEvents(c *fiber.Ctx) error {
	events, err := h.eventService.GetEvents()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(events)
}

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

func (h *eventHandler) UpdateEvent(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	event := new(model.Event)
	if err := c.BodyParser(event); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	event.ID = int64(id)

	if err := h.eventService.UpdateEvent(event); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "updated successfully"})
}

func (h *eventHandler) DeleteEvent(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	err = h.eventService.DeleteEvent(int64(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Event deleted successfully"})
}
