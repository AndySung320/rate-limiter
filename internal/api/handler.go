package api

import (
	"net/http"

	"github.com/AndySung320/rate-limiter/internal/ratelimit"
	"github.com/AndySung320/rate-limiter/internal/storage"
	"github.com/gin-gonic/gin"
)

type CheckRequest struct {
	Key      string `json:"key" binding:"required"`
	Endpoint string `json:"endpoint" binding:"required"`
	Cost     int    `json:"cost" binding:"required"`
}

type CheckResponse struct {
	Allowed   bool  `json:"allowed"`
	Remaining int64 `json:"remaining"`
}

type RateLimiterHandler struct {
	storage *storage.RedisStorage
}

func NewRateLimiterHandler(storage *storage.RedisStorage) *RateLimiterHandler {
	return &RateLimiterHandler{
		storage: storage,
	}
}

func (h *RateLimiterHandler) CheckHandler(c *gin.Context) {
	var req CheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create bucket key (user:endpoint)
	bucketKey := req.Key + ":" + req.Endpoint
	// Create Redis bucket with default settings
	// TODO: Make these configurable later
	bucket := ratelimit.NewRedisBucket(bucketKey, 100, 10, h.storage)

	allowed, remaining, err := bucket.Allow(req.Cost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Rate limiter unavailable"})
		return
	}

	resp := CheckResponse{
		Allowed:   allowed,
		Remaining: remaining,
	}

	c.JSON(http.StatusOK, resp)
}

// func CheckHandler(bucket *limiter.TokenBucket) gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		var req CheckRequest
// 		if err := c.ShouldBindJSON(&req); err != nil {
// 			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 			return
// 		}

// 		allowed, remaining := bucket.Allow(req.Cost)
// 		resp := CheckResponse{
// 			Allowed:   allowed,
// 			Remaining: remaining,
// 		}
// 		c.JSON(http.StatusOK, resp)
// 	}
// }
