package gin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ginframework "github.com/gin-gonic/gin"
	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
)

type stubLimiter struct {
	decision ratelimit.Decision
	key      string
}

func (stub *stubLimiter) Allow(_ context.Context, key string, _ ratelimit.Policy) (ratelimit.Decision, error) {
	stub.key = key
	return stub.decision, nil
}

func TestMiddlewareUsesDirectPeerAndAllows(t *testing.T) {
	ginframework.SetMode(ginframework.TestMode)
	limiter := &stubLimiter{decision: ratelimit.Decision{
		Allowed:    true,
		Limit:      5,
		Remaining:  4,
		ResetAfter: time.Second,
	}}
	handler, err := New(limiter, Options{
		Policy: ratelimit.Policy{Limit: 5, Window: time.Second},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	router := ginframework.New()
	router.Use(handler)
	router.GET("/", func(context *ginframework.Context) {
		context.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.99")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if limiter.key != "192.0.2.10" {
		t.Fatalf("limiter key = %q, want direct peer IP", limiter.key)
	}
}

func TestMiddlewareDenies(t *testing.T) {
	ginframework.SetMode(ginframework.TestMode)
	limiter := &stubLimiter{decision: ratelimit.Decision{
		Allowed:    false,
		Limit:      1,
		ResetAfter: time.Second,
		RetryAfter: time.Second,
	}}
	handler, err := New(limiter, Options{
		Policy: ratelimit.Policy{Limit: 1, Window: time.Second},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	router := ginframework.New()
	router.Use(handler)
	router.GET("/", func(*ginframework.Context) {
		t.Fatal("denied request reached route handler")
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
}
