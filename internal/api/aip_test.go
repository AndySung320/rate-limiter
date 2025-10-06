package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/AndySung320/rate-limiter/config"
	"github.com/AndySung320/rate-limiter/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
)

// Mock the storage interface
type MockRedisStorage struct {
	mock.Mock
}

func (m *MockRedisStorage) AtomicTokenBucket(key string, capacity, refillRate int64, cost int64, ttl time.Duration) (bool, int64, error) {
	args := m.Called(key, capacity, refillRate, cost, ttl)
	return args.Bool(0), args.Get(1).(int64), args.Error(2)
}

func (m *MockRedisStorage) AtomicDualBucket(userKey, globalKey string, globalCap, globalRate, userCap, userRate int64, cost int64, ttl time.Duration) (bool, int64, int64, error) {
	args := m.Called(userKey, globalKey, globalCap, globalRate, userCap, userRate, cost, ttl)
	return args.Bool(0), args.Get(1).(int64), args.Get(2).(int64), args.Error(3)
}

func (m *MockRedisStorage) Ping() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRedisStorage) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestTierValidation(t *testing.T) {
	// Setup mock rules
	mockRules := &config.RuleSet{
		Tiers: map[string]config.TierConfig{
			"free": {
				Capacity:   100,
				RefillRate: 10,
			},
			"premium": {
				Capacity:   1000,
				RefillRate: 100,
			},
		},
		Endpoints: map[string]config.EndpointConfig{
			"/api/upload": {
				Rule:             "tiers+endpoints",
				Cost:             10,
				GlobalCapacity:   10000,
				GlobalRefillRate: 2000,
			},
		},
		IPs: config.IPConfig{
			Capacity:   500,
			RefillRate: 50,
		},
	}

	// Setup mock Redis storage
	mockStorage := new(MockRedisStorage)
	// Setup mock expectations for valid requests
	mockStorage.On("AtomicDualBucket",
		mock.Anything, mock.Anything,
		mock.Anything, mock.Anything,
		mock.Anything, mock.Anything,
		mock.Anything, mock.Anything,
	).Return(true, int64(90), int64(9990), nil)

	handler := NewRateLimiterHandler(mockStorage, mockRules)

	tests := []struct {
		name           string
		requestBody    CheckRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name: "valid free tier",
			requestBody: CheckRequest{
				Key:      "user123",
				Endpoint: "/api/upload",
				UserTier: "free",
			},
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name: "valid premium tier",
			requestBody: CheckRequest{
				Key:      "user456",
				Endpoint: "/api/upload",
				UserTier: "premium",
			},
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name: "invalid tier",
			requestBody: CheckRequest{
				Key:      "user789",
				Endpoint: "/api/upload",
				UserTier: "enterprise", // doesn't exist
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid user_tier",
		},
		{
			name: "empty tier",
			requestBody: CheckRequest{
				Key:      "user999",
				Endpoint: "/api/upload",
				UserTier: "",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid user_tier",
		},
		{
			name: "unknown endpoint",
			requestBody: CheckRequest{
				Key:      "user111",
				Endpoint: "/api/unknown",
				UserTier: "free",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "unknown endpoint",
		},
		{
			name: "missing key",
			requestBody: CheckRequest{
				Endpoint: "/api/upload",
				UserTier: "free",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "", // Gin validation error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Gin test context
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// Prepare request body
			body, _ := json.Marshal(tt.requestBody)
			c.Request, _ = http.NewRequest(http.MethodPost, "/check", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			// Call handler
			handler.CheckHandler(c)

			// Assert status code
			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Assert error message if expected
			if tt.expectedError != "" {
				var response map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &response)

				if errorMsg, ok := response["error"].(string); ok {
					if errorMsg != tt.expectedError && !contains(errorMsg, tt.expectedError) {
						t.Errorf("expected error containing '%s', got '%s'", tt.expectedError, errorMsg)
					}
				} else {
					t.Errorf("expected error field in response")
				}
			}

			// For valid requests, check response structure
			if tt.expectedStatus == http.StatusOK {
				var response CheckResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Errorf("failed to parse response: %v", err)
				}
				// Response should have allowed, userRemaining, globalRemaining fields
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}

// Helper to test valid tiers are returned in error
func TestInvalidTierErrorMessage(t *testing.T) {
	mockRules := &config.RuleSet{
		Tiers: map[string]config.TierConfig{
			"free":    {Capacity: 100, RefillRate: 10},
			"premium": {Capacity: 1000, RefillRate: 100},
		},
		Endpoints: map[string]config.EndpointConfig{
			"/api/upload": {
				Rule:             "tiers+endpoints",
				Cost:             10,
				GlobalCapacity:   10000,
				GlobalRefillRate: 2000,
			},
		},
	}

	mockStorage := &storage.RedisStorage{}
	handler := NewRateLimiterHandler(mockStorage, mockRules)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	requestBody := CheckRequest{
		Key:      "user123",
		Endpoint: "/api/upload",
		UserTier: "invalid_tier",
	}

	body, _ := json.Marshal(requestBody)
	c.Request, _ = http.NewRequest(http.MethodPost, "/check", bytes.NewBuffer(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CheckHandler(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	// Check that valid_tiers is present in response
	if validTiers, ok := response["valid_tiers"].([]interface{}); ok {
		if len(validTiers) != 2 {
			t.Errorf("expected 2 valid tiers, got %d", len(validTiers))
		}
	} else {
		t.Error("expected valid_tiers field in error response")
	}
}

func TestCheckHandler_StatusCodes(t *testing.T) {
	// Setup mock rules
	mockRules := &config.RuleSet{
		Tiers: map[string]config.TierConfig{
			"free": {
				Capacity:   100,
				RefillRate: 10,
			},
			"premium": {
				Capacity:   1000,
				RefillRate: 100,
			},
		},
		Endpoints: map[string]config.EndpointConfig{
			"/api/upload": {
				Rule:             "tiers+endpoints",
				Cost:             10,
				GlobalCapacity:   10000,
				GlobalRefillRate: 2000,
			},
		},
		IPs: config.IPConfig{
			Capacity:   500,
			RefillRate: 50,
		},
	}

	tests := []struct {
		name           string
		allowed        bool
		err            error
		expectedStatus int
	}{
		{"allowed request returns 200", true, nil, http.StatusOK},
		{"rate limited request returns 429", false, nil, http.StatusTooManyRequests},
		{"storage error returns 500", false, errors.New("Rate limiter unavailable"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// build mockStorage
			mockStorage := new(MockRedisStorage)

			// assign allowed value
			mockStorage.On("AtomicDualBucket",
				mock.Anything, mock.Anything,
				mock.Anything, mock.Anything,
				mock.Anything, mock.Anything,
				mock.Anything, mock.Anything,
			).Return(tt.allowed, int64(90), int64(9990), tt.err)

			mockStorage.On("Ping").Return(nil)
			mockStorage.On("Close").Return(nil)

			handler := NewRateLimiterHandler(mockStorage, mockRules)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			req := CheckRequest{
				Key:      "user123",
				Endpoint: "/api/upload",
				UserTier: "free",
			}
			body, _ := json.Marshal(req)
			c.Request, _ = http.NewRequest(http.MethodPost, "/check", bytes.NewBuffer(body))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.CheckHandler(c)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard) // Turn off all the log when testing
	os.Exit(m.Run())
}
