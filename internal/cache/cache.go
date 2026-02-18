package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/neexbeast/ygo-test/internal/destination"
)

const defaultTTL = time.Hour

// Cache wraps a Redis client and provides typed get/set/delete for destination data.
type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewCache constructs a Cache with a 1-hour TTL.
func NewCache(client *redis.Client) *Cache {
	return &Cache{client: client, ttl: defaultTTL}
}

// key returns the Redis key for the given city.
func key(city string) string {
	return "destination:" + strings.ToLower(strings.TrimSpace(city))
}

// Get retrieves destination data from cache.
// Returns nil, nil on a cache miss (not an error).
func (c *Cache) Get(ctx context.Context, city string) (*destination.DestinationData, error) {
	val, err := c.client.Get(ctx, key(city)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("cache get for city %s: %w", city, err)
	}

	var data destination.DestinationData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, fmt.Errorf("unmarshaling cached data for city %s: %w", city, err)
	}

	return &data, nil
}

// Set stores destination data in cache with the configured TTL.
func (c *Cache) Set(ctx context.Context, city string, data *destination.DestinationData) error {
	if data == nil {
		return nil
	}

	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling destination data for city %s: %w", city, err)
	}

	if err := c.client.Set(ctx, key(city), b, c.ttl).Err(); err != nil {
		return fmt.Errorf("cache set for city %s: %w", city, err)
	}

	return nil
}

// Delete removes the cached entry for the given city.
func (c *Cache) Delete(ctx context.Context, city string) error {
	if err := c.client.Del(ctx, key(city)).Err(); err != nil {
		return fmt.Errorf("cache delete for city %s: %w", city, err)
	}
	return nil
}
