# Distributed API Rate Limiter

A high-performance, distributed API Rate Limiter middleware written in **Go** and backed by **Redis**. This project demonstrates a production-ready **Sliding Window** rate limiting algorithm to prevent API abuse and handle high throughput efficiently across distributed nodes.

## Features
- **Sliding Window Algorithm**: Smooth rate limiting without the edge-case spikes of Fixed Window algorithms.
- **Distributed State**: Uses Redis as the single source of truth, meaning multiple API server nodes share the same rate limit state.
- **Concurrency Safe**: Utilizes Redis atomic transactions (`TxPipeline`) to ensure no race conditions under heavy concurrent load.
- **High Performance**: Built with Go and the Gin Web Framework.

## Tech Stack
- **Language:** Go (Golang)
- **Framework:** Gin (`github.com/gin-gonic/gin`)
- **Datastore:** Redis (`github.com/redis/go-redis/v9`)

## Prerequisites
- Go 1.22+ installed
- Redis server running on `localhost:6379`

## How to Run

1. **Start Redis:**
   ```bash
   docker run -d --name redis-server -p 6379:6379 redis:alpine
   ```

2. **Run the Go Server:**
   ```bash
   go mod tidy
   go run main.go
   ```
   The server will start on `http://localhost:8080`.

## Testing the Rate Limiter

The application has a configured limit of **5 requests per 10 seconds** per IP address.

Run the following curl command simulator to blast the server with 7 quick requests:
```bash
for i in {1..7}; do curl -i http://localhost:8080/ping; done
```

**Expected Output Screenshot/Log:**
```text
HTTP/1.1 200 OK
{"message":"pong"}
HTTP/1.1 200 OK
{"message":"pong"}
HTTP/1.1 200 OK
{"message":"pong"}
HTTP/1.1 200 OK
{"message":"pong"}
HTTP/1.1 200 OK
{"message":"pong"}
HTTP/1.1 429 Too Many Requests
{"error":"Rate limit exceeded"}
HTTP/1.1 429 Too Many Requests
{"error":"Rate limit exceeded"}
```
*Notice how exactly 5 requests succeed (200 OK) and the subsequent ones in the same time window are rejected (429 Too Many Requests).*

## Architecture Highlights
- `ZAdd` combined with Unix timestamps in microseconds acts as unique members for the Sorted Set.
- `ZRemRangeByScore` clears out hits older than the `time.Now() - windowSize`.
- `ZCard` accurately measures the window hit count.
- `TxPipeline` packages this into one rapid round-trip to Redis.
