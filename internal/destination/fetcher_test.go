package destination_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neexbeast/ygo-test/internal/destination"
)

// buildTestFetcher creates a Fetcher that points all clients at the given test servers.
func buildTestFetcher(weatherURL, poiGeoURL, poiRadiusURL, countriesURL, teleportURL string) *destination.Fetcher {
	return destination.NewFetcherWithClients(
		destination.NewWeatherClientWithURL(weatherURL, "test-key"),
		destination.NewPOIClientWithURLs(poiGeoURL, poiRadiusURL, "test-key"),
		destination.NewCountriesClientWithURL(countriesURL),
		destination.NewTeleportClientWithURL(teleportURL),
	)
}

func weatherHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"main": map[string]any{
				"temp":       22.5,
				"feels_like": 21.0,
				"humidity":   60,
			},
			"weather": []map[string]any{{"description": "clear sky"}},
			"wind":    map[string]any{"speed": 3.5},
		})
	}
}

func geoHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"lat": 48.8566, "lon": 2.3522})
	}
}

func poiHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"features": []map[string]any{
				{
					"properties": map[string]any{
						"name":  "Eiffel Tower",
						"kinds": "architecture",
						"rate":  7,
					},
				},
			},
		})
	}
}

func countriesHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"capital":    []string{"Paris"},
				"region":     "Europe",
				"languages":  map[string]string{"fra": "French"},
				"currencies": map[string]any{"EUR": map[string]string{"name": "Euro"}},
			},
		})
	}
}

func teleportHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"categories": []map[string]any{
				{"name": "Housing", "score_out_of_10": 5.5},
				{"name": "Safety", "score_out_of_10": 6.0},
			},
		})
	}
}

func TestFetchAll_Success(t *testing.T) {
	wSrv := httptest.NewServer(weatherHandler(t))
	defer wSrv.Close()

	geoSrv := httptest.NewServer(geoHandler(t))
	defer geoSrv.Close()

	poiSrv := httptest.NewServer(poiHandler(t))
	defer poiSrv.Close()

	cSrv := httptest.NewServer(countriesHandler(t))
	defer cSrv.Close()

	tSrv := httptest.NewServer(teleportHandler(t))
	defer tSrv.Close()

	f := buildTestFetcher(wSrv.URL, geoSrv.URL, poiSrv.URL, cSrv.URL, tSrv.URL)

	data, err := f.FetchAll(context.Background(), "Paris", "France")
	require.NoError(t, err)
	require.NotNil(t, data)

	require.NotNil(t, data.Weather)
	assert.Equal(t, 22.5, data.Weather.Temperature)
	assert.Equal(t, "clear sky", data.Weather.Description)

	require.Len(t, data.PointsOfInt, 1)
	assert.Equal(t, "Eiffel Tower", data.PointsOfInt[0].Name)

	require.NotNil(t, data.Country)
	assert.Equal(t, "Europe", data.Country.Region)
	assert.Equal(t, "Paris", data.Country.Capital)

	require.Len(t, data.QualityScores, 2)
}

func TestFetchAll_WeatherFails_PartialData(t *testing.T) {
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	geoSrv := httptest.NewServer(geoHandler(t))
	defer geoSrv.Close()

	poiSrv := httptest.NewServer(poiHandler(t))
	defer poiSrv.Close()

	cSrv := httptest.NewServer(countriesHandler(t))
	defer cSrv.Close()

	tSrv := httptest.NewServer(teleportHandler(t))
	defer tSrv.Close()

	f := buildTestFetcher(badSrv.URL, geoSrv.URL, poiSrv.URL, cSrv.URL, tSrv.URL)

	data, err := f.FetchAll(context.Background(), "Paris", "France")
	require.NoError(t, err)
	require.NotNil(t, data)

	assert.Nil(t, data.Weather, "weather should be nil on failure")
	require.NotNil(t, data.Country)
	require.Len(t, data.QualityScores, 2)
}

func TestFetchAll_AllAPIsFail_ReturnsPartial(t *testing.T) {
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	f := buildTestFetcher(badSrv.URL, badSrv.URL, badSrv.URL, badSrv.URL, badSrv.URL)

	data, err := f.FetchAll(context.Background(), "Paris", "France")
	require.NoError(t, err)
	require.NotNil(t, data)

	assert.Nil(t, data.Weather)
	assert.Nil(t, data.Country)
	assert.Empty(t, data.PointsOfInt)
	assert.Empty(t, data.QualityScores)
}

func TestFetchAll_Timeout(t *testing.T) {
	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer slowSrv.Close()

	f := buildTestFetcher(slowSrv.URL, slowSrv.URL, slowSrv.URL, slowSrv.URL, slowSrv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// With partial-failure mode, timeout causes all fetches to return nil — no error.
	data, err := f.FetchAll(ctx, "Paris", "France")
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Nil(t, data.Weather)
}

func TestWeatherClient_Fetch(t *testing.T) {
	srv := httptest.NewServer(weatherHandler(t))
	defer srv.Close()

	c := destination.NewWeatherClientWithURL(srv.URL, "key")
	wd, err := c.Fetch(context.Background(), "Paris")
	require.NoError(t, err)
	require.NotNil(t, wd)
	assert.Equal(t, 22.5, wd.Temperature)
	assert.Equal(t, 60, wd.Humidity)
}

func TestWeatherClient_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "err", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := destination.NewWeatherClientWithURL(srv.URL, "key")
	_, err := c.Fetch(context.Background(), "Paris")
	require.Error(t, err)
}

func TestPOIClient_Fetch(t *testing.T) {
	geoSrv := httptest.NewServer(geoHandler(t))
	defer geoSrv.Close()

	poiSrv := httptest.NewServer(poiHandler(t))
	defer poiSrv.Close()

	c := destination.NewPOIClientWithURLs(geoSrv.URL, poiSrv.URL, "key")
	pois, err := c.Fetch(context.Background(), "Paris")
	require.NoError(t, err)
	require.Len(t, pois, 1)
	assert.Equal(t, "Eiffel Tower", pois[0].Name)
}

func TestPOIClient_GeoFails(t *testing.T) {
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusInternalServerError)
	}))
	defer badSrv.Close()

	c := destination.NewPOIClientWithURLs(badSrv.URL, badSrv.URL, "key")
	_, err := c.Fetch(context.Background(), "Paris")
	require.Error(t, err)
}

func TestCountriesClient_Fetch(t *testing.T) {
	srv := httptest.NewServer(countriesHandler(t))
	defer srv.Close()

	c := destination.NewCountriesClientWithURL(srv.URL)
	cd, err := c.Fetch(context.Background(), "France")
	require.NoError(t, err)
	require.NotNil(t, cd)
	assert.Equal(t, "Europe", cd.Region)
	assert.Equal(t, "Paris", cd.Capital)
}

func TestCountriesClient_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	c := destination.NewCountriesClientWithURL(srv.URL)
	_, err := c.Fetch(context.Background(), "Nowhere")
	require.Error(t, err)
}

func TestTeleportClient_Fetch(t *testing.T) {
	srv := httptest.NewServer(teleportHandler(t))
	defer srv.Close()

	c := destination.NewTeleportClientWithURL(srv.URL)
	scores, err := c.Fetch(context.Background(), "Paris")
	require.NoError(t, err)
	require.Len(t, scores, 2)
}

func TestTeleportClient_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := destination.NewTeleportClientWithURL(srv.URL)
	_, err := c.Fetch(context.Background(), "Unknown")
	require.Error(t, err)
}
