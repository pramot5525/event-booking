package repository

import (
	"event-service/internal/model"

	"gorm.io/gorm"
)

type EventRepository interface {
	GetAll() ([]*model.Event, error)
	GetByID(id int64) (*model.Event, error)
	Create(event *model.Event) (*int64, error)
	Update(event *model.Event) error
	Delete(id int64) error
}

type eventRepository struct {
	db *gorm.DB
}

func NewEventRepository(db *gorm.DB) *eventRepository {
	return &eventRepository{db: db}
}

func (r *eventRepository) GetAll() ([]*model.Event, error) {
	var events []*model.Event
	err := r.db.Order("id DESC").Find(&events).Error
	return events, err
}

func (r *eventRepository) GetByID(id int64) (*model.Event, error) {
	var event model.Event
	err := r.db.First(&event, id).Error
	return &event, err
}

func (r *eventRepository) Create(event *model.Event) (*int64, error) {
	err := r.db.Create(event).Error
	if err != nil {
		return nil, err
	}
	return &event.ID, err
}

func (r *eventRepository) Update(event *model.Event) error {
	return r.db.Save(event).Error
}

func (r *eventRepository) Delete(id int64) error {
	return r.db.Delete(&model.Event{}, id).Error
}
