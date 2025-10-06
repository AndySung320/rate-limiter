package storage

import (
	"context"
	"io"
	"log"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
)

// Create Mock Redis Client
type MockRedisClient struct {
	mock.Mock
}

func (m *MockRedisClient) EvalSha(ctx context.Context, sha string, keys []string, args ...interface{}) *redis.Cmd {
	mockArgs := m.Called(ctx, sha, keys, args)
	return mockArgs.Get(0).(*redis.Cmd)
}

func (m *MockRedisClient) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	mockArgs := m.Called(ctx, script)
	return mockArgs.Get(0).(*redis.StringCmd)
}

func (m *MockRedisClient) Ping(ctx context.Context) *redis.StatusCmd {
	mockArgs := m.Called(ctx)
	return mockArgs.Get(0).(*redis.StatusCmd)
}

func (m *MockRedisClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// internal/storage/redis_test.go
func TestAtomicTokenBucket_AllowsRequest(t *testing.T) {
	mockClient := new(MockRedisClient)

	storage := &RedisStorage{
		client: mockClient,
		ctx:    context.Background(),
		scripts: map[string]*ScriptInfo{
			"endpoint_only": {
				SHA: "abc123",
			},
		},
	}

	// Mock successful Redis response
	cmd := redis.NewCmd(context.Background())
	cmd.SetVal([]interface{}{int64(1), int64(90)}) // allowed=1, remaining=90

	mockClient.On("EvalSha",
		mock.Anything,
		"abc123",
		mock.Anything,
		mock.Anything,
	).Return(cmd)

	// Test
	allowed, remaining, err := storage.AtomicTokenBucket("test_key", 100, 10, 10, time.Hour)

	// Assert
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected request to be allowed")
	}
	if remaining != 90 {
		t.Errorf("expected 90 remaining, got %d", remaining)
	}

	mockClient.AssertExpectations(t)
}

func TestAtomicTokenBucket_DeniesRequest(t *testing.T) {
	mockClient := new(MockRedisClient)

	storage := &RedisStorage{
		client: mockClient,
		ctx:    context.Background(),
		scripts: map[string]*ScriptInfo{
			"endpoint_only": {SHA: "abc123"},
		},
	}

	// Mock Redis response for denied request
	cmd := redis.NewCmd(context.Background())
	cmd.SetVal([]interface{}{int64(0), int64(0)}) // allowed=0, remaining=0

	mockClient.On("EvalSha", mock.Anything, "abc123", mock.Anything, mock.Anything).Return(cmd)

	allowed, remaining, err := storage.AtomicTokenBucket("test_key", 100, 10, 10, time.Hour)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected request to be denied")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
}

func TestAtomicDualBucket_BothBucketsChecked(t *testing.T) {
	mockClient := new(MockRedisClient)

	storage := &RedisStorage{
		client: mockClient,
		ctx:    context.Background(),
		scripts: map[string]*ScriptInfo{
			"tier_endpoint": {SHA: "def456"},
		},
	}

	// Mock dual bucket success
	cmd := redis.NewCmd(context.Background())
	cmd.SetVal([]interface{}{int64(1), int64(90), int64(9990)}) // allowed, user_remaining, global_remaining

	mockClient.On("EvalSha", mock.Anything, "def456", mock.Anything, mock.Anything).Return(cmd)

	allowed, userRemaining, globalRemaining, err := storage.AtomicDualBucket(
		"user:123", "global:/api/test",
		10000, 1000, 100, 10,
		10, time.Hour,
	)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("expected request to be allowed")
	}
	if userRemaining != 90 {
		t.Errorf("expected user remaining 90, got %d", userRemaining)
	}
	if globalRemaining != 9990 {
		t.Errorf("expected global remaining 9990, got %d", globalRemaining)
	}
}

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard) // Turn off all the log when testing
	os.Exit(m.Run())
}
