# Proxy and client-IP security

IP-based rate limiting is only useful when client identity cannot be freely
spoofed.

## Safe default

Both middleware adapters use the direct TCP peer from
`http.Request.RemoteAddr`. They ignore `X-Forwarded-For` by default.

This is correct when clients connect directly. Behind a reverse proxy, it
groups requests by proxy address until trusted networks are configured.

## Why forwarding headers are dangerous

Any direct client can send:

```http
X-Forwarded-For: 203.0.113.10
```

Trusting that value without validating the direct peer lets attackers rotate
identities, bypass limits, and create many Redis keys.

## Trusted proxy resolver

`middleware.TrustedProxyKey` accepts forwarding data only when the direct
peer belongs to an explicitly configured CIDR.

```go
key, err := middleware.TrustedProxyKey(
    "10.0.0.0/8",
    "192.168.0.0/16",
)
```

It scans the forwarding chain from the proxy side and returns the nearest
untrusted address. Invalid addresses produce an error instead of silently
changing identity.

## Deployment checklist

- List only networks controlled by your infrastructure.
- Do not use `0.0.0.0/0` or `::/0`.
- Configure the edge proxy to replace untrusted forwarding headers.
- Ensure the application cannot be reached around the edge proxy.
- Prefer authenticated user, API-key, or tenant identities when available.
- Test the actual forwarding chain in staging.
- Decide explicitly whether identity-resolution errors fail open or closed.

## Demo configuration

The demo accepts comma-separated trusted networks:

```bash
TRUSTED_PROXIES=10.0.0.0/8,192.168.0.0/16
```

When this variable is empty, forwarding headers remain disabled.

## Non-IP identities

For authenticated APIs, user or tenant IDs are usually more stable than IP
addresses. Mobile networks, NAT, corporate proxies, and IPv6 privacy addresses
can make IP-based policies unfair or easy to rotate.

Custom identity functions should return stable, non-secret values and include
route or policy names when independent buckets are required.
