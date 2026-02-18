# YGO Backend Challenge — Destination Data Aggregation API

A Go REST API that aggregates real-time travel destination data from four external sources,
stores it in PostgreSQL (JSONB), caches it in Redis, and serves it behind bearer token auth.

## Setup

```bash
# 1. Clone the repository
git clone https://github.com/neexbeast/ygo-test.git
cd ygo-test

# 2. Copy env and fill in your API keys
cp .env.example .env
# Edit .env with your OPENWEATHER_API_KEY, OPENTRIPMAP_API_KEY, and BEARER_TOKEN

# 3. Start PostgreSQL + Redis
docker-compose up -d

# 4. Run the server
go run ./cmd/server/main.go
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `BEARER_TOKEN` | Secret token for API authentication |
| `DATABASE_URL` | PostgreSQL connection string |
| `REDIS_URL` | Redis connection string |
| `OPENWEATHER_API_KEY` | OpenWeatherMap API key (free tier) |
| `OPENTRIPMAP_API_KEY` | OpenTripMap API key (free tier) |
| `PORT` | Server port (default: `8080`) |

## API Endpoints

### Health Check (no auth required)

```bash
curl http://localhost:8080/api/v1/health
```
```json
{"db":"ok","redis":"ok","status":"ok"}
```

### Fetch Cached/Stored Destination

```bash
curl -H "Authorization: Bearer your-secret-token" \
  http://localhost:8080/api/v1/destinations/Paris
```

Returns `404` if the city hasn't been refreshed yet. Run the refresh endpoint first.

### Refresh Destination (fetch fresh data from all APIs)

```bash
curl -X POST \
  -H "Authorization: Bearer your-secret-token" \
  "http://localhost:8080/api/v1/destinations/Paris/refresh?country=France"
```
```json
{
  "weather": {
    "temperature": 14.2,
    "feels_like": 13.0,
    "humidity": 72,
    "description": "overcast clouds",
    "wind_speed": 4.1
  },
  "points_of_interest": [
    {"name": "Eiffel Tower", "kinds": "architecture,towers", "rate": 7}
  ],
  "country": {
    "currencies": {"EUR": "Euro"},
    "languages": ["French"],
    "region": "Europe",
    "capital": "Paris"
  },
  "quality_scores": [
    {"name": "Housing", "score_out_of_10": 3.9},
    {"name": "Cost of Living", "score_out_of_10": 4.8},
    {"name": "Safety", "score_out_of_10": 5.1}
  ]
}
```

## Test Coverage

```
ok  github.com/neexbeast/ygo-test/internal/api         coverage: 94.8% of statements
ok  github.com/neexbeast/ygo-test/internal/cache       coverage: 80.0% of statements
ok  github.com/neexbeast/ygo-test/internal/destination coverage: 84.1% of statements
ok  github.com/neexbeast/ygo-test/internal/storage     coverage: 93.3% of statements

total: 88.6% of statements
```

Run tests yourself:
```bash
go test -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out
```

## Architecture

### JSONB Usage
The `destinations.data` column stores all variable destination data (weather, POI, quality scores)
as JSONB. Two JSONB operators are used:
- `?` (key existence): `WHERE city = $1 AND data ? 'weather'` — only return records with weather data
- `@>` (containment): `WHERE data @> $1::jsonb` — query by weather condition

A GIN index on the `data` column keeps these queries fast.

### Parallel Fetching (errgroup)
`Fetcher.FetchAll` uses `golang.org/x/sync/errgroup` to call all four external APIs concurrently.
All failures are non-fatal — partial data is returned with warnings logged. This means even if
Teleport or RestCountries is down, you still get weather and POI data.

### Cache-Aside Pattern
```
GET /destinations/{city}
  → Redis hit?  → return cached JSON
  → DB hit?     → store in Redis, return JSON
  → miss        → 404 (POST /refresh first)

POST /destinations/{city}/refresh
  → Fetch all APIs in parallel
  → Upsert into PostgreSQL
  → Delete + re-set Redis key (1h TTL)
  → Return fresh JSON
```

### External APIs
| API | Data | Auth |
|-----|------|------|
| OpenWeatherMap | Temperature, humidity, wind, conditions | API key |
| OpenTripMap | Top 5 points of interest | API key |
| RestCountries | Currencies, languages, region, capital | None |
| Teleport | Urban quality scores (housing, safety, etc.) | None |
