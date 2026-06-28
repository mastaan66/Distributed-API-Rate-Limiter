package ratelimit

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisLimiterSlidingWindow(t *testing.T) {
	client := integrationRedis(t)
	prefix := integrationPrefix(t)
	key := "sliding-window"
	redisKey := prefix + ":" + key
	t.Cleanup(func() {
		_ = client.Del(context.Background(), redisKey).Err()
	})

	limiter, err := NewRedisLimiter(
		client,
		WithPrefix(prefix),
		WithKeyEncoder(func(value string) string { return value }),
	)
	if err != nil {
		t.Fatalf("NewRedisLimiter() error: %v", err)
	}
	policy := Policy{Limit: 2, Window: 150 * time.Millisecond}

	first, err := limiter.Allow(context.Background(), key, policy)
	if err != nil || !first.Allowed || first.Remaining != 1 {
		t.Fatalf("first decision = %+v, error = %v", first, err)
	}
	second, err := limiter.Allow(context.Background(), key, policy)
	if err != nil || !second.Allowed || second.Remaining != 0 {
		t.Fatalf("second decision = %+v, error = %v", second, err)
	}
	denied, err := limiter.Allow(context.Background(), key, policy)
	if err != nil || denied.Allowed || denied.RetryAfter <= 0 {
		t.Fatalf("denied decision = %+v, error = %v", denied, err)
	}

	cardinality, err := client.ZCard(context.Background(), redisKey).Result()
	if err != nil {
		t.Fatalf("ZCard() error: %v", err)
	}
	if cardinality != policy.Limit {
		t.Fatalf("stored request count = %d, want %d", cardinality, policy.Limit)
	}

	time.Sleep(policy.Window + 40*time.Millisecond)
	afterReset, err := limiter.Allow(context.Background(), key, policy)
	if err != nil || !afterReset.Allowed {
		t.Fatalf("decision after reset = %+v, error = %v", afterReset, err)
	}
}

func TestRedisLimiterConcurrentAdmission(t *testing.T) {
	client := integrationRedis(t)
	prefix := integrationPrefix(t)
	key := "concurrent"
	redisKey := prefix + ":" + key
	t.Cleanup(func() {
		_ = client.Del(context.Background(), redisKey).Err()
	})

	const instances = 4
	limiters := make([]*RedisLimiter, 0, instances)
	for index := 0; index < instances; index++ {
		limiter, err := NewRedisLimiter(
			client,
			WithPrefix(prefix),
			WithKeyEncoder(func(value string) string { return value }),
		)
		if err != nil {
			t.Fatalf("NewRedisLimiter() error: %v", err)
		}
		limiters = append(limiters, limiter)
	}

	policy := Policy{Limit: 25, Window: 2 * time.Second}
	const requests = 200
	var allowed atomic.Int64
	var failures atomic.Int64
	var group sync.WaitGroup
	group.Add(requests)
	for index := 0; index < requests; index++ {
		go func(index int) {
			defer group.Done()
			decision, err := limiters[index%instances].Allow(context.Background(), key, policy)
			if err != nil {
				failures.Add(1)
				return
			}
			if decision.Allowed {
				allowed.Add(1)
			}
		}(index)
	}
	group.Wait()

	if failures.Load() != 0 {
		t.Fatalf("limiter failures = %d, want 0", failures.Load())
	}
	if allowed.Load() != policy.Limit {
		t.Fatalf("allowed requests = %d, want %d", allowed.Load(), policy.Limit)
	}
	cardinality, err := client.ZCard(context.Background(), redisKey).Result()
	if err != nil {
		t.Fatalf("ZCard() error: %v", err)
	}
	if cardinality != policy.Limit {
		t.Fatalf("stored request count = %d, want %d", cardinality, policy.Limit)
	}
}

func integrationRedis(t *testing.T) *redis.Client {
	t.Helper()
	address := os.Getenv("REDIS_ADDR")
	if address == "" {
		t.Skip("set REDIS_ADDR to run Redis integration tests")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	context, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(context).Err(); err != nil {
		t.Fatalf("connect to integration Redis at %s: %v", address, err)
	}
	return client
}

func integrationPrefix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("ratelimit:test:%d", time.Now().UnixNano())
}
