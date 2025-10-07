# Distributed Rate Limiter

A high-performance distributed rate limiting service built with Go, Redis, and Lua scripts. Designed for multi-instance deployment with atomic multi-bucket enforcement and configuration-driven rules.

## 🚀 Features

- **Atomic Multi-Bucket Rate Limiting**: Enforces multiple rate limit buckets (user tier, global endpoint, IP-based) in a single atomic operation using Redis Lua scripts
- **Configuration-Driven**: YAML-based configuration for flexible rate limiting rules without code changes
- **Distributed Architecture**: Multiple service instances coordinate through shared Redis state
- **High Performance**: 8,300+ requests/sec with p99 latency under 30ms
- **Production-Ready**: Comprehensive test coverage including unit and integration tests
- **Multiple Rate Limiting Strategies**:
  - User tier-based limits (free, premium, etc.)
  - Global endpoint protection
  - IP-based rate limiting
  - Endpoint-only limiting

## 📊 Performance

**Benchmark Results:**
- **Throughput**: 8,300 requests/second
- **Latency**: 
  - p50: 11ms
  - p95: 18ms
  - p99: 30ms
- **Concurrency**: Successfully handles 100 concurrent connections
- **Reliability**: Processes 100,000 requests without failures

**Test Environment:**
- Go 1.21
- Redis 7-alpine
- Single machine deployment

## 🏗️ Architecture
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│  Instance 1 │────▶│             │
│             │     │  (Port 8080)│     │             │
└─────────────┘     └─────────────┘     │             │
│    Redis    │
┌─────────────┐     ┌─────────────┐     │   (Shared   │
│   Client    │────▶│  Instance 2 │────▶│    State)   │
│             │     │  (Port 8081)│     │             │
└─────────────┘     └─────────────┘     └─────────────┘
│
▼
┌─────────────────────┐
│   Lua Scripts       │
│   (Atomic Ops)      │
│                     │
│ • Single Bucket     │
│ • Dual Bucket       │
│ • Token Refill      │
└─────────────────────┘
**Key Components:**
1. **API Handler**: Validates requests and routes to appropriate rate limiting strategy
2. **Storage Layer**: Manages Redis connections and Lua script execution
3. **Lua Scripts**: Ensures atomic operations across multiple buckets
4. **Configuration**: YAML-based rules for flexible rate limit definitions

## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- Redis 7+
- Docker & Docker Compose (optional)

### Using Docker Compose (Recommended)
```bash
# Start Redis and rate limiter
docker-compose up

# In another terminal, test the service
curl -X POST http://localhost:8080/check \
  -H "Content-Type: application/json" \
  -d '{
    "key": "user123",
    "endpoint": "/api/upload",
    "user_tier": "premium"
  }'
```
# Manual Setup
```bash
# 1. Start Redis
docker run -d -p 6379:6379 redis:7-alpine

# 2. Clone and build
git clone <repo>
cd rate-limiter
go mod download
go build -o rate-limiter ./cmd/server

# 3. Run the service
./rate-limiter

# 4. Test
curl -X POST http://localhost:8080/check \
  -H "Content-Type: application/json" \
  -d '{"key": "user123", "endpoint": "/api/upload", "user_tier": "free"}'
  ```
# ⚙️ Configuration
## Example Configuration `(config/rules.yaml)`
```bash
# User tier definitions
tiers:
  free:
    capacity: 100        # Total tokens available
    refill_rate: 10      # Tokens refilled per second
  premium:
    capacity: 1000
    refill_rate: 100
  enterprise:
    capacity: 10000
    refill_rate: 1000

# IP-based rate limiting
ips:
  capacity: 500
  refill_rate: 50

# Endpoint-specific rules
endpoints:
  /api/upload:
    rule: tiers+endpoints         # Check both user tier and global endpoint
    cost: 10                       # Tokens consumed per request
    global_capacity: 10000         # Global endpoint capacity
    global_refill_rate: 2000       # Global refill rate
  
  /api/download:
    rule: tiers+endpoints
    cost: 10
    global_capacity: 10000
    global_refill_rate: 2000
  
  /api/ping:
    rule: IP+endpoints             # Check IP and global endpoint
    cost: 1
    global_capacity: 5000
    global_refill_rate: 1000
  
  /api/list:
    rule: endpoint                 # Check only global endpoint
    cost: 10
    global_capacity: 10000
    global_refill_rate: 1000
```

# Rate Limiting Rules

* `tiers+endpoints`: Enforces both user tier limits and global endpoint limits
* `IP+endpoints`: Enforces IP-based limits and global endpoint limits
* `endpoint`: Enforces only global endpoint limits

# Project Structure
```
rate-limiter/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── handler.go           # HTTP handlers
│   │   └── handler_test.go      # Handler tests
│   └── storage/
│       ├── redis.go             # Redis storage implementation
│       ├── interface.go         # Storage interface
│       ├── tokenbucket.lua      # Single bucket Lua script
│       ├── tokenbucket_dual.lua # Dual bucket Lua script
│       └── redis_test.go        # Storage tests
├── config/
│   ├── rules.go                 # Configuration structures
│   ├── rules.yaml               # Rate limit rules
│   └── rules_test.go            # Config tests
├── tests/
│   └── integration/
│       └── rate_limiter_test.go # Integration tests
├── docker-compose.yml
├── Dockerfile
└── README.md
```
# Testing
## Run Unit Tests
```bash
go test ./...
```

## Run Integration Tests
```bash
# Requires Docker for testcontainers
go test -tags=integration ./tests/integration/
```

## Run All Tests
```bash
go test -tags=integration -v ./...
```
## 🛠️ Built With

| Technology | Description |
|-------------|--------------|
| **Go** | Core programming language used for backend and concurrency control. |
| **Gin** | Lightweight web framework for building RESTful APIs. |
| **Redis** | In-memory data store used for token bucket state and Lua script execution. |
| **go-redis** | Official Go client for Redis, used for atomic script evaluation and connection pooling. |
| **Lua Scripts** | Used for atomic rate limiting operations directly inside Redis. |
| **Docker / Docker Compose** | Containerized environment for local development and multi-instance testing. |
| **testcontainers-go** | Used for integration testing with ephemeral Redis containers. |
| **ApacheBench (ab)** | Used for local performance benchmarking and throughput testing. |

---

## 📝 Future Enhancements

These are planned features and areas for future improvement to make the rate limiter more robust and production-ready:

- **Configuration hot-reloading** — Support live reloading of `rules.yaml` without restarting the service, using file watchers.
- **Prometheus metrics integration** — Expose key metrics such as requests per second, allowed/denied counts, and Redis latency for monitoring.
- **Admin API for runtime updates** — Provide a management endpoint to dynamically update tier or endpoint rules via API calls.
- **Distributed tracing support** — Integrate with OpenTelemetry for end-to-end request visibility and latency breakdown.
- **Rate limit quota dashboard** — Build a lightweight UI to visualize per-user and per-endpoint usage over time.
- **Custom Lua script support** — Allow users to specify their own Lua scripts via configuration for advanced rate-limiting strategies.
- **gRPC API support** — Extend the `/check` endpoint to support gRPC clients for low-latency inter-service communication.
