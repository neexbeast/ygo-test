package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neexbeast/ygo-test/internal/api"
	"github.com/neexbeast/ygo-test/internal/destination"
)

// ---- mock implementations ----

type mockRepo struct {
	getDestinationFn func(ctx context.Context, city string) (*destination.Destination, error)
	upsertFn         func(ctx context.Context, city, country string, data destination.DestinationData) error
}

func (m *mockRepo) GetDestination(ctx context.Context, city string) (*destination.Destination, error) {
	return m.getDestinationFn(ctx, city)
}
func (m *mockRepo) UpsertDestination(ctx context.Context, city, country string, data destination.DestinationData) error {
	return m.upsertFn(ctx, city, country, data)
}

type mockCache struct {
	getFn    func(ctx context.Context, city string) (*destination.DestinationData, error)
	setFn    func(ctx context.Context, city string, data *destination.DestinationData) error
	deleteFn func(ctx context.Context, city string) error
}

func (m *mockCache) Get(ctx context.Context, city string) (*destination.DestinationData, error) {
	return m.getFn(ctx, city)
}
func (m *mockCache) Set(ctx context.Context, city string, data *destination.DestinationData) error {
	return m.setFn(ctx, city, data)
}
func (m *mockCache) Delete(ctx context.Context, city string) error {
	return m.deleteFn(ctx, city)
}

type mockFetcher struct {
	fetchAllFn func(ctx context.Context, city, country string) (*destination.DestinationData, error)
}

func (m *mockFetcher) FetchAll(ctx context.Context, city, country string) (*destination.DestinationData, error) {
	return m.fetchAllFn(ctx, city, country)
}

type mockPinger struct{ err error }

func (m *mockPinger) Ping(_ context.Context) error { return m.err }

// ---- helpers ----

func sampleData() *destination.DestinationData {
	return &destination.DestinationData{
		Weather: &destination.WeatherData{Temperature: 22.5, Description: "clear sky"},
	}
}

func sampleDest() *destination.Destination {
	return &destination.Destination{
		ID:      1,
		City:    "Paris",
		Country: "France",
		Data:    *sampleData(),
	}
}

const testToken = "secret-token"

