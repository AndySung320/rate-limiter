package storage

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
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
	if err := storage.LoadScript("endpoint_only", "tokenbucket.lua"); err != nil {
		log.Fatalf("❌ Failed to load script endpoint_only: %v", err)
	}
	if err := storage.LoadScript("tier_endpoint", "tokenbucket_dual.lua"); err != nil {
		log.Fatalf("❌ Failed to load script tier_endpoint: %v", err)
	}

	for name, script := range storage.scripts {
		log.Printf("✅ Script loaded: %s (SHA=%s, len=%d)", name, script.SHA, len(script.Content))
	}
	return storage
}

func (r *RedisStorage) LoadScript(name, luaScriptName string) error {
	_, file, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(file) // internal/storage
	scriptPath := filepath.Join(baseDir, luaScriptName)
	log.Printf("DEBUG: LoadScript from %s", scriptPath)
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to read lua script (%s): %w", scriptPath, err)
	}
	sha, err := r.client.ScriptLoad(r.ctx, string(content)).Result()
	if err != nil {
		return fmt.Errorf("failed to load script into redis: %w", err)
	}

	r.scripts[name] = &ScriptInfo{
		Name:     name,
		SHA:      sha,
		Content:  string(content),
		LoadedAt: time.Now(),
	}

	log.Printf("Loaded script '%s' from %s (SHA: %s)", name, scriptPath, sha)
	return nil
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
