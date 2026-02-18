package cache_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neexbeast/ygo-test/internal/cache"
	"github.com/neexbeast/ygo-test/internal/destination"
)

func newTestCache(t *testing.T) (*cache.Cache, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return cache.NewCache(client), mr
}

func sampleData() *destination.DestinationData {
	return &destination.DestinationData{
		Weather: &destination.WeatherData{
			Temperature: 22.5,
			Description: "clear sky",
		},
	}
}

func TestCache_SetAndGet(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	data := sampleData()
	require.NoError(t, c.Set(ctx, "Paris", data))

	got, err := c.Get(ctx, "Paris")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 22.5, got.Weather.Temperature)
	assert.Equal(t, "clear sky", got.Weather.Description)
}

func TestCache_Get_Miss(t *testing.T) {
	c, _ := newTestCache(t)

	got, err := c.Get(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got, "cache miss should return nil, nil")
}

func TestCache_CityKeyIsLowercased(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	data := sampleData()
	require.NoError(t, c.Set(ctx, "PARIS", data))

	// Retrieve with different casing — should still hit.
	got, err := c.Get(ctx, "paris")
	require.NoError(t, err)
	require.NotNil(t, got)

	got2, err := c.Get(ctx, "PARIS")
	require.NoError(t, err)
	require.NotNil(t, got2)
}

func TestCache_Delete(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "Paris", sampleData()))
	require.NoError(t, c.Delete(ctx, "Paris"))

	got, err := c.Get(ctx, "Paris")
	require.NoError(t, err)
	assert.Nil(t, got, "entry should be gone after delete")
}

func TestCache_Delete_NonExistent(t *testing.T) {
	c, _ := newTestCache(t)
	// Deleting a key that doesn't exist should not error.
	err := c.Delete(context.Background(), "ghost")
	require.NoError(t, err)
}

func TestCache_Set_NilData(t *testing.T) {
	c, _ := newTestCache(t)
	// Setting nil data should be a no-op, not an error.
	err := c.Set(context.Background(), "Paris", nil)
	require.NoError(t, err)
}

func TestCache_TTL(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "Paris", sampleData()))

	// Fast-forward miniredis time by 2 hours.
	mr.FastForward(2 * 60 * 60 * 1e9) // 2h in nanoseconds

	got, err := c.Get(ctx, "Paris")
	require.NoError(t, err)
	assert.Nil(t, got, "entry should be expired after TTL")
}

func TestConnect_InvalidURL(t *testing.T) {
	_, err := cache.Connect(context.Background(), "not-a-url")
	require.Error(t, err)
}

func TestConnect_UnreachableServer(t *testing.T) {
	_, err := cache.Connect(context.Background(), "redis://localhost:19999")
	require.Error(t, err)
}
