package gin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ginframework "github.com/gin-gonic/gin"
	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
	common "github.com/mastaan66/Distributed-API-Rate-Limiter/middleware"
)

type stubLimiter struct {
	decision ratelimit.Decision
	key      string
	policy   ratelimit.Policy
}

func (stub *stubLimiter) Allow(_ context.Context, key string, policy ratelimit.Policy) (ratelimit.Decision, error) {
	stub.key = key
	stub.policy = policy
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

func TestMiddlewareDynamicPolicyReportOnlyAndContext(t *testing.T) {
	ginframework.SetMode(ginframework.TestMode)

	selected := ratelimit.Policy{Limit: 50, Window: time.Minute}
	limiter := &stubLimiter{decision: ratelimit.Decision{
		Allowed:    false,
		Limit:      selected.Limit,
		ResetAfter: 5 * time.Second,
		RetryAfter: 5 * time.Second,
	}}
	handler, err := New(limiter, Options{
		PolicyFor: func(context *ginframework.Context) (ratelimit.Policy, error) {
			if context.GetHeader("X-Plan") != "enterprise" {
				t.Fatalf("PolicyFor() did not receive Gin context")
			}
			return selected, nil
		},
		Enforcement: common.ReportOnly,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	reached := false
	router := ginframework.New()
	router.Use(handler)
	router.GET("/", func(context *ginframework.Context) {
		reached = true
		result, ok := common.ResultFromContext(context.Request.Context())
		if !ok {
			t.Fatal("rate-limit result missing from request context")
		}
		if result.Policy != selected {
			t.Fatalf("context policy = %+v, want %+v", result.Policy, selected)
		}
		if result.Decision.Allowed {
			t.Fatal("context decision = allowed, want report-only denial")
		}
		context.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Plan", "enterprise")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if !reached {
		t.Fatal("report-only denial did not reach route handler")
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
