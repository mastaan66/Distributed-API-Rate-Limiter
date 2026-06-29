package nethttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
	common "github.com/mastaan66/Distributed-API-Rate-Limiter/middleware"
)

type stubLimiter struct {
	decision ratelimit.Decision
	err      error
	key      string
	policy   ratelimit.Policy
}

func (stub *stubLimiter) Allow(_ context.Context, key string, policy ratelimit.Policy) (ratelimit.Decision, error) {
	stub.key = key
	stub.policy = policy
	return stub.decision, stub.err
}

func TestMiddlewareAllowsAndAddsHeaders(t *testing.T) {
	t.Parallel()

	limiter := &stubLimiter{decision: ratelimit.Decision{
		Allowed:    true,
		Limit:      5,
		Remaining:  4,
		ResetAfter: time.Second,
	}}
	middleware, err := New(limiter, Options{
		Policy: ratelimit.Policy{Limit: 5, Window: time.Second},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if limiter.key != "192.0.2.10" {
		t.Fatalf("limiter key = %q, want direct peer IP", limiter.key)
	}
	if got := recorder.Header().Get("RateLimit-Remaining"); got != "4" {
		t.Fatalf("RateLimit-Remaining = %q, want 4", got)
	}
}

func TestMiddlewareDenies(t *testing.T) {
	t.Parallel()

	limiter := &stubLimiter{decision: ratelimit.Decision{
		Allowed:    false,
		Limit:      1,
		ResetAfter: time.Second,
		RetryAfter: time.Second,
	}}
	middleware, err := New(limiter, Options{
		Policy: ratelimit.Policy{Limit: 1, Window: time.Second},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("denied request reached next handler")
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if got := recorder.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestMiddlewareDynamicPolicyReportOnlyAndContext(t *testing.T) {
	t.Parallel()

	selected := ratelimit.Policy{Limit: 25, Window: time.Minute}
	limiter := &stubLimiter{decision: ratelimit.Decision{
		Allowed:    false,
		Limit:      selected.Limit,
		Remaining:  0,
		ResetAfter: 10 * time.Second,
		RetryAfter: 10 * time.Second,
	}}
	rateLimit, err := New(limiter, Options{
		PolicyFor: func(request *http.Request) (ratelimit.Policy, error) {
			if request.Header.Get("X-Plan") != "pro" {
				t.Fatalf("PolicyFor() did not receive request headers")
			}
			return selected, nil
		},
		Enforcement: common.ReportOnly,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	reached := false
	handler := rateLimit(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		reached = true
		result, ok := common.ResultFromContext(request.Context())
		if !ok {
			t.Fatal("rate-limit result missing from request context")
		}
		if result.Policy != selected {
			t.Fatalf("context policy = %+v, want %+v", result.Policy, selected)
		}
		if result.Decision.Allowed {
			t.Fatal("context decision = allowed, want report-only denial")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/reports", nil)
	request.Header.Set("X-Plan", "pro")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if !reached {
		t.Fatal("report-only denial did not reach next handler")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if limiter.policy != selected {
		t.Fatalf("limiter policy = %+v, want %+v", limiter.policy, selected)
	}
	if got := recorder.Header().Get("RateLimit-Report-Only"); got != "true" {
		t.Fatalf("RateLimit-Report-Only = %q, want true", got)
	}
	if got := recorder.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After = %q, want empty for admitted request", got)
	}
}

func TestMiddlewareRejectsInvalidDynamicPolicy(t *testing.T) {
	t.Parallel()

	limiter := &stubLimiter{}
	rateLimit, err := New(limiter, Options{
		PolicyFor: func(*http.Request) (ratelimit.Policy, error) {
			return ratelimit.Policy{}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	recorder := httptest.NewRecorder()
	rateLimit(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("invalid policy reached next handler")
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if limiter.key != "" {
		t.Fatalf("limiter was called with key %q", limiter.key)
	}
}

func TestMiddlewareFailureModes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		mode common.FailureMode
		want int
	}{
		{name: "closed", mode: common.FailClosed, want: http.StatusServiceUnavailable},
		{name: "open", mode: common.FailOpen, want: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			limiter := &stubLimiter{err: errors.New("redis unavailable")}
			middleware, err := New(limiter, Options{
				Policy:      ratelimit.Policy{Limit: 1, Window: time.Second},
				FailureMode: test.mode,
			})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			recorder := httptest.NewRecorder()
			middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}