func buildRouter(repo api.DestinationRepo, cache api.DestinationCache, fetcher api.DestinationFetcher, db, redis *mockPinger) http.Handler {
	if db == nil {
		db = &mockPinger{}
	}
	if redis == nil {
		redis = &mockPinger{}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := api.NewHandlers(repo, cache, fetcher, log)
	return api.NewRouter(handlers, testToken, db, redis, log)
}

// ---- GET /api/v1/destinations/{city} ----

func TestGetDestination_CacheHit(t *testing.T) {
	data := sampleData()
	repo := &mockRepo{
		getDestinationFn: func(_ context.Context, _ string) (*destination.Destination, error) {
			t.Fatal("repo should not be called on cache hit")
			return nil, nil
		},
		upsertFn: func(_ context.Context, _, _ string, _ destination.DestinationData) error { return nil },
	}
	cache := &mockCache{
		getFn:    func(_ context.Context, _ string) (*destination.DestinationData, error) { return data, nil },
		setFn:    func(_ context.Context, _ string, _ *destination.DestinationData) error { return nil },
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	fetcher := &mockFetcher{
		fetchAllFn: func(_ context.Context, _, _ string) (*destination.DestinationData, error) { return data, nil },
	}

	router := buildRouter(repo, cache, fetcher, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/destinations/Paris", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got destination.DestinationData
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, 22.5, got.Weather.Temperature)
}

func TestGetDestination_DBHit_CacheMiss(t *testing.T) {
	setCalled := false
	repo := &mockRepo{
		getDestinationFn: func(_ context.Context, _ string) (*destination.Destination, error) {
			return sampleDest(), nil
		},
		upsertFn: func(_ context.Context, _, _ string, _ destination.DestinationData) error { return nil },
	}
	cache := &mockCache{
		getFn: func(_ context.Context, _ string) (*destination.DestinationData, error) { return nil, nil },
		setFn: func(_ context.Context, _ string, _ *destination.DestinationData) error {
			setCalled = true
			return nil
		},
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	fetcher := &mockFetcher{
		fetchAllFn: func(_ context.Context, _, _ string) (*destination.DestinationData, error) { return sampleData(), nil },
	}

	router := buildRouter(repo, cache, fetcher, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/destinations/Paris", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, setCalled, "cache.Set should be called after DB hit")
}

func TestGetDestination_NotFound(t *testing.T) {
	repo := &mockRepo{
		getDestinationFn: func(_ context.Context, _ string) (*destination.Destination, error) { return nil, nil },
		upsertFn:         func(_ context.Context, _, _ string, _ destination.DestinationData) error { return nil },
	}
	cache := &mockCache{
		getFn:    func(_ context.Context, _ string) (*destination.DestinationData, error) { return nil, nil },
		setFn:    func(_ context.Context, _ string, _ *destination.DestinationData) error { return nil },
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	fetcher := &mockFetcher{
		fetchAllFn: func(_ context.Context, _, _ string) (*destination.DestinationData, error) { return sampleData(), nil },
	}

	router := buildRouter(repo, cache, fetcher, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/destinations/Atlantis", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDestination_DBError(t *testing.T) {
	repo := &mockRepo{
		getDestinationFn: func(_ context.Context, _ string) (*destination.Destination, error) {
			return nil, fmt.Errorf("db down")
		},
		upsertFn: func(_ context.Context, _, _ string, _ destination.DestinationData) error { return nil },
	}
	cache := &mockCache{
		getFn:    func(_ context.Context, _ string) (*destination.DestinationData, error) { return nil, nil },
		setFn:    func(_ context.Context, _ string, _ *destination.DestinationData) error { return nil },
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	fetcher := &mockFetcher{
		fetchAllFn: func(_ context.Context, _, _ string) (*destination.DestinationData, error) { return sampleData(), nil },
	}

	router := buildRouter(repo, cache, fetcher, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/destinations/Paris", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---- POST /api/v1/destinations/{city}/refresh ----

func TestRefreshDestination_Success(t *testing.T) {
	upsertCalled := false
	repo := &mockRepo{
		getDestinationFn: func(_ context.Context, _ string) (*destination.Destination, error) { return sampleDest(), nil },
		upsertFn: func(_ context.Context, _, _ string, _ destination.DestinationData) error {
			upsertCalled = true
			return nil
		},
	}
	cache := &mockCache{
		getFn:    func(_ context.Context, _ string) (*destination.DestinationData, error) { return nil, nil },
		setFn:    func(_ context.Context, _ string, _ *destination.DestinationData) error { return nil },
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	fetcher := &mockFetcher{
		fetchAllFn: func(_ context.Context, _, _ string) (*destination.DestinationData, error) { return sampleData(), nil },
	}

	router := buildRouter(repo, cache, fetcher, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/destinations/Paris/refresh?country=France", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, upsertCalled)
}

func TestRefreshDestination_FetchError(t *testing.T) {
	repo := &mockRepo{
		getDestinationFn: func(_ context.Context, _ string) (*destination.Destination, error) { return nil, nil },
		upsertFn:         func(_ context.Context, _, _ string, _ destination.DestinationData) error { return nil },
	}
	cache := &mockCache{
		getFn:    func(_ context.Context, _ string) (*destination.DestinationData, error) { return nil, nil },
		setFn:    func(_ context.Context, _ string, _ *destination.DestinationData) error { return nil },
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	fetcher := &mockFetcher{
		fetchAllFn: func(_ context.Context, _, _ string) (*destination.DestinationData, error) {
			return nil, fmt.Errorf("all APIs down")
		},
	}

	router := buildRouter(repo, cache, fetcher, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/destinations/Paris/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRefreshDestination_UpsertError(t *testing.T) {
	repo := &mockRepo{
		getDestinationFn: func(_ context.Context, _ string) (*destination.Destination, error) { return nil, nil },
		upsertFn: func(_ context.Context, _, _ string, _ destination.DestinationData) error {
			return fmt.Errorf("db error")
		},
	}
	cache := &mockCache{
		getFn:    func(_ context.Context, _ string) (*destination.DestinationData, error) { return nil, nil },
		setFn:    func(_ context.Context, _ string, _ *destination.DestinationData) error { return nil },
		deleteFn: func(_ context.Context, _ string) error { return nil },
	}
	fetcher := &mockFetcher{
		fetchAllFn: func(_ context.Context, _, _ string) (*destination.DestinationData, error) { return sampleData(), nil },
	}

	router := buildRouter(repo, cache, fetcher, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/destinations/Paris/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ---- GET /api/v1/health ----

func TestHealth_OK(t *testing.T) {
	router := buildRouter(nil, nil, nil, &mockPinger{}, &mockPinger{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "ok", body["db"])
	assert.Equal(t, "ok", body["redis"])
}

func TestHealth_DBDown(t *testing.T) {
	router := buildRouter(nil, nil, nil,
		&mockPinger{err: fmt.Errorf("db unreachable")},
		&mockPinger{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "error", body["db"])
}

func TestHealth_RedisDown(t *testing.T) {
	router := buildRouter(nil, nil, nil,
		&mockPinger{},
		&mockPinger{err: fmt.Errorf("redis unreachable")},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ---- Auth middleware ----

func TestBearerAuth_NoHeader(t *testing.T) {
	router := buildRouter(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/destinations/Paris", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBearerAuth_WrongToken(t *testing.T) {
	router := buildRouter(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/destinations/Paris", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBearerAuth_HealthNoAuth(t *testing.T) {
	// Health endpoint must not require auth.
	router := buildRouter(nil, nil, nil, &mockPinger{}, &mockPinger{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBearerAuth_MissingBearerPrefix(t *testing.T) {
	router := buildRouter(nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/destinations/Paris", nil)
	req.Header.Set("Authorization", testToken) // no "Bearer " prefix
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
