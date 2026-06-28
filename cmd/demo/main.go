package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	ginframework "github.com/gin-gonic/gin"
	ratelimit "github.com/mastaan66/Distributed-API-Rate-Limiter"
	common "github.com/mastaan66/Distributed-API-Rate-Limiter/middleware"
	ginlimit "github.com/mastaan66/Distributed-API-Rate-Limiter/middleware/gin"
	"github.com/redis/go-redis/v9"
)

var version = "dev"

type config struct {
	Address        string
	RedisAddress   string
	Limit          int64
	Window         time.Duration
	TrustedProxies []string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	configuration, err := loadConfig()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(2)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: configuration.RedisAddress})
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Error("close Redis client", "error", err)
		}
	}()

	startupContext, cancelStartup := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelStartup()
	if err := redisClient.Ping(startupContext).Err(); err != nil {
		logger.Error("connect to Redis", "address", configuration.RedisAddress, "error", err)
		os.Exit(1)
	}

	limiter, err := ratelimit.NewRedisLimiter(redisClient, ratelimit.WithPrefix("demo:ratelimit"))
	if err != nil {
		logger.Error("create limiter", "error", err)
		os.Exit(1)
	}

	ginframework.SetMode(ginframework.ReleaseMode)
	router := ginframework.New()
	router.Use(ginframework.Logger(), ginframework.Recovery())
	router.GET("/healthz", func(context *ginframework.Context) {
		context.JSON(http.StatusOK, ginframework.H{"status": "ok", "version": version})
	})
	router.GET("/readyz", func(context *ginframework.Context) {
		checkContext, cancel := context2s(context.Request.Context())
		defer cancel()
		if err := redisClient.Ping(checkContext).Err(); err != nil {
			context.JSON(http.StatusServiceUnavailable, ginframework.H{"status": "unavailable"})
			return
		}
		context.JSON(http.StatusOK, ginframework.H{"status": "ready"})
	})

	keyFunction, err := requestKey(configuration.TrustedProxies)
	if err != nil {
		logger.Error("configure trusted proxies", "error", err)
		os.Exit(2)
	}
	middleware, err := ginlimit.New(limiter, ginlimit.Options{
		Policy: ratelimit.Policy{Limit: configuration.Limit, Window: configuration.Window},
		Key: func(context *ginframework.Context) (string, error) {
			return keyFunction(context.Request)
		},
		FailureMode: common.FailClosed,
		Observe: func(decision ratelimit.Decision, err error) {
			if err != nil {
				logger.Error("rate limit decision failed", "error", err)
			}
		},
	})
	if err != nil {
		logger.Error("create middleware", "error", err)
		os.Exit(2)
	}
	router.Use(middleware)
	router.GET("/ping", func(context *ginframework.Context) {
		context.JSON(http.StatusOK, ginframework.H{"message": "pong"})
	})
	router.GET("/api/data", func(context *ginframework.Context) {
		context.JSON(http.StatusOK, ginframework.H{"data": "protected demo response"})
	})

	server := &http.Server{
		Addr:              configuration.Address,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	shutdownContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(context); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
	}()

	logger.Info("starting demo server", "address", configuration.Address, "version", version)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

func loadConfig() (config, error) {
	limit, err := strconv.ParseInt(environment("RATE_LIMIT", "5"), 10, 64)
	if err != nil || limit <= 0 {
		return config{}, fmt.Errorf("RATE_LIMIT must be a positive integer")
	}
	window, err := time.ParseDuration(environment("RATE_WINDOW", "10s"))
	if err != nil || window < time.Millisecond {
		return config{}, fmt.Errorf("RATE_WINDOW must be a duration of at least 1ms")
	}
	var trusted []string
	if configured := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); configured != "" {
		for _, network := range strings.Split(configured, ",") {
			trusted = append(trusted, strings.TrimSpace(network))
		}
	}
	return config{
		Address:        environment("HTTP_ADDRESS", ":8080"),
		RedisAddress:   environment("REDIS_ADDR", "localhost:6379"),
		Limit:          limit,
		Window:         window,
		TrustedProxies: trusted,
	}, nil
}

func requestKey(trustedProxies []string) (common.KeyFunc, error) {
	if len(trustedProxies) == 0 {
		return common.RemoteIPKey, nil
	}
	return common.TrustedProxyKey(trustedProxies...)
}

func environment(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func context2s(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 2*time.Second)
}
