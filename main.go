package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()
var rdb *redis.Client

func init() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
}

func rateLimiterMiddleware(limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		key := "rate_limit:" + clientIP

		// Sliding window implementation using Redis ZSET
		now := time.Now().UnixMilli()
		windowStart := now - window.Milliseconds()

		pipe := rdb.TxPipeline()
		// Remove older entries
		pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
		// Add current request
		// Member needs to be unique for each request. Appending a random string or just using the current nano time is better.
		nowNano := time.Now().UnixNano()
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d", nowNano)})
		// Get number of requests in current window
		reqCountInfo := pipe.ZCard(ctx, key)
		// Expire the key to prevent memory leaks
		pipe.Expire(ctx, key, window)

		_, err := pipe.Exec(ctx)
		if err != nil {
			log.Printf("Redis error: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
			return
		}

		count := reqCountInfo.Val()

		if count > int64(limit) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
			})
			return
		}

		c.Next()
	}
}

func main() {
	r := gin.Default()

	// Apply Rate Limiter middleware: max 5 requests per 10 seconds
	r.Use(rateLimiterMiddleware(5, 10*time.Second))

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.GET("/api/data", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"data": "Secure payment data via Stripe Simulation",
		})
	})

	log.Println("Starting Server on :8080...")
	if err := r.Run(":8080"); err != nil {
		log.Fatal("Server failed to start ", err)
	}
}
