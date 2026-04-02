package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type EventClient interface {
	GetEventSeatLimit(ctx context.Context, eventID uint) (int64, error)
}

type eventClient struct {
	baseURL string
	client  *http.Client
}

type eventDTO struct {
	ID        uint  `json:"id"`
	SeatLimit int64 `json:"seat_limit"`
}

func NewEventClient(baseURL string) EventClient {
	return &eventClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *eventClient) GetEventSeatLimit(ctx context.Context, eventID uint) (int64, error) {
	url := fmt.Sprintf("%s/api/v1/events/%d", c.baseURL, eventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("new request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request event-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("event-service returned status %d", resp.StatusCode)
	}

	var event eventDTO
	if err := json.NewDecoder(resp.Body).Decode(&event); err != nil {
		return 0, fmt.Errorf("decode event response: %w", err)
	}

	if event.SeatLimit < 0 {
		return 0, fmt.Errorf("invalid seat_limit %d", event.SeatLimit)
	}

	return event.SeatLimit, nil
}
