// Package redisx opens the Redis client.
package redisx

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// Connect opens a client from a redis:// URL and checks that Redis answers.
func Connect(ctx context.Context, url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, err
	}
	return rdb, nil
}
