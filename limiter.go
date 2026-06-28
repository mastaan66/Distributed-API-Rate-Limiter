// Package ratelimit provides distributed rate-limiting primitives backed by
// Redis. Framework integrations live in the middleware subpackages.
package ratelimit

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrEmptyKey is returned when a request has no rate-limit identity.
	ErrEmptyKey = errors.New("rate limit key must not be empty")
	// ErrInvalidPolicy is returned when a policy cannot be enforced safely.
	ErrInvalidPolicy = errors.New("invalid rate limit policy")
)

// Limiter decides whether a request identified by key may proceed.
type Limiter interface {
	Allow(ctx context.Context, key string, policy Policy) (Decision, error)
}

// Decision describes the outcome of one rate-limit check.
type Decision struct {
	Allowed    bool
	Limit      int64
	Remaining  int64
	ResetAt    time.Time
	ResetAfter time.Duration
	RetryAfter time.Duration
}
