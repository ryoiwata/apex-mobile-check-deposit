package db

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient parses the given URL, creates a Redis client, and verifies
// connectivity via Ping. Returns the client ready for use or an error with context.
func NewRedisClient(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("db: parsing redis URL: %w", err)
	}

	rdb := redis.NewClient(opt)

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("db: pinging redis: %w", err)
	}

	return rdb, nil
}
