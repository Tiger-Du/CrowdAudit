package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

func NewClientFromURL(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL) // supports redis:// and rediss://
	if err != nil {
		return nil, err
	}

	// Conservative defaults
	opt.ReadTimeout = 500 * time.Millisecond
	opt.WriteTimeout = 500 * time.Millisecond
	opt.DialTimeout = 500 * time.Millisecond
	opt.PoolSize = 50

	rdb := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}
	return rdb, nil
}
