package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/neexbeast/ygo-test/internal/api"
	"github.com/neexbeast/ygo-test/internal/cache"
	"github.com/neexbeast/ygo-test/internal/destination"
	"github.com/neexbeast/ygo-test/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	databaseURL := mustEnv("DATABASE_URL")
	redisURL := mustEnv("REDIS_URL")
	bearerToken := mustEnv("BEARER_TOKEN")
	weatherKey := mustEnv("OPENWEATHER_API_KEY")
	poiKey := mustEnv("OPENTRIPMAP_API_KEY")
	port := getEnv("PORT", "8080")

	ctx := context.Background()

	// Connect to PostgreSQL.
	pool, err := storage.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Run migrations.
	migrationsDir := "migrations"
	if err := storage.RunMigrations(ctx, pool, migrationsDir); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	log.Info("migrations applied")

	// Connect to Redis.
	redisClient, err := cache.Connect(ctx, redisURL)
	if err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	defer func() { _ = redisClient.Close() }()

	// Wire dependencies.
	repo := storage.NewRepository(pool)
	cacheLayer := cache.NewCache(redisClient)
	fetcher := destination.NewFetcher(weatherKey, poiKey)
	handlers := api.NewHandlers(repo, cacheLayer, fetcher, log)

	// Build router with pingers adapted for health check.
	dbPinger := &pgxPoolPinger{pool: pool}
	redisPinger := &redisPingerAdapter{client: redisClient}

	router := api.NewRouter(handlers, bearerToken, dbPinger, redisPinger, log)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("server goroutine panicked", "recover", r)
				errCh <- fmt.Errorf("server panicked: %v", r)
			}
		}()
		log.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("listening: %w", err)
		}
	}()

	select {
	case sig := <-quit:
		log.Info("shutdown signal received", "signal", sig)
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	log.Info("server shut down cleanly")
	return nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// pgxPoolPinger adapts pgxpool.Pool to the api.dbPinger interface.
type pgxPoolPinger struct {
	pool interface {
		Ping(ctx context.Context) error
	}
}

func (p *pgxPoolPinger) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// redisPingerAdapter adapts redis.Client to the api.redisPinger interface.
type redisPingerAdapter struct {
	client *redis.Client
}

func (r *redisPingerAdapter) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
