# Contributing

Thanks for helping improve Distributed API Rate Limiter.

## Before opening an issue

- Search existing issues and discussions.
- Use the security process for vulnerabilities.
- Include the Go version, Redis version, operating system, configuration, and
  a minimal reproduction for bugs.

## Development setup

You need Go 1.25.11 or later and Redis.

```bash
git clone https://github.com/mastaan66/Distributed-API-Rate-Limiter.git
cd Distributed-API-Rate-Limiter
go mod download
make test
make test-integration
make vet
```

Integration tests use the Redis instance selected by `REDIS_ADDR` and clean
up their own namespaced keys.

## Pull requests

1. Create a focused branch.
2. Add tests for behavior changes.
3. Update documentation and the Unreleased changelog section.
4. Run formatting, tests, the race detector, and vet.
5. Explain the user impact and compatibility implications in the PR.

Keep exported APIs small. New dependencies require a clear maintenance and
security justification.

## Commit guidance

Use clear imperative subjects, for example:

```text
Add trusted proxy key resolver
Fix reset calculation at window boundary
```

## Compatibility

Before v1.0.0, exported APIs may change with release notes. After v1.0.0,
breaking changes require a new major version.

Changes to rate-limit semantics, Redis key formats, Lua return values, or
failure behavior must be treated as compatibility-sensitive.

## Conduct

Participation is governed by [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
