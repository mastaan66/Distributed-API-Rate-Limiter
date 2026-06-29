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

// PolicyFunc selects a rate-limit policy for an HTTP request.
type PolicyFunc func(*http.Request) (ratelimit.Policy, error)

// Options configures net/http rate-limit middleware.
type Options struct {
	Policy       ratelimit.Policy
	PolicyFor    PolicyFunc
	Key          common.KeyFunc
	FailureMode  common.FailureMode
	Enforcement  common.EnforcementMode
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
	if options.PolicyFor == nil {
		if err := options.Policy.Validate(); err != nil {
			return nil, err
		}
	}
	if options.FailureMode != common.FailClosed && options.FailureMode != common.FailOpen {
		return nil, fmt.Errorf("unsupported failure mode %d", options.FailureMode)
	}
	if options.Enforcement != common.Enforce && options.Enforcement != common.ReportOnly {
		return nil, fmt.Errorf("unsupported enforcement mode %d", options.Enforcement)
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

			policy := options.Policy
			if options.PolicyFor != nil {
				var err error
				policy, err = options.PolicyFor(request)
				if err == nil {
					err = policy.Validate()
				}
				if err != nil {
					observe(options.Observe, ratelimit.Decision{}, err)
					if options.FailureMode == common.FailOpen {
						next.ServeHTTP(writer, request)
						return
					}
					options.LimiterError(writer, request, err)
					return
				}
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

			decision, err := limiter.Allow(request.Context(), key, policy)
			observe(options.Observe, decision, err)
			if err != nil {
				if options.FailureMode == common.FailOpen {
					next.ServeHTTP(writer, request)
					return
				}
				options.LimiterError(writer, request, err)
				return
			}

			request = request.WithContext(common.ContextWithResult(request.Context(), common.Result{
				Policy:   policy,
				Decision: decision,
			}))
			if options.Enforcement == common.ReportOnly {
				common.ApplyReportOnlyHeaders(writer.Header(), decision)
			} else {
				common.ApplyHeaders(writer.Header(), decision)
			}
			if !decision.Allowed && options.Enforcement == common.Enforce {
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
