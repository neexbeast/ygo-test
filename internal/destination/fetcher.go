package destination

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/sync/errgroup"
)

// weatherFetcher is the interface satisfied by WeatherClient.
type weatherFetcher interface {
	Fetch(ctx context.Context, city string) (*WeatherData, error)
}

// poiFetcher is the interface satisfied by POIClient.
type poiFetcher interface {
	Fetch(ctx context.Context, city string) ([]POI, error)
}

// countriesFetcher is the interface satisfied by CountriesClient.
type countriesFetcher interface {
	Fetch(ctx context.Context, country string) (*CountryData, error)
}

// teleportFetcher is the interface satisfied by TeleportClient.
type teleportFetcher interface {
	Fetch(ctx context.Context, city string) ([]QualityScore, error)
}

// Fetcher aggregates data from all external APIs in parallel.
type Fetcher struct {
	weather   weatherFetcher
	poi       poiFetcher
	countries countriesFetcher
	teleport  teleportFetcher
}

// NewFetcher constructs a Fetcher with all four API clients using production URLs.
func NewFetcher(weatherKey, poiKey string) *Fetcher {
	return &Fetcher{
		weather:   NewWeatherClient(weatherKey),
		poi:       NewPOIClient(poiKey),
		countries: NewCountriesClient(),
		teleport:  NewTeleportClient(),
	}
}

// NewFetcherWithClients constructs a Fetcher with injectable clients (used in tests).
func NewFetcherWithClients(w weatherFetcher, p poiFetcher, c countriesFetcher, t teleportFetcher) *Fetcher {
	return &Fetcher{weather: w, poi: p, countries: c, teleport: t}
}

// FetchAll fetches data from all external APIs in parallel using errgroup.
// All API failures are non-fatal: partial data is returned with failures logged.
func (f *Fetcher) FetchAll(ctx context.Context, city, country string) (*DestinationData, error) {
	g, gCtx := errgroup.WithContext(ctx)

	var weatherData *WeatherData
	var poiData []POI
	var countryData *CountryData
	var qualityScores []QualityScore

	g.Go(func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("weather fetch panicked", "recover", r)
				err = fmt.Errorf("weather fetch panicked: %v", r)
			}
		}()
		wd, fetchErr := f.weather.Fetch(gCtx, city)
		if fetchErr != nil {
			slog.Warn("weather fetch failed", "city", city, "err", fetchErr)
			return nil
		}
		weatherData = wd
		return nil
	})

	g.Go(func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("poi fetch panicked", "recover", r)
				err = fmt.Errorf("poi fetch panicked: %v", r)
			}
		}()
		pd, fetchErr := f.poi.Fetch(gCtx, city)
		if fetchErr != nil {
			slog.Warn("poi fetch failed", "city", city, "err", fetchErr)
			return nil
		}
		poiData = pd
		return nil
	})

	g.Go(func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("countries fetch panicked", "recover", r)
				err = fmt.Errorf("countries fetch panicked: %v", r)
			}
		}()
		cd, fetchErr := f.countries.Fetch(gCtx, country)
		if fetchErr != nil {
			slog.Warn("countries fetch failed", "country", country, "err", fetchErr)
			return nil
		}
		countryData = cd
		return nil
	})

	g.Go(func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("teleport fetch panicked", "recover", r)
				err = fmt.Errorf("teleport fetch panicked: %v", r)
			}
		}()
		qs, fetchErr := f.teleport.Fetch(gCtx, city)
		if fetchErr != nil {
			slog.Warn("teleport fetch failed", "city", city, "err", fetchErr)
			return nil
		}
		qualityScores = qs
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("fetching destination data for %s: %w", city, err)
	}

	return &DestinationData{
		Weather:       weatherData,
		PointsOfInt:   poiData,
		Country:       countryData,
		QualityScores: qualityScores,
	}, nil
}
