package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func bookingQueueKey(eventID int64) string {
	return fmt.Sprintf("booking:queue:%d", eventID)
}

type QueueRepository interface {
	Enqueue(ctx context.Context, eventID int64, payload string) error
	Dequeue(ctx context.Context, eventID int64, timeout time.Duration) (string, error)
}

type queueRepository struct {
	rdb *redis.Client
}

func NewQueueRepository(rdb *redis.Client) QueueRepository {
	return &queueRepository{rdb: rdb}
}

func (r *queueRepository) Enqueue(ctx context.Context, eventID int64, payload string) error {
	return r.rdb.LPush(ctx, bookingQueueKey(eventID), payload).Err()
}

func (r *queueRepository) Dequeue(ctx context.Context, eventID int64, timeout time.Duration) (string, error) {
	result, err := r.rdb.BRPop(ctx, timeout, bookingQueueKey(eventID)).Result()
	if err != nil {
		return "", err
	}
	return result[1], nil
}
