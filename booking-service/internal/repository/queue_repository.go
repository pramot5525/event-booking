package repository

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const bookingQueueKey = "booking:queue"

type QueueRepository interface {
	Enqueue(ctx context.Context, payload string) error
	Dequeue(ctx context.Context, timeout time.Duration) (string, error)
}

type queueRepository struct {
	rdb *redis.Client
}

func NewQueueRepository(rdb *redis.Client) QueueRepository {
	return &queueRepository{rdb: rdb}
}

func (r *queueRepository) Enqueue(ctx context.Context, payload string) error {
	return r.rdb.LPush(ctx, bookingQueueKey, payload).Err()
}

func (r *queueRepository) Dequeue(ctx context.Context, timeout time.Duration) (string, error) {
	result, err := r.rdb.BRPop(ctx, timeout, bookingQueueKey).Result()
	if err != nil {
		return "", err
	}
	return result[1], nil
}
