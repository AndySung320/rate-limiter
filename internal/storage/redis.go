package storage

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStorage struct {
	client  RedisClient
	ctx     context.Context
	scripts map[string]*ScriptInfo // Registry of all scripts
}

type ScriptInfo struct {
	Name     string
	SHA      string
	Content  string
	LoadedAt time.Time
}

func NewRedisStorage(addr, password string, db int) *RedisStorage {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	storage := &RedisStorage{
		client:  rdb,
		ctx:     context.Background(),
		scripts: make(map[string]*ScriptInfo),
	}
	// Load all scripts at startup
	storage.LoadScript("endpoint_only", "internal/storage/tokenbucket.lua")
	storage.LoadScript("tier_endpoint", "internal/storage/tokenbucket_dual.lua")

	return storage
}

func (r *RedisStorage) LoadScript(name, path string) error {
	content, err := loadLuaScript(path)
	if err != nil {
		log.Fatalf("Failed to load lua script: %v", err)
		return err
	}
	sha, err := r.client.ScriptLoad(r.ctx, content).Result()
	if err != nil {
		return err
	}

	r.scripts[name] = &ScriptInfo{
		Name:     name,
		SHA:      sha,
		Content:  content,
		LoadedAt: time.Now(),
	}

	log.Printf("Loaded script '%s' with SHA: %s", name, sha)
	return nil
}

func loadLuaScript(path string) (string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (r *RedisStorage) ExecuteScript(scriptName string, keys []string, args ...interface{}) (interface{}, error) {
	script, exists := r.scripts[scriptName]
	if !exists {
		return nil, fmt.Errorf("script '%s' not found", scriptName)
	}

	result, err := r.client.EvalSha(r.ctx, script.SHA, keys, args...).Result()

	if err != nil && strings.Contains(err.Error(), "NOSCRIPT") {
		// Reload and retry
		log.Printf("Reloading script '%s'...", scriptName)
		sha, err := r.client.ScriptLoad(r.ctx, r.scripts[scriptName].Content).Result()
		if err != nil {
			return nil, err
		}
		r.scripts[scriptName].SHA = sha
		log.Printf("New script SHA after reload: %s", sha)

		result, err = r.client.EvalSha(r.ctx, script.SHA, keys, args...).Result()
	}

	return result, err
}

func (r *RedisStorage) AtomicTokenBucket(key string, capacity, refillRate int64, cost int64, ttl time.Duration) (bool, int64, error) {
	now := time.Now().UnixMilli()
	result, err := r.ExecuteScript("endpoint_only",
		[]string{r.bucketKey(key)},
		capacity, refillRate, cost, now, int(ttl.Seconds()))
	values := result.([]interface{})
	allowed := values[0].(int64) == 1
	globalRemaining := values[1].(int64)
	return allowed, globalRemaining, err
}

// func (r *RedisStorage) AtomicTokenBucket(key string, capacity, refillRate int64, cost int, ttl time.Duration) (bool, int64, error) {
// 	now := time.Now().UnixMilli()
// 	// Use cached script SHA (faster than sending full script)
// 	result, err := r.client.EvalSha(r.ctx, r.scriptSHA, []string{r.bucketKey(key)},
// 		capacity, refillRate, cost, now, int(ttl.Seconds())).Result()

// 	if err != nil && strings.Contains(err.Error(), "NOSCRIPT") {
// 		// Script not loaded, reload it
// 		log.Println("Reloading Lua script after Redis restart...")
// 		sha, err := r.client.ScriptLoad(r.ctx, r.originalScript).Result()
// 		if err != nil {
// 			return false, 0, err
// 		}
// 		r.scriptSHA = sha

// 		// Retry with new SHA
// 		result, err = r.client.EvalSha(r.ctx, r.scriptSHA, []string{r.bucketKey(key)},
// 			capacity, refillRate, cost, now, int(ttl.Seconds())).Result()
// 		if err != nil {
// 			return false, 0, err
// 		}
// 		log.Printf("New script SHA after reload: %s", sha)
// 	}

// 	values := result.([]interface{})
// 	allowed := values[0].(int64) == 1
// 	remaining := values[1].(int64)

// 	return allowed, remaining, nil
// }

func (r *RedisStorage) AtomicDualBucket(userKey, globalKey string, globalCap, globalRate, userCap, userRate int64, cost int64, ttl time.Duration) (bool, int64, int64, error) {
	now := time.Now().UnixMilli()
	result, err := r.ExecuteScript("tier_endpoint",
		[]string{r.bucketKey(userKey), r.bucketKey(globalKey)},
		globalCap, globalRate, userCap, userRate, cost, now, int(ttl.Seconds()))
	values := result.([]interface{})
	allowed := values[0].(int64) == 1
	userRemaining := values[1].(int64)
	globalRemaining := values[2].(int64)
	return allowed, userRemaining, globalRemaining, err
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
