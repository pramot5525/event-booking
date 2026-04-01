package repository

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type LockRepository interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key string) error
}

type lockRepository struct {
	rdb *redis.Client
}

func NewLockRepository(rdb *redis.Client) LockRepository {
	return &lockRepository{rdb: rdb}
}

func (r *lockRepository) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	err := r.rdb.SetArgs(ctx, key, "locked", redis.SetArgs{
		Mode: "NX",
		TTL:  ttl,
	}).Err()

	if err == redis.Nil {
		return false, nil
	}

	return err == nil, err
}

func (r *lockRepository) Release(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, key).Err()
}
