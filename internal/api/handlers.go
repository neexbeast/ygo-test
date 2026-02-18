package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handlers holds the dependencies for all HTTP handlers.
type Handlers struct {
	repo    DestinationRepo
	cache   DestinationCache
	fetcher DestinationFetcher
	log     *slog.Logger
}

// NewHandlers constructs Handlers with all required dependencies.
func NewHandlers(repo DestinationRepo, cache DestinationCache, fetcher DestinationFetcher, log *slog.Logger) *Handlers {
	return &Handlers{
		repo:    repo,
		cache:   cache,
		fetcher: fetcher,
		log:     log,
	}
}

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// GetDestination handles GET /api/v1/destinations/{city}.
// Cache hit → return. DB hit → cache + return. Neither → 404.
func (h *Handlers) GetDestination(w http.ResponseWriter, r *http.Request) {
	city := chi.URLParam(r, "city")

	cached, err := h.cache.Get(r.Context(), city)
	if err != nil {
		h.log.Error("cache get failed", "city", city, "err", err)
	}
	if cached != nil {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	dest, err := h.repo.GetDestination(r.Context(), city)
	if err != nil {
		h.log.Error("db get failed", "city", city, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if dest == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "destination not found — POST /refresh first"})
		return
	}

	if err := h.cache.Set(r.Context(), city, &dest.Data); err != nil {
		h.log.Warn("cache set failed after db hit", "city", city, "err", err)
	}

	writeJSON(w, http.StatusOK, dest.Data)
}

// RefreshDestination handles POST /api/v1/destinations/{city}/refresh.
// Fetches fresh data, upserts DB, invalidates + repopulates cache.
func (h *Handlers) RefreshDestination(w http.ResponseWriter, r *http.Request) {
	city := chi.URLParam(r, "city")
	country := r.URL.Query().Get("country")
	if country == "" {
		country = city
	}

	data, err := h.fetcher.FetchAll(r.Context(), city, country)
	if err != nil {
		h.log.Error("fetch all failed", "city", city, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch destination data"})
		return
	}

	if err := h.repo.UpsertDestination(r.Context(), city, country, *data); err != nil {
		h.log.Error("upsert failed", "city", city, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store destination data"})
		return
	}

	if err := h.cache.Delete(r.Context(), city); err != nil {
		h.log.Warn("cache delete failed", "city", city, "err", err)
	}
	if err := h.cache.Set(r.Context(), city, data); err != nil {
		h.log.Warn("cache set failed after refresh", "city", city, "err", err)
	}

	writeJSON(w, http.StatusOK, data)
}

// HealthCheck handles GET /api/v1/health.
// Pings DB and Redis; returns 200 if both ok, 503 otherwise.
type dbPinger interface {
	Ping(ctx context.Context) error
}

type redisPinger interface {
	Ping(ctx context.Context) error
}

// HealthHandlerFunc returns an http.HandlerFunc that checks db and redis connectivity.
func HealthHandlerFunc(db dbPinger, redis redisPinger, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := http.StatusOK
		dbStatus := "ok"
		redisStatus := "ok"

		if err := db.Ping(ctx); err != nil {
			log.Error("health check: db ping failed", "err", err)
			dbStatus = "error"
			status = http.StatusServiceUnavailable
		}

		if err := redis.Ping(ctx); err != nil {
			log.Error("health check: redis ping failed", "err", err)
			redisStatus = "error"
			status = http.StatusServiceUnavailable
		}

		writeJSON(w, status, map[string]string{
			"status": func() string {
				if status == http.StatusOK {
					return "ok"
				}
				return "degraded"
			}(),
			"db":    dbStatus,
			"redis": redisStatus,
		})
	}
}
