# Architecture

## Components

```text
HTTP request
    |
    v
Gin or net/http middleware
    |
    +-- resolve identity
    +-- apply route policy
    |
    v
ratelimit.RedisLimiter
    |
    v
atomic Redis Lua script
    |
    +-- Redis TIME
    +-- remove expired accepted requests
    +-- count active accepted requests
    +-- conditionally add the request
    +-- set expiry
    +-- return decision metadata
```

The core package has no dependency on Gin. Framework adapters translate HTTP
requests and decisions around the shared `ratelimit.Limiter` interface.

## Data model

Each encoded identity uses one Redis sorted set:

```text
ratelimit:<sha256(identity)>
```

Scores are Redis timestamps in milliseconds. Members are 128-bit random request
identifiers generated with `crypto/rand`.

Only accepted requests are stored. After pruning, cardinality is bounded by the
configured limit for that identity.

## Atomicity

The entire decision executes in one Redis Lua script. Redis executes scripts
atomically, so concurrent application instances cannot both observe the same
capacity and over-admit requests.

go-redis invokes the cached script by SHA and transparently retries with the
script body when Redis returns `NOSCRIPT`.

## Time

The script calls Redis `TIME`. Application-node clock skew therefore does not
affect admission or reset decisions.

The HTTP adapter exposes reset intervals calculated from the Redis response,
not from the local application clock.

## Complexity

For `N` accepted requests currently in a window and `M` entries removed:

- prune: `O(log N + M)`;
- insert: `O(log N)`;
- count: `O(1)`;
- oldest entry lookup: `O(log N)`; and
- memory: `O(limit * active identities)`.

The implementation favors exact behavior. Very large per-identity limits may
be better served by an approximate sliding-window counter, GCRA, or token
bucket.

## Failure behavior

The core returns Redis and context errors. Framework adapters decide whether to
fail closed with HTTP 503 or fail open and call the next handler.

Fail closed is the default because silently bypassing limits is unsafe for many
protected APIs.

## Redis topology

One decision touches one Redis key, making the script compatible with Redis
Cluster key-slot restrictions. Availability, replication, authentication, TLS,
and persistence remain deployment responsibilities.

## Extension points

- Implement `ratelimit.Limiter` for another algorithm or backend.
- Supply custom identity functions to framework adapters.
- Use observer callbacks for metrics and traces.
- Customize denied and limiter-error HTTP responses.
