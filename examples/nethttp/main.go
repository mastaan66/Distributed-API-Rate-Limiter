package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
	nethttplimit "github.com/mastaan66/Distributed-API-Rate-Limiter/middleware/nethttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer func() { _ = client.Close() }()
	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Fatal(err)
	}

	limiter, err := ratelimit.NewRedisLimiter(client)
	if err != nil {
		log.Fatal(err)
	}
	rateLimit, err := nethttplimit.New(limiter, nethttplimit.Options{
		Policy: ratelimit.Policy{Limit: 100, Window: time.Minute},
	})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("request allowed\n"))
	})
	server := &http.Server{
		Addr:              ":8080",
		Handler:           rateLimit(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
