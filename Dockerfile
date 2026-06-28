FROM golang:1.25.11-alpine AS build

ARG VERSION=dev
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/ratelimit-demo ./cmd/demo

FROM scratch

COPY --from=build /out/ratelimit-demo /ratelimit-demo
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/ratelimit-demo"]
