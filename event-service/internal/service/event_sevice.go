package service

import (
	"event-service/internal/model"
	"event-service/internal/repository"
)

type EventService interface {
	GetEvent(id int64) (*model.Event, error)
	GetEvents() ([]*model.Event, error)
	CreateEvent(event *model.Event) (*int64, error)
	UpdateEvent(event *model.Event) error
	DeleteEvent(id int64) error
}

type eventService struct {
	eventRepo repository.EventRepository
}

func NewEventService(eventRepo repository.EventRepository) *eventService {
	return &eventService{eventRepo: eventRepo}
}

func (s *eventService) GetEvent(id int64) (*model.Event, error) {
	return s.eventRepo.GetByID(id)
}

func (s *eventService) GetEvents() ([]*model.Event, error) {
	return s.eventRepo.GetAll()
}

func (s *eventService) CreateEvent(event *model.Event) (*int64, error) {
	id, err := s.eventRepo.Create(event)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (s *eventService) UpdateEvent(event *model.Event) error {
	return s.eventRepo.Update(event)
}

func (s *eventService) DeleteEvent(id int64) error {
	return s.eventRepo.Delete(id)
}
