package storage

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Storage interface {
	AtomicTokenBucket(key string, capacity, refillRate int64, cost int64, ttl time.Duration) (bool, int64, error)
	AtomicDualBucket(userKey, globalKey string, globalCap, globalRate, userCap, userRate int64, cost int64, ttl time.Duration) (bool, int64, int64, error)
	Ping() error
	Close() error
}

type RedisClient interface {
	EvalSha(ctx context.Context, sha string, keys []string, args ...interface{}) *redis.Cmd
	ScriptLoad(ctx context.Context, script string) *redis.StringCmd
	Ping(ctx context.Context) *redis.StatusCmd
	Close() error
}

var _ Storage = (*RedisStorage)(nil)
var _ RedisClient = (*redis.Client)(nil)
