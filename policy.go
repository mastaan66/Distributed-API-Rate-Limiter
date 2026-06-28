package ratelimit

import (
	"fmt"
	"time"
)

const maxLuaInteger = int64(1<<53 - 1)

// Policy defines the maximum number of accepted requests in a rolling window.
type Policy struct {
	Limit  int64
	Window time.Duration
}

// Validate checks whether the policy can be represented by the Redis script.
func (p Policy) Validate() error {
	if p.Limit <= 0 {
		return fmt.Errorf("%w: limit must be greater than zero", ErrInvalidPolicy)
	}
	if p.Limit > maxLuaInteger {
		return fmt.Errorf("%w: limit exceeds Redis Lua integer precision", ErrInvalidPolicy)
	}
	if p.Window < time.Millisecond {
		return fmt.Errorf("%w: window must be at least one millisecond", ErrInvalidPolicy)
	}
	if p.Window.Milliseconds() <= 0 {
		return fmt.Errorf("%w: window is outside the supported range", ErrInvalidPolicy)
	}
	return nil
}
