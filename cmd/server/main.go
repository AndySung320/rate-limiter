package main

import (
	"log"
	"net/http"

	"github.com/AndySung320/rate-limiter/internal/api"
	"github.com/AndySung320/rate-limiter/internal/storage"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	redisStorage := storage.NewRedisStorage("localhost:6379", "", 0)
	// Test Redis connection
	if err := redisStorage.Ping(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	log.Println("Connected to Redis")

	// Initialize handler
	handler := api.NewRateLimiterHandler(redisStorage)
	// Rate limit check (暫時回固定值)
	// r.POST("/check", func(c *gin.Context) {
	// 	c.JSON(http.StatusOK, gin.H{
	// 		"allowed":   true,
	// 		"remaining": 100,
	// 	})
	// })
	r.POST("/check", handler.CheckHandler)
	log.Println("Starting server on :8080")
	r.Run(":8080") // 啟動 server
}
