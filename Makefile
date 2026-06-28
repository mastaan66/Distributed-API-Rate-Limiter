.PHONY: build check docker-down docker-up fmt test test-integration vet

GO ?= go

build:
	$(GO) build -trimpath -o bin/ratelimit-demo ./cmd/demo

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

test-integration:
	REDIS_ADDR=localhost:6379 $(GO) test ./...

vet:
	$(GO) vet ./...

check: fmt vet test

docker-up:
	docker compose up --build

docker-down:
	docker compose down
