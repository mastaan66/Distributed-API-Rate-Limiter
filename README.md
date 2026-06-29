# Distributed API Rate Limiter

[![CI](https://github.com/mastaan66/Distributed-API-Rate-Limiter/actions/workflows/ci.yml/badge.svg)](https://github.com/mastaan66/Distributed-API-Rate-Limiter/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/mastaan66/Distributed-API-Rate-Limiter.svg)](https://pkg.go.dev/github.com/mastaan66/Distributed-API-Rate-Limiter)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A Redis-backed exact sliding-window rate limiter for distributed Go services.
It provides a small core API plus safe middleware for Gin and standard
`net/http`.

> **Project status:** active pre-release development. The exported API may
> change until v1.0.0.

## Why use it?

- One atomic Redis Lua execution per decision
- Redis server time, avoiding clock skew between application nodes
- Exact rolling windows rather than fixed-window boundary spikes
- Rejected requests do not consume capacity
- Memory bounded by the configured limit for each active identity
- SHA-256 encoded Redis keys by default
- Safe direct-peer IP identity by default
- Explicit trusted-proxy support
- Gin and `net/http` adapters
- Dynamic per-request policies for routes, tenants, and subscription plans
- Report-only mode for safely trialing new limits
- Fail-open or fail-closed behavior
- Limit, remaining, reset, and retry response headers
- Policy and decision metadata available to downstream handlers
- Context cancellation, integration tests, and concurrency tests

## Five-minute demo

The quickest path starts Redis and the demo API:

```bash
docker compose up --build
```

Then send seven requests. The first five are accepted and the rest return 429:

```bash
for i in 1 2 3 4 5 6 7; do
  curl -i http://localhost:8080/ping
done
```

Health endpoints are intentionally excluded from rate limiting:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

## Installation

You need Go 1.25.11 or later and a reachable Redis server.

```bash
go get github.com/mastaan66/Distributed-API-Rate-Limiter@latest
```

## Gin

```go
package main

import (
    "log"
    "time"

    "github.com/gin-gonic/gin"
    ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
    ginlimit "github.com/mastaan66/Distributed-API-Rate-Limiter/middleware/gin"
    "github.com/redis/go-redis/v9"
)

func main() {
    client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    limiter, err := ratelimit.NewRedisLimiter(client)
    if err != nil {
        log.Fatal(err)
    }
    guard, err := ginlimit.New(limiter, ginlimit.Options{
        Policy: ratelimit.Policy{Limit: 100, Window: time.Minute},
    })
    if err != nil {
        log.Fatal(err)
    }

    router := gin.Default()
    router.Use(guard)
    router.GET("/api", func(context *gin.Context) {
        context.JSON(200, gin.H{"status": "allowed"})
    })
    log.Fatal(router.Run(":8080"))
}
```

The default key is the direct network peer IP. It ignores
`X-Forwarded-For` until trusted proxies are configured.

## Standard net/http

```go
limiter, _ := ratelimit.NewRedisLimiter(redisClient)
guard, _ := nethttplimit.New(limiter, nethttplimit.Options{
    Policy: ratelimit.Policy{Limit: 100, Window: time.Minute},
})

server := &http.Server{
    Addr:    ":8080",
    Handler: guard(yourHandler),
}
```

See [the complete net/http example](examples/nethttp/main.go).

## Custom identities

Rate limits can represent users, API keys, tenants, routes, or composite
identities:

```go
guard, err := ginlimit.New(limiter, ginlimit.Options{
    Policy: ratelimit.Policy{Limit: 1000, Window: time.Hour},
    Key: func(context *gin.Context) (string, error) {
        tenant := context.GetString("tenant_id")
        user := context.GetString("user_id")
        return tenant + ":" + user + ":" + context.FullPath(), nil
    },
})
```

Do not put secrets directly into custom keys. The Redis limiter hashes
identities by default, but application logs and observers may still expose the
original value if you record it.

## Dynamic policies

Use `PolicyFor` when the quota depends on request data. The returned policy is
validated for every request before Redis is called:

```go
guard, err := ginlimit.New(limiter, ginlimit.Options{
    PolicyFor: func(context *gin.Context) (ratelimit.Policy, error) {
        switch context.GetHeader("X-Plan") {
        case "enterprise":
            return ratelimit.Policy{Limit: 10_000, Window: time.Hour}, nil
        case "pro":
            return ratelimit.Policy{Limit: 1_000, Window: time.Hour}, nil
        default:
            return ratelimit.Policy{Limit: 100, Window: time.Hour}, nil
        }
    },
})
```

`PolicyFor` takes precedence over the static `Policy` field. Resolver errors
and invalid returned policies follow the configured fail-open or fail-closed
behavior.

Requests with different quota scopes should also have different keys. For
example, include the route in `Key` when each route has an independent quota;
omit it when all routes intentionally share one quota.

## Report-only rollout

Trial a policy without rejecting traffic:

```go
guard, err := ginlimit.New(limiter, ginlimit.Options{
    Policy:      ratelimit.Policy{Limit: 100, Window: time.Minute},
    Enforcement: middleware.ReportOnly,
    Observe: func(decision ratelimit.Decision, err error) {
        if err == nil && !decision.Allowed {
            metrics.WouldRateLimit.Inc()
        }
    },
})
```

A would-be denial continues to the next handler and includes
`RateLimit-Report-Only: true`. It does not include `Retry-After`, because the
HTTP request was admitted. The zero value, `middleware.Enforce`, preserves the
default behavior of returning HTTP 429.

## Downstream decision metadata

After a successful limiter check, both adapters attach the selected policy and
decision to the request context:

```go
result, ok := middleware.ResultFromContext(request.Context())
if ok {
    log.Printf(
        "allowed=%t remaining=%d limit=%d",
        result.Decision.Allowed,
        result.Decision.Remaining,
        result.Policy.Limit,
    )
}
```

Gin handlers can use `context.Request.Context()` in the same way. Skipped
requests and fail-open limiter errors do not contain a result.

## Trusted reverse proxies

Forwarding headers are attacker-controlled unless the direct peer is a trusted
proxy. Configure proxy networks explicitly:

```go
proxyKey, err := middleware.TrustedProxyKey(
    "10.0.0.0/8",
    "192.168.0.0/16",
)
```

Adapt the returned function to Gin through `ginlimit.Options.Key`, or use it
directly as `nethttplimit.Options.Key`. See
[proxy security](docs/proxy-security.md) before enabling forwarding headers.

## Failure behavior

The adapters fail closed by default and return HTTP 503 when Redis or key
resolution fails. Services that prefer availability may opt into
`middleware.FailOpen`.

Use fail-open only when temporary rate-limit bypass is less harmful than
rejecting legitimate traffic.

## Response headers

Successful and denied decisions include:

```text
RateLimit-Limit: 100
RateLimit-Remaining: 0
RateLimit-Reset: 12
```

Denied responses also include:

```text
Retry-After: 12
```

Reset and retry values are whole seconds rounded up.

## Algorithm

Each identity maps to one Redis sorted set. A Lua script obtains Redis time,
removes expired entries, counts accepted requests, conditionally inserts a
random request ID, updates expiration, and returns the decision metadata.

Because the operation is one script:

- concurrent nodes cannot over-admit;
- every node uses the same clock;
- denied requests are not stored; and
- script-cache loss is handled automatically by go-redis.

See [architecture](docs/architecture.md) for complexity and operational
tradeoffs.

## Demo configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `HTTP_ADDRESS` | `:8080` | Demo listen address |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `RATE_LIMIT` | `5` | Accepted requests per window |
| `RATE_WINDOW` | `10s` | Rolling window duration |
| `TRUSTED_PROXIES` | empty | Comma-separated proxy CIDRs |

## Development

```bash
make test
make test-integration
make vet
```

Redis integration tests run when `REDIS_ADDR` is set. CI runs both ordinary
and race-enabled tests against real Redis.

## Contributing and security

Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request. Please
report vulnerabilities privately using the process in [SECURITY.md](SECURITY.md).

## License

Released under the [MIT License](LICENSE).
