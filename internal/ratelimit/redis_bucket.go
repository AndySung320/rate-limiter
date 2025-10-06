package ratelimit

import (
	"time"

	"github.com/AndySung320/rate-limiter/internal/storage"
)

type RedisBucket struct {
	key        string
	capacity   int64
	refillRate int64
	storage    *storage.RedisStorage
	ttl        time.Duration
}

func NewRedisBucket(key string, capacity, refillRate int64, storage *storage.RedisStorage) *RedisBucket {
	return &RedisBucket{
		key:        key,
		capacity:   capacity,
		refillRate: refillRate,
		storage:    storage,
		ttl:        time.Hour, // Bucket expires after 1 hour of inactivity
	}
}

// func (rb *RedisBucket) Allow(cost int) (bool, int64, error) {
// 	requestID := fmt.Sprintf("%d", time.Now().UnixNano())
// 	log.Printf("🔄 [%s] Request START - key: %s, cost: %d", requestID, rb.key, cost)
// 	allowed, remaining, err := rb.storage.AtomicTokenBucket(rb.key, rb.capacity, rb.refillRate, cost, rb.ttl)
// 	if err != nil {

// 		return false, 0, err
// 	}

// 	log.Printf("💾 [%s] WRITE to Redis - tokens: %d, allowed: %v", requestID, remaining, allowed)
// 	log.Printf("✅ Request COMPLETE - remaining: %d", remaining)
// 	return allowed, remaining, nil
// }

// func min(a, b int64) int64 {
// 	if a < b {
// 		return a
// 	}
// 	return b
// }
