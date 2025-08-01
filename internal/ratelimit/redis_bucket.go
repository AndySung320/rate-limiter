package ratelimit

import (
	"sync"
	"time"

	"github.com/AndySung320/rate-limiter/internal/storage"
)

type RedisBucket struct {
	key        string
	capacity   int64
	refillRate int64
	storage    *storage.RedisStorage
	mutex      sync.Mutex
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

func (rb *RedisBucket) Allow(cost int) (bool, int64, error) {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	// Get current state from Redis
	state, err := rb.storage.GetBucketState(rb.key)
	if err != nil {
		return false, 0, err
	}

	now := time.Now()

	// Initialize bucket if it doesn't exist
	if state == nil {
		state = &storage.BucketState{
			Tokens:     rb.capacity,
			LastRefill: now,
			Capacity:   rb.capacity,
			RefillRate: rb.refillRate,
		}
	}

	// Refill tokens based on time elapsed
	if state.Tokens < state.Capacity {
		delta := now.Sub(state.LastRefill).Seconds()
		tokensToAdd := int64(delta * float64(state.RefillRate))

		if tokensToAdd > 0 {
			state.Tokens = min(state.Capacity, state.Tokens+tokensToAdd)
			state.LastRefill = now
		}
	}

	// Check if we have enough tokens
	allowed := int64(cost) <= state.Tokens
	if allowed {
		state.Tokens -= int64(cost)
	}

	// Save state back to Redis
	if err := rb.storage.SetBucketState(rb.key, state, rb.ttl); err != nil {
		return false, 0, err
	}

	return allowed, state.Tokens, nil
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
