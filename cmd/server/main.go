package main

import (
	"log"
	"net/http"
	"os"

	"github.com/AndySung320/rate-limiter/config"
	"github.com/AndySung320/rate-limiter/internal/api"
	"github.com/AndySung320/rate-limiter/internal/storage"
	"github.com/gin-gonic/gin"
)

func main() {
	cwd, _ := os.Getwd()
	log.Println("Running from:", cwd)
	rulSet, err := config.LoadRuleSet("config/rules.yaml")
	if err != nil {
		log.Fatalf("Failed to load rate limit rules: %v", err)
	}

	// Try to initialize Redis storage
	redisStorage := storage.NewRedisStorage("localhost:6379", "", 0)

	// Test Redis connection
	if err := redisStorage.Ping(); err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
		log.Println("Please start Redis with: docker run --name redis-rate-limiter -p 6379:6379 -d redis:alpine")
		log.Fatal("Redis is required for this rate limiter to work")
	}

	log.Println("✅ Connected to Redis")

	// Initialize handler
	handler := api.NewRateLimiterHandler(redisStorage, rulSet)

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
