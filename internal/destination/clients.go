package destination

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const httpTimeout = 10 * time.Second

// newHTTPClient returns an http.Client with a 10-second timeout.
func newHTTPClient() *http.Client {
	return &http.Client{Timeout: httpTimeout}
}

// doGet performs a GET request and decodes the JSON response into dst.
func doGet(ctx context.Context, client *http.Client, rawURL string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("creating request for %s: %w", rawURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s returned status %d", rawURL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decoding response from %s: %w", rawURL, err)
	}

	return nil
}

// ---- OpenWeatherMap ----

// WeatherClient fetches current weather from OpenWeatherMap.
type WeatherClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

const owmDefaultURL = "https://api.openweathermap.org/data/2.5/weather"

// NewWeatherClient constructs a WeatherClient with the given API key.
func NewWeatherClient(apiKey string) *WeatherClient {
	return &WeatherClient{apiKey: apiKey, baseURL: owmDefaultURL, client: newHTTPClient()}
}

// NewWeatherClientWithURL constructs a WeatherClient pointing at a custom base URL (for tests).
func NewWeatherClientWithURL(baseURL, apiKey string) *WeatherClient {
	return &WeatherClient{apiKey: apiKey, baseURL: baseURL, client: newHTTPClient()}
}

type owmResponse struct {
	Main struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
		Humidity  int     `json:"humidity"`
	} `json:"main"`
	Weather []struct {
		Description string `json:"description"`
	} `json:"weather"`
	Wind struct {
		Speed float64 `json:"speed"`
	} `json:"wind"`
}

// Fetch retrieves weather data for the given city.
func (c *WeatherClient) Fetch(ctx context.Context, city string) (*WeatherData, error) {
	endpoint := c.baseURL + "?q=" + url.QueryEscape(city) + "&appid=" + c.apiKey + "&units=metric"

	var raw owmResponse
	if err := doGet(ctx, c.client, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("openweathermap fetch for %s: %w", city, err)
	}

	description := ""
	if len(raw.Weather) > 0 {
		description = raw.Weather[0].Description
	}

	return &WeatherData{
		Temperature: raw.Main.Temp,
		FeelsLike:   raw.Main.FeelsLike,
		Humidity:    raw.Main.Humidity,
		Description: description,
		WindSpeed:   raw.Wind.Speed,
	}, nil
}

// ---- OpenTripMap ----

// POIClient fetches points of interest from OpenTripMap.
type POIClient struct {
	apiKey     string
	geoBaseURL string
	poiBaseURL string
	client     *http.Client
}

const (
	otmGeoDefault = "https://api.opentripmap.com/0.1/en/places/geoname"
	otmPOIDefault = "https://api.opentripmap.com/0.1/en/places/radius"
)

// NewPOIClient constructs a POIClient with the given API key.
func NewPOIClient(apiKey string) *POIClient {
	return &POIClient{
		apiKey:     apiKey,
		geoBaseURL: otmGeoDefault,
		poiBaseURL: otmPOIDefault,
		client:     newHTTPClient(),
	}
}

// NewPOIClientWithURLs constructs a POIClient pointing at custom URLs (for tests).
func NewPOIClientWithURLs(geoBaseURL, poiBaseURL, apiKey string) *POIClient {
	return &POIClient{
		apiKey:     apiKey,
		geoBaseURL: geoBaseURL,
		poiBaseURL: poiBaseURL,
		client:     newHTTPClient(),
	}
}

type otmGeoResponse struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type otmRadiusResponse struct {
	Features []struct {
		Properties struct {
			Name  string `json:"name"`
			Kinds string `json:"kinds"`
			Rate  int    `json:"rate"`
		} `json:"properties"`
	} `json:"features"`
}

// Fetch retrieves the top 5 points of interest near the given city.
func (c *POIClient) Fetch(ctx context.Context, city string) ([]POI, error) {
	geoURL := c.geoBaseURL + "?name=" + url.QueryEscape(city) + "&apikey=" + c.apiKey

	var geo otmGeoResponse
	if err := doGet(ctx, c.client, geoURL, &geo); err != nil {
		return nil, fmt.Errorf("opentripmap geocode for %s: %w", city, err)
	}

	poiURL := fmt.Sprintf(
		"%s?radius=5000&lon=%f&lat=%f&limit=5&format=geojson&apikey=%s",
		c.poiBaseURL, geo.Lon, geo.Lat, c.apiKey,
	)

	var raw otmRadiusResponse
	if err := doGet(ctx, c.client, poiURL, &raw); err != nil {
		return nil, fmt.Errorf("opentripmap radius for %s: %w", city, err)
	}

	pois := make([]POI, 0, len(raw.Features))
	for _, f := range raw.Features {
		if f.Properties.Name == "" {
			continue
		}
		pois = append(pois, POI{
			Name:  f.Properties.Name,
			Kinds: f.Properties.Kinds,
			Rate:  f.Properties.Rate,
		})
	}

	return pois, nil
}

