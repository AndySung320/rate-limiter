package ratelimit

import (
	"sync"
	"time"
)

type TokenBucket struct {
	capacity   int64
	tokens     int64
	refillRate int64 // tokens per second
	lastRefill time.Time
	mutex      sync.Mutex
}

func NewTokenBucket(capacity, refillRate int64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) Allow(cost int) (bool, int64) {
	// Token bucket logic here
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	now := time.Now()
	if tb.tokens < tb.capacity {
		delta := now.Sub(tb.lastRefill).Seconds()
		added := int64(delta * float64(tb.refillRate))
		if added > 0 {
			tb.tokens = min(tb.capacity, tb.tokens+added)
			tb.lastRefill = now
		}
	}
	if int64(cost) <= tb.tokens {
		tb.tokens -= int64(cost)
		return true, tb.tokens
	}
	return false, tb.tokens
}
