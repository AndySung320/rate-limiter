// tests/integration/rate_limiter_test.go
//go:build integration
// +build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AndySung320/rate-limiter/config"
	"github.com/AndySung320/rate-limiter/internal/api"
	"github.com/AndySung320/rate-limiter/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/redis"
)

func setupRedisContainer(t *testing.T) (string, func()) {
	ctx := context.Background()

	redisContainer, err := redis.RunContainer(ctx,
		testcontainers.WithImage("redis:7-alpine"),
	)
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}

	endpoint, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("failed to get redis endpoint: %v", err)
	}

	cleanup := func() {
		if err := redisContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}

	return endpoint, cleanup
}

func TestRateLimiter_EndToEnd(t *testing.T) {
	// Start Redis
	redisAddr, cleanup := setupRedisContainer(t)
	defer cleanup()

	// Setup storage
	redisStorage := storage.NewRedisStorage(redisAddr, "", 0)
	defer redisStorage.Close()

	// Wait for Redis to be ready
	time.Sleep(100 * time.Millisecond)

	if err := redisStorage.Ping(); err != nil {
		t.Fatalf("redis not ready: %v", err)
	}
	// Load config
	rules := &config.RuleSet{
		Tiers: map[string]config.TierConfig{
			"free": {Capacity: 100, RefillRate: 10},
		},
		Endpoints: map[string]config.EndpointConfig{
			"/api/test": {
				Rule:             "tiers+endpoints",
				Cost:             10,
				GlobalCapacity:   1000,
				GlobalRefillRate: 100,
			},
		},
		IPs: config.IPConfig{
			Capacity:   500,
			RefillRate: 50,
		},
	}

	handler := api.NewRateLimiterHandler(redisStorage, rules)

	// Setup Gin
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/check", handler.CheckHandler)

	// Test 1: First request should succeed
	resp1 := makeRequest(t, router, api.CheckRequest{
		Key:      "user123",
		Endpoint: "/api/test",
		UserTier: "free",
	})

	if !resp1.Allowed {
		t.Error("first request should be allowed")
	}
	if resp1.UserRemaining != 90 {
		t.Errorf("expected 90 remaining, got %d", resp1.UserRemaining)
	}

	// Test 2: Consume all tokens
	for i := 0; i < 9; i++ {
		makeRequest(t, router, api.CheckRequest{
			Key:      "user123",
			Endpoint: "/api/test",
			UserTier: "free",
		})
	}

	// Test 3: 11th request should be denied
	resp11 := makeRequest(t, router, api.CheckRequest{
		Key:      "user123",
		Endpoint: "/api/test",
		UserTier: "free",
	})

	if resp11.Allowed {
		t.Error("request should be denied when tokens exhausted")
	}
	if resp11.UserRemaining != 0 {
		t.Errorf("expected 0 remaining, got %d", resp11.UserRemaining)
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	redisAddr, cleanup := setupRedisContainer(t)
	defer cleanup()

	redisStorage := storage.NewRedisStorage(redisAddr, "", 0)
	defer redisStorage.Close()

	time.Sleep(100 * time.Millisecond)

	rules := &config.RuleSet{
		Tiers: map[string]config.TierConfig{
			"free": {Capacity: 100, RefillRate: 10}, // 10 tokens per second
		},
		Endpoints: map[string]config.EndpointConfig{
			"/api/test": {
				Rule:             "tiers+endpoints",
				Cost:             10,
				GlobalCapacity:   1000,
				GlobalRefillRate: 100,
			},
		},
		IPs: config.IPConfig{Capacity: 500, RefillRate: 50},
	}

	handler := api.NewRateLimiterHandler(redisStorage, rules)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/check", handler.CheckHandler)

	// Consume 50 tokens
	for i := 0; i < 5; i++ {
		makeRequest(t, router, api.CheckRequest{
			Key:      "user456",
			Endpoint: "/api/test",
			UserTier: "free",
		})
	}

	// Wait 2 seconds (should refill 20 tokens)
	time.Sleep(2 * time.Second)

	resp := makeRequest(t, router, api.CheckRequest{
		Key:      "user456",
		Endpoint: "/api/test",
		UserTier: "free",
	})

	// After 5 requests: 100 - 50 = 50 remaining
	// After 2 seconds: 50 + 20 = 70 remaining
	// After this request: 70 - 10 = 60 remaining
	expectedRemaining := int64(60)

	if !resp.Allowed {
		t.Error("request should be allowed after refill")
	}

	// Allow some tolerance for timing
	if resp.UserRemaining < expectedRemaining-5 || resp.UserRemaining > expectedRemaining+5 {
		t.Errorf("expected ~%d remaining after refill, got %d", expectedRemaining, resp.UserRemaining)
	}
}

func TestRateLimiter_ConcurrentRequests(t *testing.T) {
	redisAddr, cleanup := setupRedisContainer(t)
	defer cleanup()

	redisStorage := storage.NewRedisStorage(redisAddr, "", 0)
	defer redisStorage.Close()

	time.Sleep(100 * time.Millisecond)

	rules := &config.RuleSet{
		Tiers: map[string]config.TierConfig{
			"free": {Capacity: 100, RefillRate: 10},
		},
		Endpoints: map[string]config.EndpointConfig{
			"/api/test": {
				Rule:             "tiers+endpoints",
				Cost:             10,
				GlobalCapacity:   10000,
				GlobalRefillRate: 1000,
			},
		},
		IPs: config.IPConfig{Capacity: 500, RefillRate: 50},
	}

	handler := api.NewRateLimiterHandler(redisStorage, rules)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/check", handler.CheckHandler)

	// Send 10 concurrent requests
	results := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			resp := makeRequest(t, router, api.CheckRequest{
				Key:      "user_concurrent",
				Endpoint: "/api/test",
				UserTier: "free",
			})
			results <- resp.Allowed
		}()
	}

	// Collect results
	allowedCount := 0
	for i := 0; i < 10; i++ {
		if <-results {
			allowedCount++
		}
	}

	// Should allow exactly 10 requests (100 tokens / 10 cost)
	if allowedCount != 10 {
		t.Errorf("expected 10 allowed requests, got %d", allowedCount)
	}

	// 11th request should be denied
	resp := makeRequest(t, router, api.CheckRequest{
		Key:      "user_concurrent",
		Endpoint: "/api/test",
		UserTier: "free",
	})

	if resp.Allowed {
		t.Error("11th request should be denied")
	}
}

