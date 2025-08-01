package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStorage struct {
	client *redis.Client
	ctx    context.Context
}

type BucketState struct {
	Tokens     int64     `json:"tokens"`
	LastRefill time.Time `json:"last_refill"`
	Capacity   int64     `json:"capacity"`
	RefillRate int64     `json:"refill_rate"`
}

func NewRedisStorage(addr, password string, db int) *RedisStorage {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &RedisStorage{
		client: rdb,
		ctx:    context.Background(),
	}
}

func (r *RedisStorage) GetBucketState(key string) (*BucketState, error) {
	val, err := r.client.Get(r.ctx, r.bucketKey(key)).Result()
	if err == redis.Nil {
		// Bucket doesn't exist, return nil
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket state: %w", err)
	}

	var state BucketState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bucket state: %w", err)
	}

	return &state, nil
}

func (r *RedisStorage) SetBucketState(key string, state *BucketState, ttl time.Duration) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal bucket state: %w", err)
	}

	err = r.client.Set(r.ctx, r.bucketKey(key), data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set bucket state: %w", err)
	}

	return nil
}

func (r *RedisStorage) Ping() error {
	return r.client.Ping(r.ctx).Err()
}

func (r *RedisStorage) Close() error {
	return r.client.Close()
}

func (r *RedisStorage) bucketKey(key string) string {
	return fmt.Sprintf("rate_limit:bucket:%s", key)
}
