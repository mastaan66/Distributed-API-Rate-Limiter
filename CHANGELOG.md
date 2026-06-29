# Changelog

All notable changes are documented here. The project follows Semantic
Versioning and the Keep a Changelog structure.

## [Unreleased]

### Added

- Reusable Redis-backed limiter package
- Atomic Lua sliding-window implementation using Redis server time
- Gin and standard net/http middleware
- Safe direct-peer and trusted-proxy identity resolvers
- Fail-open and fail-closed policies
- Rate-limit response headers
- Dynamic per-request policy resolvers for Gin and standard `net/http`
- Report-only enforcement mode for safely evaluating new policies
- Downstream request-context access to selected policies and decisions
- Real-Redis concurrency and integration tests
- Production-shaped demo server and Docker Compose quickstart
- CI, security, and release automation

### Changed

- Corrected the Go module path to match the GitHub repository
- Replaced application timestamps with Redis time
- Rejected requests no longer consume rate-limit capacity

### Removed

- Removed the checked-in platform-specific server executable
