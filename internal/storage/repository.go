package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neexbeast/ygo-test/internal/destination"
)

// Querier abstracts the subset of pgxpool.Pool used by Repository.
// This allows injection of a mock in tests.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Repository provides database access for destination records.
type Repository struct {
	q Querier
}

// NewRepository constructs a Repository backed by the given pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{q: pool}
}

// NewRepositoryWithQuerier constructs a Repository with a custom Querier (for tests).
func NewRepositoryWithQuerier(q Querier) *Repository {
	return &Repository{q: q}
}

// GetDestination retrieves a destination by city name.
// Uses JSONB ? operator to ensure the record has weather data.
// Returns nil, nil when the city is not found.
func (r *Repository) GetDestination(ctx context.Context, city string) (*destination.Destination, error) {
	const q = `
		SELECT id, city, country, data, fetched_at, created_at, updated_at
		FROM destinations
		WHERE city = $1
		AND data ? 'weather'
	`

	var d destination.Destination
	var dataJSON []byte
	var fetchedAt *time.Time

	err := r.q.QueryRow(ctx, q, city).Scan(
		&d.ID,
		&d.City,
		&d.Country,
		&dataJSON,
		&fetchedAt,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying destination for city %s: %w", city, err)
	}

	if err := json.Unmarshal(dataJSON, &d.Data); err != nil {
		return nil, fmt.Errorf("unmarshaling destination data for city %s: %w", city, err)
	}

	d.FetchedAt = fetchedAt
	return &d, nil
}

// UpsertDestination inserts or updates a destination record.
// On conflict (city), updates data, country, fetched_at, and updated_at.
func (r *Repository) UpsertDestination(ctx context.Context, city, country string, data destination.DestinationData) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling destination data for city %s: %w", city, err)
	}

	const q = `
		INSERT INTO destinations (city, country, data, fetched_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (city) DO UPDATE
		SET country    = EXCLUDED.country,
		    data       = EXCLUDED.data,
		    fetched_at = EXCLUDED.fetched_at,
		    updated_at = EXCLUDED.updated_at
	`

	if _, err := r.q.Exec(ctx, q, city, country, dataJSON); err != nil {
		return fmt.Errorf("upserting destination for city %s: %w", city, err)
	}

	return nil
}

// GetDestinationByWeatherCondition returns destinations whose data contains
// a specific weather condition. Uses the JSONB @> containment operator.
func (r *Repository) GetDestinationByWeatherCondition(ctx context.Context, condition string) ([]*destination.Destination, error) {
	filter, err := json.Marshal(map[string]any{
		"weather": map[string]any{"description": condition},
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling JSONB filter: %w", err)
	}

	const q = `
		SELECT id, city, country, data, fetched_at, created_at, updated_at
		FROM destinations
		WHERE data @> $1::jsonb
	`

	rows, err := r.q.Query(ctx, q, string(filter))
	if err != nil {
		return nil, fmt.Errorf("querying destinations by weather condition: %w", err)
	}
	defer rows.Close()

	var results []*destination.Destination
	for rows.Next() {
		var d destination.Destination
		var dataJSON []byte
		var fetchedAt *time.Time

		if err := rows.Scan(
			&d.ID,
			&d.City,
			&d.Country,
			&dataJSON,
			&fetchedAt,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning destination row: %w", err)
		}

		if err := json.Unmarshal(dataJSON, &d.Data); err != nil {
			return nil, fmt.Errorf("unmarshaling destination data: %w", err)
		}

		d.FetchedAt = fetchedAt
		results = append(results, &d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating destination rows: %w", err)
	}

	return results, nil
}
