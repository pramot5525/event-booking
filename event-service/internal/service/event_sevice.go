package service

import (
	"context"
	"encoding/json"
	"event-service/internal/model"
	"event-service/internal/repository"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const keyAllEvents = "events:all"

func eventKey(id int64) string {
	return fmt.Sprintf("event:%d", id)
}

type EventService interface {
	GetEvent(id int64) (*model.Event, error)
	GetEvents() ([]*model.Event, error)
	CreateEvent(event *model.Event) (*int64, error)
	UpdateEvent(event *model.Event) error
	DeleteEvent(id int64) error
}

type eventService struct {
	eventRepo repository.EventRepository
	rdb       *redis.Client
	ttl       time.Duration
	sfg       singleflight.Group
}

func NewEventService(eventRepo repository.EventRepository, rdb *redis.Client, ttl time.Duration) *eventService {
	return &eventService{eventRepo: eventRepo, rdb: rdb, ttl: ttl}
}

func (s *eventService) GetEvent(id int64) (*model.Event, error) {
	key := eventKey(id)

	if s.rdb != nil {
		if cached, err := s.rdb.Get(context.Background(), key).Bytes(); err == nil {
			log.Printf("cache hit key=%s", key)
			var event model.Event
			if err := json.Unmarshal(cached, &event); err == nil {
				return &event, nil
			}
			log.Printf("cache unmarshal failed key=%s: %v", key, err)
		} else if err == redis.Nil {
			log.Printf("cache miss key=%s", key)
		} else {
			log.Printf("cache get error key=%s: %v", key, err)
		}
	}

	v, err, _ := s.sfg.Do(key, func() (any, error) {
		event, err := s.eventRepo.GetByID(id)
		if err != nil {
			return nil, err
		}
		if s.rdb != nil {
			if b, err := json.Marshal(event); err == nil {
				if err := s.rdb.Set(context.Background(), key, b, s.ttl).Err(); err != nil {
					log.Printf("cache set error key=%s: %v", key, err)
				} else {
					log.Printf("cache set success key=%s ttl=%s", key, s.ttl)
				}
			}
		}
		return event, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*model.Event), nil
}

func (s *eventService) GetEvents() ([]*model.Event, error) {
	if s.rdb != nil {
		if cached, err := s.rdb.Get(context.Background(), keyAllEvents).Bytes(); err == nil {
			log.Printf("cache hit key=%s", keyAllEvents)
			var events []*model.Event
			if err := json.Unmarshal(cached, &events); err == nil {
				return events, nil
			}
			log.Printf("cache unmarshal failed key=%s: %v", keyAllEvents, err)
		} else if err == redis.Nil {
			log.Printf("cache miss key=%s", keyAllEvents)
		} else {
			log.Printf("cache get error key=%s: %v", keyAllEvents, err)
		}
	}

	v, err, _ := s.sfg.Do(keyAllEvents, func() (any, error) {
		events, err := s.eventRepo.GetAll()
		if err != nil {
			return nil, err
		}
		if s.rdb != nil {
			if b, err := json.Marshal(events); err == nil {
				if err := s.rdb.Set(context.Background(), keyAllEvents, b, s.ttl).Err(); err != nil {
					log.Printf("cache set error key=%s: %v", keyAllEvents, err)
				} else {
					log.Printf("cache set success key=%s ttl=%s", keyAllEvents, s.ttl)
				}
			}
		}
		return events, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]*model.Event), nil
}

func (s *eventService) CreateEvent(event *model.Event) (*int64, error) {
	id, err := s.eventRepo.Create(event)
	if err != nil {
		return nil, err
	}
	s.invalidate(keyAllEvents)
	return id, nil
}

func (s *eventService) UpdateEvent(event *model.Event) error {
	if err := s.eventRepo.Update(event); err != nil {
		return err
	}
	s.invalidate(eventKey(event.ID), keyAllEvents)
	return nil
}

func (s *eventService) DeleteEvent(id int64) error {
	if err := s.eventRepo.Delete(id); err != nil {
		return err
	}
	s.invalidate(eventKey(id), keyAllEvents)
	return nil
}

func (s *eventService) invalidate(keys ...string) {
	if s.rdb == nil {
		return
	}
	if err := s.rdb.Del(context.Background(), keys...).Err(); err != nil {
		log.Printf("cache invalidate %v: %v", keys, err)
	}
}
