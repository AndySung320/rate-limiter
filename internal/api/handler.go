package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/AndySung320/rate-limiter/config"
	"github.com/AndySung320/rate-limiter/internal/storage"
	"github.com/gin-gonic/gin"
)

type CheckRequest struct {
	Key      string `json:"key" binding:"required"`
	Endpoint string `json:"endpoint" binding:"required"`
	// Cost      int               `json:"cost" binding:"required"`
	UserTier  string            `json:"user_tier,omitempty"`  // Optional
	IPAddress string            `json:"ip_address,omitempty"` // Optional
	Metadata  map[string]string `json:"metadata,omitempty"`   // Flexible attributes
}

type CheckResponse struct {
	Allowed         bool  `json:"allowed"`
	UserRemaining   int64 `json:"userRemaining"`
	GlobalRemaining int64 `json:"globalRemaining"`
}

type RateLimiterHandler struct {
	storage storage.Storage
	rules   *config.RuleSet
}

func NewRateLimiterHandler(storage storage.Storage, rules *config.RuleSet) *RateLimiterHandler {
	return &RateLimiterHandler{
		storage: storage,
		rules:   rules,
	}
}

func (h *RateLimiterHandler) CheckHandler(c *gin.Context) {
	var req CheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ep, ok := h.rules.Endpoints[req.Endpoint]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown endpoint"})
		return
	}

	// log.Printf("DEBUG: ep = %+v", ep)
	// log.Printf("DEBUG: req.UserTier = %s", req.UserTier)
	// log.Printf("DEBUG: h.rules.Tiers = %+v", h.rules.Tiers)

	rule := ep.Rule
	globalKey := fmt.Sprintf("global:%s", req.Endpoint)
	cost := ep.Cost
	globalCapacity := h.rules.Endpoints[req.Endpoint].GlobalCapacity
	globalRefillrate := h.rules.Endpoints[req.Endpoint].GlobalRefillRate
	var allowed bool
	var userRemaining, globalRemaining int64
	var err error
	switch rule {
	case "tiers+endpoints":
		// Validate user tier exists
		tier, hasTier := h.rules.Tiers[req.UserTier]
		if !hasTier {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":       "invalid user_tier",
				"provided":    req.UserTier,
				"valid_tiers": getValidTiers(h.rules.Tiers), // Helper function
			})
			return
		}
		userKey := fmt.Sprintf("user:%s:%s:%s", req.Key, req.Endpoint, req.UserTier)
		userRefillrate := tier.RefillRate
		userCapacity := tier.Capacity
		log.Printf("user key: %s, user refill rate: %d, user capacity: %d", userKey, userRefillrate, userCapacity)
		requestID := fmt.Sprintf("%d", time.Now().UnixNano())
		log.Printf("🔄 [%s] Request START - key: %s, cost: %d", requestID, globalKey, cost)
		allowed, userRemaining, globalRemaining, err = h.storage.AtomicDualBucket(userKey, globalKey, globalCapacity, globalRefillrate, userCapacity, userRefillrate, cost, time.Hour)
		log.Printf("💾 [%s] WRITE to Redis - userTokens: %d, endpointTokens: %d, allowed: %v", requestID, userRemaining, globalRemaining, allowed)
		log.Printf("✅ Request COMPLETE - userRemaining: %d globalRemaining: %d", userRemaining, globalRemaining)

	case "IP+endpoints":
		if req.IPAddress == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ip_address required for this endpoint"})
			return
		}

		ipKey := fmt.Sprintf("ip:%s:%s", req.IPAddress, req.Endpoint)
		ipCapacity := h.rules.IPs.Capacity
		ipRefillrate := h.rules.IPs.RefillRate
		// Reuse your AtomicDualBucket with IP instead of user
		var ipRemaining int64
		allowed, ipRemaining, globalRemaining, err = h.storage.AtomicDualBucket(
			ipKey, globalKey,
			globalCapacity, globalRefillrate,
			ipCapacity, ipRefillrate, // Need to define IP limits in config
			cost, time.Hour,
		)
		requestID := fmt.Sprintf("%d", time.Now().UnixNano())
		log.Printf("🔄 [%s] Request START - key: %s, cost: %d", requestID, globalKey, cost)
		log.Printf("💾 [%s] WRITE to Redis - ipTokens: %d, endpointTokens: %d, allowed: %v", requestID, ipRemaining, globalRemaining, allowed)
		log.Printf("✅ Request COMPLETE - ipRemaining: %d globalRemaining: %d", ipRemaining, globalRemaining)

	case "endpoint":
		endpointKey := fmt.Sprintf("endpoint:%s", req.Endpoint)
		log.Printf("endPoint key: %s, endPoint refill rate: %d, global capacity: %d", endpointKey, globalRefillrate, globalCapacity)
		requestID := fmt.Sprintf("%d", time.Now().UnixNano())
		log.Printf("🔄 [%s] Request START - key: %s, cost: %d", requestID, globalKey, cost)
		allowed, globalRemaining, err = h.storage.AtomicTokenBucket(endpointKey, globalCapacity, globalRefillrate, cost, time.Hour)
		log.Printf("💾 [%s] WRITE to Redis - endPointTokens: %d, allowed: %v", requestID, globalRemaining, allowed)
		log.Printf("✅ Request COMPLETE - globalRemaining: %d", globalRemaining)
	}

	// Create bucket key (user:endpoint)
	// bucketKey := req.Key + ":" + req.Endpoint
	// Create Redis bucket with default settings
	// TODO: Make these configurable later

	// endPointBucket := ratelimit.NewRedisBucket(req.Endpoint, endPointCapacity, endPointRefillrate, h.storage)
	// userBucket := ratelimit.NewRedisBucket(bucketKey, userCapacity, userRefillrate, h.storage)
	// allowed, remaining, err := bucket.Allow(req.Cost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Rate limiter unavailable"})
		return
	}

	resp := CheckResponse{
		Allowed:         allowed,
		UserRemaining:   userRemaining,
		GlobalRemaining: globalRemaining,
	}
	log.Printf("allowed=%v, userRemaining=%d, globalRemaining=%d\n", allowed, userRemaining, globalRemaining)
	if !resp.Allowed {
		c.JSON(http.StatusTooManyRequests, resp)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func getValidTiers(tiers map[string]config.TierConfig) []string {
	var validTiers []string
	for tier := range tiers {
		validTiers = append(validTiers, tier)
	}
	return validTiers
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
