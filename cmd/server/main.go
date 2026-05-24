package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/AndySung320/rate-limiter/config"
	"github.com/AndySung320/rate-limiter/internal/api"
	"github.com/AndySung320/rate-limiter/internal/storage"
	"github.com/gin-gonic/gin"
)

func main() {
	cwd, _ := os.Getwd()
	log.Println("Running from:", cwd)

	rulePath := os.Getenv("RULES_PATH")
	if rulePath == "" {
		rulePath = "config/rules.yaml"
	}
	ruleStore, err := config.NewRuleStore(rulePath)
	if err != nil {
		log.Fatalf("Failed to load rate limit rules: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go ruleStore.Watch(ctx)

	// Try to initialize Redis storage
	// ✅ Redis address from environment (fallback to localhost)
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	log.Printf("Connecting to Redis at %s", redisAddr)
	redisStorage := storage.NewRedisStorage(redisAddr, "", 0)

	// Test Redis connection
	if err := redisStorage.Ping(); err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
		log.Println("Please start Redis with: docker run --name redis-rate-limiter -p 6379:6379 -d redis:alpine")
		log.Fatal("Redis is required for this rate limiter to work")
	}

	log.Println("✅ Connected to Redis")

	// Initialize handler
	handler := api.NewRateLimiterHandler(redisStorage, ruleStore)

	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		// Also check Redis health
		if err := redisStorage.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"redis":  "disconnected",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"redis":  "connected",
		})
	})

	// Rate limit check
	r.POST("/check", handler.CheckHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Starting server on :%s", port)
	r.Run(":" + port)
}
