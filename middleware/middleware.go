// Package middleware contains HTTP-framework-neutral helpers shared by the
// framework adapters.
package middleware

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
)

// FailureMode controls behavior when the limiter cannot make a decision.
type FailureMode uint8

const (
	// FailClosed rejects requests when the limiter or identity resolver fails.
	FailClosed FailureMode = iota
	// FailOpen allows requests when the limiter or identity resolver fails.
	FailOpen
)

// EnforcementMode controls whether a denied decision blocks the request.
type EnforcementMode uint8

const (
	// Enforce rejects requests when the limiter denies them.
	Enforce EnforcementMode = iota
	// ReportOnly records and exposes denied decisions but lets requests proceed.
	// It is useful when evaluating a new policy before enforcing it.
	ReportOnly
)

// Result contains the policy and decision evaluated for an HTTP request.
type Result struct {
	Policy   ratelimit.Policy
	Decision ratelimit.Decision
}

type resultContextKey struct{}

// ContextWithResult returns a child context containing a rate-limit result.
// HTTP adapters use this to make decisions available to downstream handlers.
func ContextWithResult(ctx context.Context, result Result) context.Context {
	return context.WithValue(ctx, resultContextKey{}, result)
}

// ResultFromContext returns the rate-limit result attached by HTTP middleware.
func ResultFromContext(ctx context.Context) (Result, bool) {
	if ctx == nil {
		return Result{}, false
	}
	result, ok := ctx.Value(resultContextKey{}).(Result)
	return result, ok
}

// KeyFunc derives a rate-limit identity from an HTTP request.
type KeyFunc func(*http.Request) (string, error)

// RemoteIPKey uses the direct peer IP and deliberately ignores forwarding
// headers. It is the safe default when trusted proxies are not configured.
func RemoteIPKey(request *http.Request) (string, error) {
	if request == nil {
		return "", fmt.Errorf("request must not be nil")
	}
	return parseRemoteIP(request.RemoteAddr)
}

// TrustedProxyKey returns a key resolver that accepts X-Forwarded-For only
// when the direct peer belongs to one of the supplied proxy networks.
func TrustedProxyKey(networks ...string) (KeyFunc, error) {
	trusted := make([]*net.IPNet, 0, len(networks))
	for _, network := range networks {
		_, parsed, err := net.ParseCIDR(strings.TrimSpace(network))
		if err != nil {
			return nil, fmt.Errorf("parse trusted proxy network %q: %w", network, err)
		}
		trusted = append(trusted, parsed)
	}
	if len(trusted) == 0 {
		return nil, fmt.Errorf("at least one trusted proxy network is required")
	}

	return func(request *http.Request) (string, error) {
		if request == nil {
			return "", fmt.Errorf("request must not be nil")
		}
		remote, err := parseRemoteIP(request.RemoteAddr)
		if err != nil {
			return "", err
		}
		remoteIP := net.ParseIP(remote)
		if !containsIP(trusted, remoteIP) {
			return remote, nil
		}

		header := strings.TrimSpace(request.Header.Get("X-Forwarded-For"))
		if header == "" {
			return remote, nil
		}
		forwarded := strings.Split(header, ",")
		addresses := make([]net.IP, 0, len(forwarded))
		for _, value := range forwarded {
			address := net.ParseIP(strings.TrimSpace(value))
			if address == nil {
				return "", fmt.Errorf("invalid X-Forwarded-For address %q", value)
			}
			addresses = append(addresses, address)
		}

		for index := len(addresses) - 1; index >= 0; index-- {
			if !containsIP(trusted, addresses[index]) {
				return addresses[index].String(), nil
			}
		}
		return addresses[0].String(), nil
	}, nil
}

// ApplyHeaders writes commonly supported rate-limit response fields.
func ApplyHeaders(header http.Header, decision ratelimit.Decision) {
	header.Set("RateLimit-Limit", strconv.FormatInt(decision.Limit, 10))
	header.Set("RateLimit-Remaining", strconv.FormatInt(decision.Remaining, 10))
	header.Set("RateLimit-Reset", strconv.FormatInt(durationSeconds(decision.ResetAfter), 10))
	if !decision.Allowed {
		header.Set("Retry-After", strconv.FormatInt(durationSeconds(decision.RetryAfter), 10))
	}
}

// ApplyReportOnlyHeaders writes rate-limit fields without Retry-After because
// a report-only denial does not reject the HTTP request.
func ApplyReportOnlyHeaders(header http.Header, decision ratelimit.Decision) {
	ApplyHeaders(header, decision)
	header.Del("Retry-After")
	header.Set("RateLimit-Report-Only", "true")
}

func parseRemoteIP(remoteAddress string) (string, error) {
	remoteAddress = strings.TrimSpace(remoteAddress)
	if remoteAddress == "" {
		return "", fmt.Errorf("request remote address is empty")
	}
	if host, _, err := net.SplitHostPort(remoteAddress); err == nil {
		remoteAddress = host
	}
	address := net.ParseIP(strings.Trim(remoteAddress, "[]"))
	if address == nil {
		return "", fmt.Errorf("invalid request remote address %q", remoteAddress)
	}
	return address.String(), nil
}

func containsIP(networks []*net.IPNet, address net.IP) bool {
	for _, network := range networks {
		if network.Contains(address) {
			return true
		}
	}
	return false
}

func durationSeconds(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return int64(math.Ceil(value.Seconds()))
}
