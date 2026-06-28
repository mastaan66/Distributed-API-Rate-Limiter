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
}

func (stub *stubLimiter) Allow(_ context.Context, key string, _ ratelimit.Policy) (ratelimit.Decision, error) {
	stub.key = key
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
