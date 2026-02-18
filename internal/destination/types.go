package destination

import "time"

// WeatherData holds current weather conditions for a city.
type WeatherData struct {
	Temperature float64 `json:"temperature"`
	FeelsLike   float64 `json:"feels_like"`
	Humidity    int     `json:"humidity"`
	Description string  `json:"description"`
	WindSpeed   float64 `json:"wind_speed"`
}

// POI represents a single point of interest.
type POI struct {
	Name  string `json:"name"`
	Kinds string `json:"kinds"`
	Rate  int    `json:"rate"`
}

// CountryData holds country-level information.
type CountryData struct {
	Currencies map[string]string `json:"currencies"`
	Languages  []string          `json:"languages"`
	Region     string            `json:"region"`
	Capital    string            `json:"capital"`
}

// QualityScore represents a single urban quality metric.
type QualityScore struct {
	Name       string  `json:"name"`
	ScoreOutOf float64 `json:"score_out_of_10"`
}

// DestinationData is the aggregated result from all external APIs.
type DestinationData struct {
	Weather       *WeatherData   `json:"weather,omitempty"`
	PointsOfInt   []POI          `json:"points_of_interest,omitempty"`
	Country       *CountryData   `json:"country,omitempty"`
	QualityScores []QualityScore `json:"quality_scores,omitempty"`
}

// Destination is a fully stored destination record from the DB.
type Destination struct {
	ID        int
	City      string
	Country   string
	Data      DestinationData
	FetchedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