func TestRateLimiter_MultipleInstances(t *testing.T) {
	redisAddr, cleanup := setupRedisContainer(t)
	defer cleanup()

	// Create two separate storage instances (simulating two app instances)
	storage1 := storage.NewRedisStorage(redisAddr, "", 0)
	defer storage1.Close()

	storage2 := storage.NewRedisStorage(redisAddr, "", 0)
	defer storage2.Close()

	time.Sleep(100 * time.Millisecond)

	rules := &config.RuleSet{
		Tiers: map[string]config.TierConfig{
			"free": {Capacity: 100, RefillRate: 10},
		},
		Endpoints: map[string]config.EndpointConfig{
			"/api/test": {
				Rule:             "tiers+endpoints",
				Cost:             10,
				GlobalCapacity:   10000,
				GlobalRefillRate: 1000,
			},
		},
		IPs: config.IPConfig{Capacity: 500, RefillRate: 50},
	}

	handler1 := api.NewRateLimiterHandler(storage1, rules)
	handler2 := api.NewRateLimiterHandler(storage2, rules)

	gin.SetMode(gin.TestMode)
	router1 := gin.New()
	router1.POST("/check", handler1.CheckHandler)

	router2 := gin.New()
	router2.POST("/check", handler2.CheckHandler)

	// Request to instance 1
	resp1 := makeRequest(t, router1, api.CheckRequest{
		Key:      "user_multi",
		Endpoint: "/api/test",
		UserTier: "free",
	})

	if !resp1.Allowed || resp1.UserRemaining != 90 {
		t.Errorf("instance 1: expected allowed with 90 remaining, got allowed=%v remaining=%d", resp1.Allowed, resp1.UserRemaining)
	}

	// Request to instance 2 (should see state from instance 1)
	resp2 := makeRequest(t, router2, api.CheckRequest{
		Key:      "user_multi",
		Endpoint: "/api/test",
		UserTier: "free",
	})

	if !resp2.Allowed || resp2.UserRemaining != 80 {
		t.Errorf("instance 2: expected allowed with 80 remaining, got allowed=%v remaining=%d", resp2.Allowed, resp2.UserRemaining)
	}
}

func makeRequest(t *testing.T, router *gin.Engine, req api.CheckRequest) api.CheckResponse {
	body, _ := json.Marshal(req)

	w := httptest.NewRecorder()
	httpReq, _ := http.NewRequest(http.MethodPost, "/check", bytes.NewBuffer(body))
	httpReq.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, httpReq)

	var resp api.CheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	return resp
}

// func TestMain(m *testing.M) {
// 	log.SetOutput(io.Discard) // Turn off all the log when testing
// 	os.Exit(m.Run())
// }
