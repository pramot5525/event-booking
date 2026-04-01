package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var ErrEventNotFound = errors.New("event not found")

type Event struct {
	ID        int64 `json:"id"`
	SeatLimit int32 `json:"seat_limit"`
}

type EventClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewEventClient(baseURL string) *EventClient {
	return &EventClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

func (c *EventClient) GetEvent(eventID int64) (*Event, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*50) * time.Millisecond)
		}
		event, err, retryable := c.doGetEvent(eventID)
		if err == nil {
			return event, nil
		}
		lastErr = err
		if !retryable {
			break
		}
	}
	return nil, lastErr
}

func (c *EventClient) doGetEvent(eventID int64) (*Event, error, bool) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/api/v1/events/%d", c.baseURL, eventID))
	if err != nil {
		return nil, fmt.Errorf("call event-service: %w", err), true
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("event %d: %w", eventID, ErrEventNotFound), false
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("event-service returned %d", resp.StatusCode), true
	}

	var event Event
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return nil, fmt.Errorf("decode event: %w", err), false
	}
	return &event, nil, false
}
