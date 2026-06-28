// Package nethttp provides distributed rate limiting for standard net/http
// handlers.
package nethttp

import (
	"encoding/json"
	"fmt"
	"net/http"

	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
	common "github.com/mastaan66/Distributed-API-Rate-Limiter/middleware"
)

// Options configures net/http rate-limit middleware.
type Options struct {
	Policy       ratelimit.Policy
	Key          common.KeyFunc
	FailureMode  common.FailureMode
	Skip         func(*http.Request) bool
	Observe      func(ratelimit.Decision, error)
	Denied       func(http.ResponseWriter, *http.Request, ratelimit.Decision)
	LimiterError func(http.ResponseWriter, *http.Request, error)
}

// New constructs middleware around limiter.
func New(limiter ratelimit.Limiter, options Options) (func(http.Handler) http.Handler, error) {
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
		options.Key = common.RemoteIPKey
	}
	if options.Denied == nil {
		options.Denied = defaultDenied
	}
	if options.LimiterError == nil {
		options.LimiterError = defaultLimiterError
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if options.Skip != nil && options.Skip(request) {
				next.ServeHTTP(writer, request)
				return
			}

			key, err := options.Key(request)
			if err != nil {
				observe(options.Observe, ratelimit.Decision{}, err)
				if options.FailureMode == common.FailOpen {
					next.ServeHTTP(writer, request)
					return
				}
				options.LimiterError(writer, request, err)
				return
			}

			decision, err := limiter.Allow(request.Context(), key, options.Policy)
			observe(options.Observe, decision, err)
			if err != nil {
				if options.FailureMode == common.FailOpen {
					next.ServeHTTP(writer, request)
					return
				}
				options.LimiterError(writer, request, err)
				return
			}

			common.ApplyHeaders(writer.Header(), decision)
			if !decision.Allowed {
				options.Denied(writer, request, decision)
				return
			}
			next.ServeHTTP(writer, request)
		})
	}, nil
}

func observe(observer func(ratelimit.Decision, error), decision ratelimit.Decision, err error) {
	if observer != nil {
		observer(decision, err)
	}
}

func defaultDenied(writer http.ResponseWriter, _ *http.Request, _ ratelimit.Decision) {
	writeJSON(writer, http.StatusTooManyRequests, "rate limit exceeded")
}

func defaultLimiterError(writer http.ResponseWriter, _ *http.Request, _ error) {
	writeJSON(writer, http.StatusServiceUnavailable, "rate limiter unavailable")
}

func writeJSON(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": message})
}
