package db

import (
	"ollmo/ollmo/internal/config"
	"github.com/redis/go-redis/v9"
)

// NewRedis returns a Redis client. No dial here; first command will connect.
func NewRedis(cfg config.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       0,
	})
}
