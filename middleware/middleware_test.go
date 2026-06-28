package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
)

func TestRemoteIPKeyIgnoresForwardedHeader(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.99")

	key, err := RemoteIPKey(request)
	if err != nil {
		t.Fatalf("RemoteIPKey() error: %v", err)
	}
	if key != "192.0.2.10" {
		t.Fatalf("RemoteIPKey() = %q, want direct peer IP", key)
	}
}

func TestTrustedProxyKey(t *testing.T) {
	t.Parallel()

	resolver, err := TrustedProxyKey("10.0.0.0/8")
	if err != nil {
		t.Fatalf("TrustedProxyKey() error: %v", err)
	}

	t.Run("trusted peer", func(t *testing.T) {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = "10.0.0.5:443"
		request.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.4")
		key, err := resolver(request)
		if err != nil {
			t.Fatalf("resolver() error: %v", err)
		}
		if key != "203.0.113.7" {
			t.Fatalf("resolver() = %q, want 203.0.113.7", key)
		}
	})

	t.Run("untrusted peer", func(t *testing.T) {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = "192.0.2.50:443"
		request.Header.Set("X-Forwarded-For", "203.0.113.7")
		key, err := resolver(request)
		if err != nil {
			t.Fatalf("resolver() error: %v", err)
		}
		if key != "192.0.2.50" {
			t.Fatalf("resolver() = %q, want direct peer IP", key)
		}
	})
}

func TestApplyHeaders(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	ApplyHeaders(recorder.Header(), ratelimit.Decision{
		Allowed:    false,
		Limit:      10,
		Remaining:  0,
		ResetAfter: 1500 * time.Millisecond,
		RetryAfter: 1500 * time.Millisecond,
	})
	if got := recorder.Header().Get("RateLimit-Limit"); got != "10" {
		t.Fatalf("RateLimit-Limit = %q, want 10", got)
	}
	if got := recorder.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
}
