package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

// NewRouter builds and returns the Chi router with all routes configured.
// The health endpoint is unauthenticated; all destination routes require bearer auth.
// Rate limiting is applied globally: 60 requests per minute per IP.
func NewRouter(handlers *Handlers, token string, db dbPinger, redisClient redisPinger, log *slog.Logger) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(httprate.LimitByIP(60, time.Minute))

	r.Get("/api/v1/health", HealthHandlerFunc(db, redisClient, log))

	r.Group(func(r chi.Router) {
		r.Use(BearerAuth(token))
		r.Get("/api/v1/destinations/{city}", handlers.GetDestination)
		r.Post("/api/v1/destinations/{city}/refresh", handlers.RefreshDestination)
	})

	return r
}

// Ensure chi.Mux implements http.Handler.
var _ http.Handler = (*chi.Mux)(nil)
