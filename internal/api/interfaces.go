package api

import (
	"context"

	"github.com/neexbeast/ygo-test/internal/destination"
)

// DestinationRepo defines the storage operations needed by handlers.
type DestinationRepo interface {
	GetDestination(ctx context.Context, city string) (*destination.Destination, error)
	UpsertDestination(ctx context.Context, city, country string, data destination.DestinationData) error
}

// DestinationCache defines the cache operations needed by handlers.
type DestinationCache interface {
	Get(ctx context.Context, city string) (*destination.DestinationData, error)
	Set(ctx context.Context, city string, data *destination.DestinationData) error
	Delete(ctx context.Context, city string) error
}

// DestinationFetcher defines the external API aggregation needed by handlers.
type DestinationFetcher interface {
	FetchAll(ctx context.Context, city, country string) (*destination.DestinationData, error)
}
