package ratelimit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mastaan66/Distributed-API-Rate-Limiter/internal/scripts"
	"github.com/redis/go-redis/v9"
)

const defaultPrefix = "ratelimit"

// KeyEncoder converts a user-facing identity into the Redis key suffix.
type KeyEncoder func(string) string

// RedisOption configures a RedisLimiter.
type RedisOption func(*RedisLimiter) error

// RedisLimiter implements an exact sliding-window log with one atomic Redis
// script execution per decision.
type RedisLimiter struct {
	client     redis.Scripter
	prefix     string
	keyEncoder KeyEncoder
	script     *redis.Script
}

// NewRedisLimiter constructs a distributed limiter. Keys are SHA-256 encoded
// by default to keep Redis keys bounded and avoid exposing user identifiers.
func NewRedisLimiter(client redis.Scripter, options ...RedisOption) (*RedisLimiter, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client must not be nil")
	}

	limiter := &RedisLimiter{
		client:     client,
		prefix:     defaultPrefix,
		keyEncoder: SHA256KeyEncoder,
		script:     redis.NewScript(scripts.SlidingWindow),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(limiter); err != nil {
			return nil, err
		}
	}
	return limiter, nil
}

// WithPrefix sets the Redis key prefix used by a limiter instance.
func WithPrefix(prefix string) RedisOption {
	return func(limiter *RedisLimiter) error {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			return fmt.Errorf("redis key prefix must not be empty")
		}
		limiter.prefix = prefix
		return nil
	}
}

// WithKeyEncoder replaces the default SHA-256 identity encoder.
func WithKeyEncoder(encoder KeyEncoder) RedisOption {
	return func(limiter *RedisLimiter) error {
		if encoder == nil {
			return fmt.Errorf("key encoder must not be nil")
		}
		limiter.keyEncoder = encoder
		return nil
	}
}

// SHA256KeyEncoder returns a stable, privacy-preserving key suffix.
func SHA256KeyEncoder(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

// Allow atomically evaluates one request against policy.
func (limiter *RedisLimiter) Allow(ctx context.Context, key string, policy Policy) (Decision, error) {
	if err := policy.Validate(); err != nil {
		return Decision{}, err
	}
	if strings.TrimSpace(key) == "" {
		return Decision{}, ErrEmptyKey
	}
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}

	requestID, err := randomRequestID()
	if err != nil {
		return Decision{}, fmt.Errorf("create request id: %w", err)
	}

	redisKey := limiter.prefix + ":" + limiter.keyEncoder(key)
	result, err := limiter.script.Run(
		ctx,
		limiter.client,
		[]string{redisKey},
		policy.Limit,
		policy.Window.Milliseconds(),
		requestID,
	).Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("execute rate limit script: %w", err)
	}
	if len(result) != 5 {
		return Decision{}, fmt.Errorf("unexpected rate limit response length: got %d", len(result))
	}

	values := make([]int64, len(result))
	for index, value := range result {
		values[index], err = redisInt64(value)
		if err != nil {
			return Decision{}, fmt.Errorf("decode rate limit response field %d: %w", index, err)
		}
	}
	if values[0] != 0 && values[0] != 1 {
		return Decision{}, fmt.Errorf("unexpected allowed value: %d", values[0])
	}

	resetAfter := time.Duration(values[3]-values[4]) * time.Millisecond
	if resetAfter < 0 {
		resetAfter = 0
	}
	decision := Decision{
		Allowed:    values[0] == 1,
		Limit:      values[1],
		Remaining:  values[2],
		ResetAt:    time.UnixMilli(values[3]),
		ResetAfter: resetAfter,
	}
	if !decision.Allowed {
		decision.RetryAfter = resetAfter
	}
	return decision, nil
}

func randomRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func redisInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("expected integer, got %T", value)
	}
}