// ---- RestCountries ----

// CountriesClient fetches country info from RestCountries (no API key required).
type CountriesClient struct {
	baseURL string
	client  *http.Client
}

const countriesDefaultURL = "https://restcountries.com/v3.1/name"

// NewCountriesClient constructs a CountriesClient.
func NewCountriesClient() *CountriesClient {
	return &CountriesClient{baseURL: countriesDefaultURL, client: newHTTPClient()}
}

// NewCountriesClientWithURL constructs a CountriesClient pointing at a custom base URL (for tests).
func NewCountriesClientWithURL(baseURL string) *CountriesClient {
	return &CountriesClient{baseURL: baseURL, client: newHTTPClient()}
}

type restCountriesEntry struct {
	Capital    []string          `json:"capital"`
	Region     string            `json:"region"`
	Languages  map[string]string `json:"languages"`
	Currencies map[string]struct {
		Name string `json:"name"`
	} `json:"currencies"`
}

// Fetch retrieves country data for the given country name.
func (c *CountriesClient) Fetch(ctx context.Context, country string) (*CountryData, error) {
	endpoint := c.baseURL + "/" + url.QueryEscape(country) + "?fullText=true"

	var raw []restCountriesEntry
	if err := doGet(ctx, c.client, endpoint, &raw); err != nil {
		return nil, fmt.Errorf("restcountries fetch for %s: %w", country, err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("restcountries: no results for %s", country)
	}

	entry := raw[0]

	currencies := make(map[string]string, len(entry.Currencies))
	for code, cur := range entry.Currencies {
		currencies[code] = cur.Name
	}

	languages := make([]string, 0, len(entry.Languages))
	for _, lang := range entry.Languages {
		languages = append(languages, lang)
	}

	capital := ""
	if len(entry.Capital) > 0 {
		capital = entry.Capital[0]
	}

	return &CountryData{
		Currencies: currencies,
		Languages:  languages,
		Region:     entry.Region,
		Capital:    capital,
	}, nil
}

// ---- Teleport ----

// TeleportClient fetches urban quality scores from the Teleport API (no key required).
type TeleportClient struct {
	urlBuilder func(city string) string
	client     *http.Client
}

// NewTeleportClient constructs a TeleportClient using the production Teleport API URL.
func NewTeleportClient() *TeleportClient {
	return &TeleportClient{
		urlBuilder: func(city string) string {
			return "https://api.teleport.org/api/urban_areas/slug:" + cityToSlug(city) + "/scores/"
		},
		client: newHTTPClient(),
	}
}

// NewTeleportClientWithURL constructs a TeleportClient that always uses the given URL (for tests).
// The city slug is ignored — the full URL is used directly.
func NewTeleportClientWithURL(fixedURL string) *TeleportClient {
	return &TeleportClient{
		urlBuilder: func(_ string) string { return fixedURL },
		client:     newHTTPClient(),
	}
}

type teleportScoresResponse struct {
	Categories []struct {
		Name       string  `json:"name"`
		ScoreOutOf float64 `json:"score_out_of_10"`
	} `json:"categories"`
}

// cityToSlug converts a city name to a Teleport-compatible slug (lowercase, spaces→hyphens).
func cityToSlug(city string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(city), " ", "-"))
}

// Fetch retrieves urban quality scores for the given city.
func (c *TeleportClient) Fetch(ctx context.Context, city string) ([]QualityScore, error) {
	endpoint := c.urlBuilder(city)

	var raw teleportScoresResponse
	if err := doGet(ctx, c.client, endpoint, &raw); err != nil {
		slog.Warn("teleport fetch failed", "city", city, "err", err)
		return nil, fmt.Errorf("teleport fetch for %s: %w", city, err)
	}

	scores := make([]QualityScore, 0, len(raw.Categories))
	for _, cat := range raw.Categories {
		scores = append(scores, QualityScore{
			Name:       cat.Name,
			ScoreOutOf: cat.ScoreOutOf,
		})
	}

	return scores, nil
}
