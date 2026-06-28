// Package gin provides distributed rate limiting for Gin applications.
package gin

import (
	"fmt"
	"net/http"

	ginframework "github.com/gin-gonic/gin"
	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
	common "github.com/mastaan66/Distributed-API-Rate-Limiter/middleware"
)

// KeyFunc derives a rate-limit identity from a Gin request.
type KeyFunc func(*ginframework.Context) (string, error)

// Options configures Gin rate-limit middleware.
type Options struct {
	Policy       ratelimit.Policy
	Key          KeyFunc
	FailureMode  common.FailureMode
	Skip         func(*ginframework.Context) bool
	Observe      func(ratelimit.Decision, error)
	Denied       func(*ginframework.Context, ratelimit.Decision)
	LimiterError func(*ginframework.Context, error)
}

// New constructs Gin middleware around limiter.
func New(limiter ratelimit.Limiter, options Options) (ginframework.HandlerFunc, error) {
	if limiter == nil {
		return nil, fmt.Errorf("limiter must not be nil")
	}
	if err := options.Policy.Validate(); err != nil {
		return nil, err
	}
	if options.FailureMode != common.FailClosed && options.FailureMode != common.FailOpen {
		return nil, fmt.Errorf("unsupported failure mode %d", options.FailureMode)
	}
	if options.Key == nil {
		options.Key = func(context *ginframework.Context) (string, error) {
			return common.RemoteIPKey(context.Request)
		}
	}
	if options.Denied == nil {
		options.Denied = defaultDenied
	}
	if options.LimiterError == nil {
		options.LimiterError = defaultLimiterError
	}

	return func(context *ginframework.Context) {
		if options.Skip != nil && options.Skip(context) {
			context.Next()
			return
		}

		key, err := options.Key(context)
		if err != nil {
			observe(options.Observe, ratelimit.Decision{}, err)
			if options.FailureMode == common.FailOpen {
				context.Next()
				return
			}
			options.LimiterError(context, err)
			return
		}

		decision, err := limiter.Allow(context.Request.Context(), key, options.Policy)
		observe(options.Observe, decision, err)
		if err != nil {
			if options.FailureMode == common.FailOpen {
				context.Next()
				return
			}
			options.LimiterError(context, err)
			return
		}

		common.ApplyHeaders(context.Writer.Header(), decision)
		if !decision.Allowed {
			options.Denied(context, decision)
			return
		}
		context.Next()
	}, nil
}

func observe(observer func(ratelimit.Decision, error), decision ratelimit.Decision, err error) {
	if observer != nil {
		observer(decision, err)
	}
}

func defaultDenied(context *ginframework.Context, _ ratelimit.Decision) {
	context.AbortWithStatusJSON(http.StatusTooManyRequests, ginframework.H{
		"error": "rate limit exceeded",
	})
}

func defaultLimiterError(context *ginframework.Context, _ error) {
	context.AbortWithStatusJSON(http.StatusServiceUnavailable, ginframework.H{
		"error": "rate limiter unavailable",
	})
}
